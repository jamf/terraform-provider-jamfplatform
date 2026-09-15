// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

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

// defaultListTimeout caps how long the list operation will wait on the Jamf Pro
// buildings endpoint. The list resource schema does not expose a user-overridable
// timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &BuildingListResource{}
var _ list.ListResourceWithConfigure = &BuildingListResource{}

// NewBuildingListResource returns a list resource for Jamf Pro building queries.
func NewBuildingListResource() list.ListResource {
	return &BuildingListResource{}
}

// BuildingListResource implements Terraform query list support for Jamf Pro buildings.
type BuildingListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *BuildingListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_building"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *BuildingListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_building")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *BuildingListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf Pro buildings using the same filter clauses as the buildings data source." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(BuildingFilterSelectors),
				BuildingFilterSelectors,
			),
		},
	}
}

// List executes the query and streams building identities back to Terraform.
func (r *BuildingListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config BuildingListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(BuildingFilterSelectors))
	tflog.Debug(ctx, "building list filters", map[string]any{"filter": filterExpression})

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListBuildingsV1(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro buildings", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, b := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = b.Name

		id := helpers.StringPointerValueOrNull(b.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, buildingIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := BuildingResourceModel{
				ID:             id,
				Name:           types.StringValue(b.Name),
				City:           helpers.StringPointerValueOrNull(b.City),
				Country:        helpers.StringPointerValueOrNull(b.Country),
				StateProvince:  helpers.StringPointerValueOrNull(b.StateProvince),
				StreetAddress1: helpers.StringPointerValueOrNull(b.StreetAddress1),
				StreetAddress2: helpers.StringPointerValueOrNull(b.StreetAddress2),
				ZipPostalCode:  helpers.StringPointerValueOrNull(b.ZipPostalCode),
				Timeouts:       helpers.NewResourceTimeoutsNullValue(buildingTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro buildings", map[string]any{
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
