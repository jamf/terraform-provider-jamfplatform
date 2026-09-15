// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation waits on the classic
// /licensedsoftware endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource asks for full resource state.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &LicensedSoftwareListResource{}
var _ list.ListResourceWithConfigure = &LicensedSoftwareListResource{}

// NewLicensedSoftwareListResource returns a list resource for Jamf Pro licensed
// software queries.
func NewLicensedSoftwareListResource() list.ListResource {
	return &LicensedSoftwareListResource{}
}

// LicensedSoftwareListResource implements Terraform query list support for Jamf
// Pro licensed software records. Classic /licensedsoftware has no RSQL — the
// optional `filter` block is applied client-side via filters.ApplyClassicFilter.
// The list endpoint returns only id + name per record; identity-only is the
// canonical list output, and full detail requires a per-record read.
type LicensedSoftwareListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *LicensedSoftwareListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_licensed_software"
}

// Configure wires the Jamf ProClassic client into the list resource.
func (r *LicensedSoftwareListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_licensed_software")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *LicensedSoftwareListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro licensed software records. Supply an optional case-insensitive `name_substring` filter applied locally after the full list is fetched. List entries surface as identity-only (id and display name); full detail requires a per-record read." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams record identities back to Terraform.
func (r *LicensedSoftwareListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config LicensedSoftwareListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListLicensedSoftware(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro licensed software", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.LicensedSoftwareAllItemLicensedSoftware{}
	if resp != nil {
		items = resp.LicensedSoftwares
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, licensedSoftwareListItemName)

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
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, licensedSoftwareIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The classic /licensedsoftware list response carries id and name
			// only. A list result with null software_definitions and licenses
			// makes `terraform query -generate-config-out` write a title with
			// neither, and applying that config back would strip both — so
			// follow up with a singular GET and hydrate from the same state
			// builder Read uses. firstHydration is true here for the same
			// reason it is on an import: the incoming model is unpopulated, so
			// the wire-present optional lists are the ones to adopt.
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			full, err := r.client.GetLicensedSoftwareByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping licensed software from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := LicensedSoftwareResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(licensedSoftwareTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignLicensedSoftwareResourceModel(itemCtx, &state, full, true)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro licensed software", map[string]any{
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
