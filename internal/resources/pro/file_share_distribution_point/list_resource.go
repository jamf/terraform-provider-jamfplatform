// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	"context"

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

// fileShareDistributionPointFilterSelectors enumerates the RSQL selectors
// accepted by the distribution points endpoint. The endpoint rejects any other
// field (notably `id`): "Fields that can be used in filtering results of this
// endpoint: [name, serverName]".
var fileShareDistributionPointFilterSelectors = []string{
	"name",
	"serverName",
}

var (
	_ list.ListResource              = &FileShareDistributionPointListResource{}
	_ list.ListResourceWithConfigure = &FileShareDistributionPointListResource{}
)

// NewFileShareDistributionPointListResource returns a list resource.
func NewFileShareDistributionPointListResource() list.ListResource {
	return &FileShareDistributionPointListResource{}
}

// FileShareDistributionPointListResource implements Terraform query list
// support for Jamf Pro file share distribution points.
type FileShareDistributionPointListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *FileShareDistributionPointListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_file_share_distribution_point"
}

// Configure wires the Jamf Pro client into the list resource.
func (r *FileShareDistributionPointListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_file_share_distribution_point")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *FileShareDistributionPointListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf Pro file share distribution points using the same filter clauses as the data source." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(fileShareDistributionPointFilterSelectors),
				fileShareDistributionPointFilterSelectors,
			),
		},
	}
}

// List executes the query and streams distribution point identities back to
// Terraform.
func (r *FileShareDistributionPointListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config FileShareDistributionPointListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(fileShareDistributionPointFilterSelectors))
	tflog.Debug(ctx, "file share distribution point list filters", map[string]any{"filter": filterExpression})

	dps, err := r.client.ListDistributionPointsV1(ctx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro file share distribution points", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(dps)) {
		maxResults = int64(len(dps))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, dp := range dps {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = dp.Name

		id := helpers.StringPointerValueOrNull(dp.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, fileShareDistributionPointIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := FileShareDistributionPointResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(fileShareDistributionPointTimeoutAttributeTypes),
			}
			dpCopy := dp
			assignFileShareDistributionPointResourceModel(&state, &dpCopy)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro file share distribution points", map[string]any{
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
