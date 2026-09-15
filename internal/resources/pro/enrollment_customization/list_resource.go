// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListEnrollmentCustomizationsV2
// Status: current. Last reviewed 2026-05-28.

package enrollment_customization

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait. The Pro v2
// list endpoint is paginated client-side by the SDK; 90 seconds is more than
// enough for tenants with hundreds of customizations.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item pane hydration issued when
// IncludeResource asks for full resource state.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &EnrollmentCustomizationListResource{}
	_ list.ListResourceWithConfigure = &EnrollmentCustomizationListResource{}
)

// EnrollmentCustomizationListResource implements Terraform query list support
// for Jamf Pro enrollment customizations. The Pro v2 list endpoint does not
// accept an RSQL filter, so the optional `filter` block is applied
// client-side after the full list is fetched.
type EnrollmentCustomizationListResource struct {
	client *pro.Client
}

// NewEnrollmentCustomizationListResource returns a list resource for Jamf Pro
// enrollment customization queries.
func NewEnrollmentCustomizationListResource() list.ListResource {
	return &EnrollmentCustomizationListResource{}
}

// Metadata sets the list resource type name.
func (r *EnrollmentCustomizationListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_enrollment_customization"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *EnrollmentCustomizationListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_enrollment_customization")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *EnrollmentCustomizationListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro enrollment customizations. An optional case-insensitive `name_substring` filter is applied client-side after the full list is fetched. Each row carries the parent record: display name, description, site, branding palette and icon URL. Panes are not included; read them with the singular resource per ID." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams customization identities back to
// Terraform.
func (r *EnrollmentCustomizationListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config EnrollmentCustomizationListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListEnrollmentCustomizationsV2(listCtx, nil)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro enrollment customizations", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, enrollmentCustomizationListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for i := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		item := items[i]

		result := req.NewListResult(ctx)
		result.DisplayName = item.DisplayName

		var id types.String
		if item.ID != nil {
			id = types.StringValue(*item.ID)
		} else {
			id = types.StringNull()
		}
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, EnrollmentCustomizationIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The list endpoint carries only the parent record. Leaving the
			// panes null makes `terraform query -generate-config-out` write a
			// customization with no text, LDAP or SSO panes, and applying that
			// config back would delete the ones it has — so hydrate them with
			// the same helper Read uses. Only the panes need fetching; the
			// parent is already in hand.
			state := EnrollmentCustomizationResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(enrollmentCustomizationTimeoutAttributeTypes),
			}
			assignParentToResource(&state, &item)
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			err := hydratePanels(itemCtx, r.client, &state)
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping enrollment customization from generated config after pane read failure", map[string]any{
					"id":    state.ID.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro enrollment customizations", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
	})

	if len(results) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}

// enrollmentCustomizationListItemName is the name accessor passed to
// filters.ApplyClassicFilter. The SDK list endpoint returns
// EnrollmentCustomizationV2 elements with a non-pointer DisplayName field.
func enrollmentCustomizationListItemName(item pro.EnrollmentCustomizationV2) string {
	return item.DisplayName
}
