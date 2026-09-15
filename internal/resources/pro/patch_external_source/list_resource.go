// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_external_source

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

// defaultListTimeout caps how long the list operation will wait on the classic
// /patchexternalsources endpoint. The list resource schema does not expose a
// user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource asks for full resource state.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &PatchExternalSourceListResource{}
var _ list.ListResourceWithConfigure = &PatchExternalSourceListResource{}

// NewPatchExternalSourceListResource returns a list resource for Jamf Pro patch
// external source queries.
func NewPatchExternalSourceListResource() list.ListResource {
	return &PatchExternalSourceListResource{}
}

// PatchExternalSourceListResource implements Terraform query list support. Classic
// /patchexternalsources accepts no query parameters, so the optional `filter`
// block is applied client-side via filters.ApplyClassicFilter after the full
// list is fetched.
type PatchExternalSourceListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *PatchExternalSourceListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_external_source"
}

// Configure wires the Jamf ProClassic client into the list resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *PatchExternalSourceListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_external_source")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *PatchExternalSourceListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro patch external sources. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams patch external source identities back to
// Terraform.
//
// The classic /patchexternalsources list endpoint returns only id+name per item
// (a flat []IDName). When include_resource is requested we therefore hydrate
// only id and name; the remaining attributes are set null rather than issuing a
// per-item GET against this concurrency-sensitive classic endpoint. A list
// preview does not require full hydration — consumers needing the full object
// should use the singular data source or the managed resource.
func (r *PatchExternalSourceListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config PatchExternalSourceListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListPatchExternalSources(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro patch external sources", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.IDName{}
	if resp != nil {
		items = resp.PatchExternalSources
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, patchExternalSourceName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, s := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(s.Name)

		id := helpers.StringValueFromIntPtr(s.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, patchExternalSourceIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The list endpoint exposes only id+name, and host_name is Required
			// on the resource schema — a list result carrying null for it
			// generates config the provider then refuses to plan. Follow up with
			// a singular GET and hydrate from the state builder Read uses.
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			full, err := r.client.GetPatchExternalSourceByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping patch external source from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := PatchExternalSourceResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(patchExternalSourceTimeoutAttributeTypes),
			}
			assignPatchExternalSourceResourceModel(&state, full)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro patch external sources", map[string]any{
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

// patchExternalSourceName is the name accessor passed to filters.ApplyClassicFilter.
func patchExternalSourceName(s proclassic.IDName) string { return helpers.DerefString(s.Name) }
