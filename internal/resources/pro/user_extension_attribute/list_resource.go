// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.ListUserExtensionAttributes   (id+name only)
//   proclassic.GetUserExtensionAttributeByID (per-item hydration on include_resource)
// Status: current. Last reviewed 2026-06-26.

package user_extension_attribute

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation waits on the Classic
// /userextensionattributes endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout caps each per-item GET issued to hydrate a list result,
// decoupled from the overall list budget so one slow record cannot stall the list.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &UserExtensionAttributeListResource{}
var _ list.ListResourceWithConfigure = &UserExtensionAttributeListResource{}

// NewUserExtensionAttributeListResource returns a list resource for user
// extension attribute queries.
func NewUserExtensionAttributeListResource() list.ListResource {
	return &UserExtensionAttributeListResource{}
}

// UserExtensionAttributeListResource implements Terraform query list support.
// The Classic list endpoint returns id + name only, so when `include_resource`
// is requested each item is re-fetched by id to hydrate data_type, input_type,
// description and popup_menu_choices — otherwise generated config would drop
// those Required fields and fail validation. A per-item read failure drops just
// that item (logged), not the whole list.
type UserExtensionAttributeListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *UserExtensionAttributeListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_extension_attribute"
}

// Configure wires the Jamf Pro Classic client into the list resource.
func (r *UserExtensionAttributeListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *UserExtensionAttributeListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro user extension attributes. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams user extension attribute identities back
// to Terraform.
func (r *UserExtensionAttributeListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config UserExtensionAttributeListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	listResp, err := r.client.ListUserExtensionAttributes(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro user extension attributes", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.IDName{}
	if listResp != nil {
		items = listResp.UserExtensionAttributes
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, userExtensionAttributeListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, item := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(item.Name)

		id := helpers.StringValueFromIntPtr(item.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, userExtensionAttributeIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancelItem := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, getErr := r.client.GetUserExtensionAttributeByID(itemCtx, id.ValueString())
			cancelItem()
			if getErr != nil {
				tflog.Warn(ctx, "Skipping user extension attribute hydration after per-item read failed", map[string]any{
					"id":    id.ValueString(),
					"error": getErr.Error(),
				})
				continue
			}

			state := UserExtensionAttributeResourceModel{
				ID:               id,
				PopupMenuChoices: types.ListNull(types.StringType),
				Timeouts:         helpers.NewResourceTimeoutsNullValue(userExtensionAttributeTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignUserExtensionAttributeResourceModel(ctx, &state, got)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro user extension attributes", map[string]any{
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

// userExtensionAttributeListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func userExtensionAttributeListItemName(s proclassic.IDName) string {
	return helpers.DerefString(s.Name)
}
