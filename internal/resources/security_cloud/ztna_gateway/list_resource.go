// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

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
	_ list.ListResource              = &GatewayListResource{}
	_ list.ListResourceWithConfigure = &GatewayListResource{}
)

// NewGatewayListResource returns a list resource for Jamf Security Cloud ZTNA
// gateway queries.
func NewGatewayListResource() list.ListResource {
	return &GatewayListResource{}
}

// GatewayListResource implements Terraform query list support for dedicated Jamf
// Security Cloud ZTNA gateways. The gateway list endpoint accepts no query
// parameters, so there is no filter block — the resource returns every gateway on
// the tenant.
//
// Generated configuration is incomplete by construction for an IPsec gateway: the
// pre-shared key is `WriteOnly` and Jamf Security Cloud never returns it, so
// `ipsec.jamf_side.shared_secret` has to be filled in by hand before the config
// will apply.
type GatewayListResource struct {
	client *securitycloud.Client
}

// Metadata sets the list resource type name.
func (r *GatewayListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_gateway"
}

// Configure wires the Jamf Security Cloud client into the list resource via the
// shared providerdata.ConfigureSecurityCloud helper.
func (r *GatewayListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_gateway")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *GatewayListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists every dedicated Jamf Security Cloud ZTNA gateway on the tenant. Jamf Security Cloud " +
			"exposes no query parameters for gateways, so this list resource takes no filter configuration. " +
			"Generated configuration for an IPsec gateway omits the pre-shared key, which Jamf Security Cloud " +
			"never returns." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams gateway identities back to Terraform.
func (r *GatewayListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config GatewayListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	gateways, err := r.client.ListZtnaGatewaysV1(ctx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Security Cloud ZTNA gateways", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(gateways.Results)) {
		maxResults = int64(len(gateways.Results))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, g := range gateways.Results {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = g.Name

		id := types.StringValue(g.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, gatewayIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := GatewayResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(gatewayTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignGatewayResourceModel(ctx, &state, &g)...)
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

	tflog.Debug(ctx, "Listed Jamf Security Cloud ZTNA gateways", map[string]any{
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
