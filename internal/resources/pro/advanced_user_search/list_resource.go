// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

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
// /advancedusersearches endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource asks for full resource state.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &AdvancedUserSearchListResource{}
var _ list.ListResourceWithConfigure = &AdvancedUserSearchListResource{}

// NewAdvancedUserSearchListResource returns a list resource for advanced user
// search queries.
func NewAdvancedUserSearchListResource() list.ListResource {
	return &AdvancedUserSearchListResource{}
}

// AdvancedUserSearchListResource implements Terraform query list support.
// Classic /advancedusersearches has no RSQL — the optional `filter` block is
// applied client-side via filters.ApplyClassicFilter after the full list is
// fetched. List items carry only id and name on the wire, so IncludeResource
// hydrates each record with a singular GET.
type AdvancedUserSearchListResource struct {
	client *proclassic.Client
}

// AdvancedUserSearchListResourceModel is the config model for list queries.
type AdvancedUserSearchListResourceModel struct {
	Filter *filters.ClassicFilterModel `tfsdk:"filter"`
}

// Metadata sets the list resource type name.
func (r *AdvancedUserSearchListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_advanced_user_search"
}

// Configure wires the Jamf ProClassic client into the list resource.
func (r *AdvancedUserSearchListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_advanced_user_search")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *AdvancedUserSearchListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro advanced user searches. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams advanced user search identities back to
// Terraform.
func (r *AdvancedUserSearchListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config AdvancedUserSearchListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListAdvancedUserSearches(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro advanced user searches", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.IDName{}
	if resp != nil {
		items = resp.AdvancedUserSearches
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, advancedUserSearchListItemName)

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
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, advancedUserSearchIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The /advancedusersearches list response carries id and name only. A list
			// result with null criteria makes `terraform query
			// -generate-config-out` write a search with no criteria at all, and
			// applying that config back would delete the ones the search
			// actually has — so follow up with a singular GET and hydrate from
			// the same state builder Read uses.
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			full, err := r.client.GetAdvancedUserSearchByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping advanced user search from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := AdvancedUserSearchResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(advancedUserSearchTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignAdvancedUserSearchResourceModel(itemCtx, &state, full)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro advanced user searches", map[string]any{
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

// advancedUserSearchListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func advancedUserSearchListItemName(s proclassic.IDName) string {
	return helpers.DerefString(s.Name)
}
