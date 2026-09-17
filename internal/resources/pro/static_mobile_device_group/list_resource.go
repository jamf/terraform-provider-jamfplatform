// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps the page read that backs a list query.
const defaultListTimeout = 90 * time.Second

var (
	_ list.ListResource              = &StaticMobileDeviceGroupListResource{}
	_ list.ListResourceWithConfigure = &StaticMobileDeviceGroupListResource{}
)

// StaticMobileDeviceGroupListResource implements Terraform query list support.
//
// One page read hydrates every result, so there is no per-item fetch and nothing
// to drop: the search reports each group's name, description, site and member
// count, and one further request resolves every platform identifier. Membership
// is deliberately left out. Fetching it would cost a request per group, and a
// generated configuration is better off leaving the attribute undeclared, which
// is what an unmanaged membership means.
type StaticMobileDeviceGroupListResource struct {
	client *pro.Client
	pd     *providerdata.Data
}

// NewStaticMobileDeviceGroupListResource returns a list resource for static
// mobile device group queries.
func NewStaticMobileDeviceGroupListResource() list.ListResource {
	return &StaticMobileDeviceGroupListResource{}
}

// Metadata sets the list resource type name, which matches the managed resource
// it lists.
func (r *StaticMobileDeviceGroupListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_mobile_device_group"
}

// Configure wires the Jamf Pro client and provider data into the list resource.
func (r *StaticMobileDeviceGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_mobile_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_mobile_device_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.pd = pd
}

// ListResourceConfigSchema describes the supported list filters.
func (r *StaticMobileDeviceGroupListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches Jamf Pro static mobile device groups using the same filter clauses as the plural data source. Results carry everything except membership, so a generated configuration leaves each group's members alone." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(FilterSelectors),
				FilterSelectors,
			),
		},
	}
}

// List runs the query and streams group identities back to Terraform.
//
// A failed platform identifier lookup is advisory and affects every result
// equally, so its warning rides on the first hydrated result rather than
// repeating once per group.
func (r *StaticMobileDeviceGroupListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config StaticMobileDeviceGroupListResourceModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(FilterSelectors))
	tflog.Debug(ctx, "static mobile device group list filters", map[string]any{"filter": filterExpression})

	groups, err := r.client.ListStaticMobileDeviceGroupsV2(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro static mobile device groups", helpers.APIErrorDetail(err)),
		})
		return
	}

	var bridgeDiags diag.Diagnostics
	byJamfProID := map[string]string{}
	if req.IncludeResource {
		byJamfProID, bridgeDiags = progroups.PlatformIDsByJamfProID(listCtx, r.client, r.pd, deviceType, groupKind)
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(groups)) {
		maxResults = int64(len(groups))
	}

	results := make([]list.ListResult, 0, maxResults)
	bridgeReported := false
	for _, g := range groups {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = g.GroupName

		id := types.StringValue(g.GroupID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, staticMobileDeviceGroupIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := listResultModel(g, byJamfProID)
			if !bridgeReported {
				result.Diagnostics.Append(bridgeDiags...)
				bridgeReported = true
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro static mobile device groups", map[string]any{
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
