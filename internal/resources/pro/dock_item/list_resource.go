// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

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

// defaultListTimeout caps how long the list operation will wait on the
// classic /dockitems endpoint. The list resource schema does not expose a
// user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &DockItemListResource{}
var _ list.ListResourceWithConfigure = &DockItemListResource{}

// NewDockItemListResource returns a list resource for Jamf Pro dock item queries.
func NewDockItemListResource() list.ListResource {
	return &DockItemListResource{}
}

// DockItemListResource implements Terraform query list support for Jamf Pro
// dock items. Classic /dockitems accepts no query parameters, so the optional
// `filter` block is applied client-side via filters.ApplyClassicFilter after
// the full list is fetched. Unlike /ibeacons, /dockitems returns only id+name
// rows on list — `type`, `path`, and `contents` are Required (type, path) /
// Computed (contents) on the resource schema, so when IncludeResource=true
// we follow up with a per-item GetDockItemByID to populate the full record.
// This makes IncludeResource=true an N+1 operation; identity-only listing
// stays a single round trip.
type DockItemListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *DockItemListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_dock_item"
}

// Configure wires the Jamf ProClassic client into the list resource via the
// shared providerdata.ConfigureProClassic helper.
func (r *DockItemListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_dock_item")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *DockItemListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro dock items. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched. By default each row returns `id` and `name`. Set `include_resource = true` to fetch the full record for each item (`type`, `path`, and the Jamf-computed `contents`)." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams dock item identities back to Terraform.
func (r *DockItemListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config DockItemListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListDockItems(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro dock items", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.IDName{}
	if resp != nil {
		items = resp.DockItems
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, dockItemListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, di := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(di.Name)

		id := helpers.StringValueFromIntPtr(di.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, dockItemIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// /dockitems list response carries only id+name. type and path are
			// Required on the resource schema and contents is server-computed,
			// so we cannot emit a list result with nulls for them — follow up
			// with a singular GET to populate the full record.
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			full, err := r.client.GetDockItemByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping dock item from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := DockItemResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(dockItemTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignDockItemResourceModel(&state, full)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro dock items", map[string]any{
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

// dockItemListItemName is the name accessor passed to filters.ApplyClassicFilter.
func dockItemListItemName(di proclassic.IDName) string {
	return helpers.DerefString(di.Name)
}
