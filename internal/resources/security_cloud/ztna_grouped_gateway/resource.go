// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ztna_grouped_gateway implements the
// jamfplatform_security_cloud_ztna_grouped_gateway resource, data sources and
// list resource backed by the Jamf Security Cloud ZTNA grouped-gateway API.
//
// A grouped gateway is a routing and failover group over two or more of the
// tenant's own dedicated gateways. It is referenced wherever a single gateway
// would be — a custom DNS zone name server accepts a grouped gateway's ID as
// readily as a dedicated one (wire-verified 2026-08-27).
package ztna_grouped_gateway

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minMemberGateways is the smallest group Jamf Security Cloud accepts. Wire-probed
// 2026-08-27: one member is refused with `400 gatewayIds: size must be greater
// than or equal to 2`, on create and on update alike.
const minMemberGateways = 2

// GroupedGatewayResource implements the Terraform resource for Jamf Security Cloud
// ZTNA grouped gateways.
type GroupedGatewayResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                = &GroupedGatewayResource{}
	_ resource.ResourceWithImportState = &GroupedGatewayResource{}
	_ resource.ResourceWithIdentity    = &GroupedGatewayResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewGroupedGatewayResource returns a new instance of GroupedGatewayResource.
func NewGroupedGatewayResource() resource.Resource {
	return &GroupedGatewayResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *GroupedGatewayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_grouped_gateway"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *GroupedGatewayResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Grouped gateway ID used to uniquely reference the group.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the ZTNA grouped gateway resource.
func (r *GroupedGatewayResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Security Cloud ZTNA grouped gateway, a routing and failover group " +
			"over two or more of your dedicated gateways. A grouped gateway can be referenced anywhere a single " +
			"gateway can, including a custom DNS zone's name servers.\n\nEvery member must be one of your own " +
			"dedicated gateways (Jamf Security Cloud's shared gateways are refused), and all members must be the " +
			"same form, either all IPsec or all internet. Deleting a member gateway while it " +
			"is still in a group is refused." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Grouped gateway ID assigned by Jamf Security Cloud.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Name\"** in the Jamf Security Cloud admin UI.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"gateway_ids": schema.ListAttribute{
				MarkdownDescription: "**\"Choose your gateways\"** in the Jamf Security Cloud admin UI: the IDs " +
					"of the member gateways, at least two. Order is significant. It is the priority order the " +
					"`First available` strategy walks, and the admin UI presents it as a drag-to-reorder list. " +
					"Jamf Security Cloud stores the order exactly as given.\n\nMembers must be your own " +
					"dedicated gateways, all of the same form, so mixing an IPsec gateway with an internet one " +
					"is refused, and so is naming one of Jamf Security Cloud's shared gateways.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(minMemberGateways),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"routing_strategy": schema.StringAttribute{
				MarkdownDescription: "**\"Routing strategy\"** in the Jamf Security Cloud admin UI. Which member " +
					"a device uses:\n\n" +
					"- `Nearest`: the geographically closest available member.\n" +
					"- `Random`: a random available member, for load balancing.\n" +
					"- `First available`: the first available member in `gateway_ids` order, failing over to the " +
					"next and back again after `required_gateway_stability`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(routingStrategyValues()...),
				},
			},
			"required_gateway_stability": schema.StringAttribute{
				MarkdownDescription: "**\"Required gateway stability\"** in the Jamf Security Cloud admin UI: how " +
					"long a recovered member must stay healthy before traffic returns to it. Applies to the " +
					"`First available` strategy, and is required whatever the strategy: Jamf Security Cloud " +
					"rejects a create without it even when the value is ignored. Valid values: " +
					markdownList(gatewayStabilityValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(gatewayStabilityValues()...),
				},
			},
			"tenant_ids": schema.SetAttribute{
				MarkdownDescription: "IDs of the tenants granted access to this grouped gateway. At least one, and " +
					"every one must belong to the same organization as the credentials the provider is configured " +
					"with.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the grouped gateway was created. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *GroupedGatewayResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_grouped_gateway")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Security Cloud grouped gateway ID.
func (r *GroupedGatewayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
