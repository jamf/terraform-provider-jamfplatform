// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignGroupedGatewayResourceModel populates a resource model from a
// GroupedGateway response.
//
// The wire also carries an `updatedAt` timestamp, which is deliberately not
// exposed: it advances on every write and would report the object as changed
// outside Terraform on refreshes that found nothing an operator could act on.
// `createdAt` is immutable, so it stays.
func assignGroupedGatewayResourceModel(ctx context.Context, state *GroupedGatewayResourceModel, g *securitycloud.GroupedGateway) diag.Diagnostics {
	var diags diag.Diagnostics

	if g.ID != "" {
		state.ID = types.StringValue(g.ID)
	}
	state.Name = types.StringValue(g.Name)
	state.RoutingStrategy = types.StringValue(labelForStrategy(g.RoutingStrategy))
	state.RequiredGatewayStability = types.StringValue(labelForStability(int64(g.RecoveryDelayInSec)))
	state.CreatedAt = types.StringValue(g.CreatedAt.Format(time.RFC3339))

	gatewayIDs, gatewayDiags := types.ListValueFrom(ctx, types.StringType, g.GatewayIds)
	diags.Append(gatewayDiags...)
	state.GatewayIDs = gatewayIDs

	tenantIDs, tenantDiags := types.SetValueFrom(ctx, types.StringType, g.TenantIds)
	diags.Append(tenantDiags...)
	state.TenantIDs = tenantIDs

	return diags
}

// assignGroupedGatewayDataSourceModel populates the singular data source model
// from a GroupedGateway response.
func assignGroupedGatewayDataSourceModel(ctx context.Context, state *GroupedGatewayDataSourceModel, g *securitycloud.GroupedGateway) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(g.ID)
	state.Name = types.StringValue(g.Name)
	state.RoutingStrategy = types.StringValue(labelForStrategy(g.RoutingStrategy))
	state.RequiredGatewayStability = types.StringValue(labelForStability(int64(g.RecoveryDelayInSec)))
	state.CreatedAt = types.StringValue(g.CreatedAt.Format(time.RFC3339))

	gatewayIDs, gatewayDiags := types.ListValueFrom(ctx, types.StringType, g.GatewayIds)
	diags.Append(gatewayDiags...)
	state.GatewayIDs = gatewayIDs

	tenantIDs, tenantDiags := types.ListValueFrom(ctx, types.StringType, g.TenantIds)
	diags.Append(tenantDiags...)
	state.TenantIDs = tenantIDs

	return diags
}

// buildGroupedGatewaysResultModel maps one GroupedGateway response into a plural
// data source result element.
func buildGroupedGatewaysResultModel(ctx context.Context, g securitycloud.GroupedGateway) (GroupedGatewaysDataSourceResultModel, diag.Diagnostics) {
	var ds GroupedGatewayDataSourceModel
	diags := assignGroupedGatewayDataSourceModel(ctx, &ds, &g)
	return GroupedGatewaysDataSourceResultModel{
		ID:                       ds.ID,
		Name:                     ds.Name,
		GatewayIDs:               ds.GatewayIDs,
		RoutingStrategy:          ds.RoutingStrategy,
		RequiredGatewayStability: ds.RequiredGatewayStability,
		TenantIDs:                ds.TenantIDs,
		CreatedAt:                ds.CreatedAt,
	}, diags
}
