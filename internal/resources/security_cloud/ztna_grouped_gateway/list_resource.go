// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var (
	_ list.ListResource              = &GroupedGatewayListResource{}
	_ list.ListResourceWithConfigure = &GroupedGatewayListResource{}
)

// NewGroupedGatewayListResource returns a list resource for Jamf Security Cloud
// ZTNA grouped gateway queries.
func NewGroupedGatewayListResource() list.ListResource {
	return &GroupedGatewayListResource{}
}

// GroupedGatewayListResource implements Terraform query list support for Jamf
// Security Cloud ZTNA grouped gateways. The endpoint accepts no query parameters,
// so there is no filter block — the resource returns every group on the tenant.
type GroupedGatewayListResource struct {
	client *securitycloud.Client
}

// Metadata sets the list resource type name.
func (r *GroupedGatewayListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_grouped_gateway"
}

// Configure wires the Jamf Security Cloud client into the list resource via the
// shared providerdata.ConfigureSecurityCloud helper.
func (r *GroupedGatewayListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_grouped_gateway")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *GroupedGatewayListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists every Jamf Security Cloud ZTNA grouped gateway on the tenant. Jamf Security Cloud " +
			"exposes no query parameters for grouped gateways, so this list resource takes no filter " +
			"configuration." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams grouped gateway identities back to
// Terraform.
func (r *GroupedGatewayListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config GroupedGatewayListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	groups, err := r.client.ListZtnaGroupedGatewaysV1(ctx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Security Cloud ZTNA grouped gateways", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(groups.Results)) {
		maxResults = int64(len(groups.Results))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, g := range groups.Results {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = g.Name

		id := types.StringValue(g.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, groupedGatewayIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := GroupedGatewayResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(groupedGatewayTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignGroupedGatewayResourceModel(ctx, &state, &g)...)
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

	tflog.Debug(ctx, "Listed Jamf Security Cloud ZTNA grouped gateways", map[string]any{
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
