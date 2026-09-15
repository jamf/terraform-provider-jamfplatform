// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildGroupedGatewayCreateInput converts the Terraform plan into the create
// payload.
func buildGroupedGatewayCreateInput(ctx context.Context, plan GroupedGatewayResourceModel) (*securitycloud.GroupedGatewayCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	gatewayIDs, tenantIDs, memberDiags := membershipFromPlan(ctx, plan)
	diags.Append(memberDiags...)
	if diags.HasError() {
		return nil, diags
	}

	return &securitycloud.GroupedGatewayCreateRequest{
		Name:               plan.Name.ValueString(),
		GatewayIds:         gatewayIDs,
		RoutingStrategy:    wireStrategyFor(plan.RoutingStrategy.ValueString()),
		RecoveryDelayInSec: int(wireStabilityFor(plan.RequiredGatewayStability.ValueString())),
		TenantIds:          tenantIDs,
	}, diags
}

// buildGroupedGatewayPatchInput converts the Terraform plan into the update
// payload.
//
// Every writable field is sent on every update. The endpoint is a merge patch
// where omission preserves, so a subset write would work too — but all five
// fields are Required in the schema, so the plan always carries a complete desired
// state and there is no partial case to model.
func buildGroupedGatewayPatchInput(ctx context.Context, plan GroupedGatewayResourceModel) (*securitycloud.GroupedGatewayPatchRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	gatewayIDs, tenantIDs, memberDiags := membershipFromPlan(ctx, plan)
	diags.Append(memberDiags...)
	if diags.HasError() {
		return nil, diags
	}

	name := plan.Name.ValueString()
	strategy := wireStrategyFor(plan.RoutingStrategy.ValueString())
	recoveryDelay := int(wireStabilityFor(plan.RequiredGatewayStability.ValueString()))

	return &securitycloud.GroupedGatewayPatchRequest{
		Name:               &name,
		GatewayIds:         &gatewayIDs,
		RoutingStrategy:    &strategy,
		RecoveryDelayInSec: &recoveryDelay,
		TenantIds:          &tenantIDs,
	}, diags
}

// membershipFromPlan extracts the member gateway IDs and tenant IDs. The gateway
// IDs keep their configured order — it is the priority order for the
// first-available strategy, and Jamf Security Cloud stores it verbatim.
func membershipFromPlan(ctx context.Context, plan GroupedGatewayResourceModel) ([]string, []string, diag.Diagnostics) {
	var diags diag.Diagnostics

	gatewayIDs := make([]string, 0, len(plan.GatewayIDs.Elements()))
	diags.Append(plan.GatewayIDs.ElementsAs(ctx, &gatewayIDs, false)...)

	tenantIDs := make([]string, 0, len(plan.TenantIDs.Elements()))
	diags.Append(plan.TenantIDs.ElementsAs(ctx, &tenantIDs, false)...)

	return gatewayIDs, tenantIDs, diags
}
