// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listskip"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// inventoryPreloadRecordFilterSelectors enumerates the RSQL selectors accepted by the
// inventory preload records endpoint. Per the Jamf Pro API documentation all record
// fields are filterable (extension-attribute-derived fields are not). Selectors keep
// their API-native spelling per the RSQL pass-through exemption in STYLE_GUIDE
// §Schema Guidelines.
var inventoryPreloadRecordFilterSelectors = []string{
	"id",
	"serialNumber",
	"deviceType",
	"username",
	"fullName",
	"emailAddress",
	"phoneNumber",
	"position",
	"department",
	"building",
	"room",
	"poNumber",
	"poDate",
	"warrantyExpiration",
	"leaseExpiration",
	"appleCareId",
	"lifeExpectancy",
	"purchasePrice",
	"purchasingContact",
	"purchasingAccount",
	"barCode1",
	"barCode2",
	"assetTag",
	"vendor",
}

// defaultListTimeout caps how long the list operation will wait on the Jamf Pro
// inventory preload records endpoint. It applies when the query's
// `timeouts.list` is unset.
const defaultListTimeout = 90 * time.Second

// skippedRecordsKind is the plural noun the trailing skip warning uses.
const skippedRecordsKind = "Jamf Pro inventory preload records"

// skippedRecordsHint closes the trailing skip warning.
const skippedRecordsHint = "The other records are unaffected. " +
	"If a record named here still exists, please report it to the provider developers with the diagnostics above."

var _ list.ListResource = &InventoryPreloadRecordListResource{}
var _ list.ListResourceWithConfigure = &InventoryPreloadRecordListResource{}

// NewInventoryPreloadRecordListResource returns a list resource for Jamf Pro
// inventory preload record queries.
func NewInventoryPreloadRecordListResource() list.ListResource {
	return &InventoryPreloadRecordListResource{}
}

// InventoryPreloadRecordListResource implements Terraform query list support for
// Jamf Pro inventory preload records.
//
// A record that cannot be hydrated is left out and named in one trailing warning
// (internal/common/listskip) rather than failing the whole query: one record whose
// extension attributes repeated used to fail every record in the tenant with
// `Duplicate Set Element` (issue #568).
type InventoryPreloadRecordListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *InventoryPreloadRecordListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_inventory_preload_record"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *InventoryPreloadRecordListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_inventory_preload_record")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *InventoryPreloadRecordListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf Pro Inventory Preload records using RSQL filter clauses." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(inventoryPreloadRecordFilterSelectors)+
					" When filtering by `deviceType`, use `0` for Computer and `1` for Mobile Device.",
				inventoryPreloadRecordFilterSelectors,
			),
		},
	}
	resp.Schema.Attributes = listtimeouts.Add(ctx, resp.Schema.Attributes)
}

// List executes the query and streams inventory preload record identities back to Terraform.
func (r *InventoryPreloadRecordListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config InventoryPreloadRecordListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(inventoryPreloadRecordFilterSelectors))
	tflog.Debug(ctx, "inventory preload record list filters", map[string]any{"filter": filterExpression})

	listCtx, cancel := listtimeouts.Bound(ctx, req.Config, &diags, defaultListTimeout)
	defer cancel()
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	items, err := r.client.ListInventoryPreloadRecordsV2(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro inventory preload records", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)
	var skipped listskip.Recorder

	for i := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		rec := &items[i]

		result := req.NewListResult(ctx)
		result.DisplayName = rec.SerialNumber

		id := helpers.StringPointerValueOrNull(rec.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, inventoryPreloadRecordIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			hydrate := hydrateListResult(ctx, &result, rec, id)
			if hydrate.HasError() {
				tflog.Warn(ctx, "Skipping Jamf Pro inventory preload record from generated config after hydration failure", map[string]any{
					"id": id.ValueString(),
				})
				skipped.Add(listskip.Item{ID: id.ValueString(), Name: rec.SerialNumber, Diags: hydrate})
				continue
			}
			result.Diagnostics.Append(hydrate...)
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro inventory preload records", map[string]any{
		"filter":   filterExpression,
		"limit":    req.Limit,
		"returned": len(results),
		"skipped":  skipped.Len(),
	})

	stream.Results = listskip.Stream(results, &skipped, skippedRecordsKind, skippedRecordsHint)
}

// hydrateListResult fills result.Resource from one listed record and returns the
// diagnostics, leaving result.Diagnostics to the caller so that a record which fails
// can be skipped rather than failing the stream. The list endpoint returns full
// records, so no per-item read is needed.
func hydrateListResult(ctx context.Context, result *list.ListResult, rec *pro.InventoryPreloadRecordV2, id types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	state := InventoryPreloadRecordResourceModel{
		Timeouts: helpers.NewResourceTimeoutsNullValue(inventoryPreloadRecordTimeoutAttributeTypes),
	}
	diags.Append(assignInventoryPreloadRecordResourceModel(ctx, &state, rec)...)
	if diags.HasError() {
		return diags
	}
	if state.ID.IsNull() || state.ID.IsUnknown() {
		state.ID = id
	}
	diags.Append(result.Resource.Set(ctx, &state)...)
	return diags
}
