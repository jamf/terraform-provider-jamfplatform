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
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
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
// inventory preload records endpoint. The list resource schema does not expose a
// user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &InventoryPreloadRecordListResource{}
var _ list.ListResourceWithConfigure = &InventoryPreloadRecordListResource{}

// NewInventoryPreloadRecordListResource returns a list resource for Jamf Pro
// inventory preload record queries.
func NewInventoryPreloadRecordListResource() list.ListResource {
	return &InventoryPreloadRecordListResource{}
}

// InventoryPreloadRecordListResource implements Terraform query list support for
// Jamf Pro inventory preload records.
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

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

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
			state := InventoryPreloadRecordResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(inventoryPreloadRecordTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignInventoryPreloadRecordResourceModel(ctx, &state, rec)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			if state.ID.IsNull() || state.ID.IsUnknown() {
				state.ID = id
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro inventory preload records", map[string]any{
		"filter":   filterExpression,
		"limit":    req.Limit,
		"returned": len(results),
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
