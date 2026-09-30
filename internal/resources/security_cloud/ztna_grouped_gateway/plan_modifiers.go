// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ModifyPlan fills an unset tenant_ids with the provider's own Security Cloud tenant when the
// grouped gateway is being created, so the plan shows the ID the create will send.
//
// Only a create resolves it. On every later plan the attribute's UseStateForUnknown keeps the
// stored value, so removing tenant_ids from the configuration of an existing grouped gateway changes
// nothing rather than re-granting it. The lookup is skipped while the client is unconfigured,
// which leaves the value unknown for Create to resolve.
func (r *GroupedGatewayResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || !req.State.Raw.IsNull() || r.client == nil {
		return
	}
	var planned types.Set
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("tenant_ids"), &planned)...)
	if resp.Diagnostics.HasError() || !planned.IsUnknown() {
		return
	}
	var configured types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("tenant_ids"), &configured)...)
	if resp.Diagnostics.HasError() || !configured.IsNull() {
		return
	}
	tenantIDs, diags := r.resolveTenantIDs(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tenant_ids"), tenantIDs)...)
}
