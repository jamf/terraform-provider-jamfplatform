// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

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

var _ list.ListResource = &PackageListResource{}
var _ list.ListResourceWithConfigure = &PackageListResource{}

// NewPackageListResource returns a list resource for Jamf Pro package queries.
func NewPackageListResource() list.ListResource {
	return &PackageListResource{}
}

// PackageListResource implements Terraform query list support for Jamf Pro
// packages. The /v1/packages list endpoint accepts a restricted RSQL filter
// — see PackageFilterSelectors for the whitelist (any unsupported selector
// returns a 400 from the server, so the schema enforces the whitelist via
// stringvalidator.OneOf).
type PackageListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *PackageListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_package"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *PackageListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_package")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters. Selector
// whitelist mirrors §13.1 — the server rejects everything else with 400.
func (r *PackageListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches Jamf Pro packages using an RSQL filter. Jamf Pro limits which selectors are supported; see the `selector` attribute for the allowed values." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(PackageFilterSelectors),
				PackageFilterSelectors,
			),
		},
	}
}

// List executes the query and streams package identities back to Terraform.
func (r *PackageListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config PackageListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(PackageFilterSelectors))
	tflog.Debug(ctx, "package list filters", map[string]any{"filter": filterExpression})

	pkgs, err := r.client.ListPackagesV1(ctx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro packages", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(pkgs)) {
		maxResults = int64(len(pkgs))
	}

	results := make([]list.ListResult, 0, maxResults)

	for i := range pkgs {
		if int64(len(results)) >= maxResults {
			break
		}
		p := &pkgs[i]

		result := req.NewListResult(ctx)
		result.DisplayName = p.PackageName

		id := helpers.StringPointerValueOrNull(p.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, packageIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := PackageResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(packageTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignPackageResourceModel(&state, p)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro packages", map[string]any{
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
