// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// GroupedGatewayResourceModel represents the Terraform resource model for a Jamf
// Security Cloud ZTNA grouped gateway.
type GroupedGatewayResourceModel struct {
	ID                       types.String           `tfsdk:"id"`
	Name                     types.String           `tfsdk:"name"`
	GatewayIDs               types.List             `tfsdk:"gateway_ids"`
	RoutingStrategy          types.String           `tfsdk:"routing_strategy"`
	RequiredGatewayStability types.String           `tfsdk:"required_gateway_stability"`
	TenantIDs                types.Set              `tfsdk:"tenant_ids"`
	CreatedAt                types.String           `tfsdk:"created_at"`
	Timeouts                 resourceTimeouts.Value `tfsdk:"timeouts"`
}

// groupedGatewayIdentityModel represents the identity object for grouped gateway
// resources and list results.
type groupedGatewayIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// GroupedGatewayDataSourceModel represents the Terraform data source model for a
// single grouped gateway.
type GroupedGatewayDataSourceModel struct {
	ID                       types.String             `tfsdk:"id"`
	Name                     types.String             `tfsdk:"name"`
	GatewayIDs               types.List               `tfsdk:"gateway_ids"`
	RoutingStrategy          types.String             `tfsdk:"routing_strategy"`
	RequiredGatewayStability types.String             `tfsdk:"required_gateway_stability"`
	TenantIDs                types.List               `tfsdk:"tenant_ids"`
	CreatedAt                types.String             `tfsdk:"created_at"`
	Timeouts                 datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// GroupedGatewaysDataSourceModel represents the Terraform data source model for
// the plural grouped gateway lookup.
type GroupedGatewaysDataSourceModel struct {
	ID              types.String                           `tfsdk:"id"`
	GroupedGateways []GroupedGatewaysDataSourceResultModel `tfsdk:"grouped_gateways"`
	Timeouts        datasourceTimeouts.Value               `tfsdk:"timeouts"`
}

// GroupedGatewaysDataSourceResultModel represents a single grouped gateway in the
// plural data source results.
type GroupedGatewaysDataSourceResultModel struct {
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	GatewayIDs               types.List   `tfsdk:"gateway_ids"`
	RoutingStrategy          types.String `tfsdk:"routing_strategy"`
	RequiredGatewayStability types.String `tfsdk:"required_gateway_stability"`
	TenantIDs                types.List   `tfsdk:"tenant_ids"`
	CreatedAt                types.String `tfsdk:"created_at"`
}

// GroupedGatewayListResourceModel represents the config model for grouped gateway
// list queries. Jamf Security Cloud exposes no query parameters on the endpoint,
// so the model carries only the `timeouts` attribute.
type GroupedGatewayListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
