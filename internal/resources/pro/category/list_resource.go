// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package category

import (
	"context"

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

var _ list.ListResource = &CategoryListResource{}
var _ list.ListResourceWithConfigure = &CategoryListResource{}

// NewCategoryListResource returns a list resource for Jamf Pro category queries.
func NewCategoryListResource() list.ListResource {
	return &CategoryListResource{}
}

// CategoryListResource implements Terraform query list support for Jamf Pro categories.
type CategoryListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *CategoryListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_category"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *CategoryListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_category")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *CategoryListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf Pro categories using the same filter clauses as the categories data source." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(CategoryFilterSelectors),
				CategoryFilterSelectors,
			),
		},
	}
}

// List executes the query and streams category identities back to Terraform.
func (r *CategoryListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config CategoryListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(CategoryFilterSelectors))
	tflog.Debug(ctx, "category list filters", map[string]any{"filter": filterExpression})

	cats, err := r.client.ListCategoriesV1(ctx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro categories", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(cats)) {
		maxResults = int64(len(cats))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, c := range cats {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = c.Name

		id := helpers.StringPointerValueOrNull(c.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, categoryIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := CategoryResourceModel{
				ID:       id,
				Name:     types.StringValue(c.Name),
				Priority: types.Int64Value(int64(c.Priority)),
				Timeouts: helpers.NewResourceTimeoutsNullValue(categoryTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro categories", map[string]any{
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
