// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// requiresReplaceOnIpsecPresenceChange forces replacement when the `ipsec` block
// appears or disappears, and only then.
//
// Presence of the block is the gateway's form, and the form is immutable:
// wire-probed 2026-08-27, adding `ipsec` to an existing dedicated internet
// gateway is refused with `400 GATEWAY_TYPE_CHANGE_NOT_SUPPORTED`, and patching
// the dedicated-egress-IP flag in either direction returns `204` and then
// silently leaves the value alone — the worse of the two failures, because
// nothing tells the caller the write did not land. Editing fields *inside* an
// existing block is a normal in-place update, so the check is on null-ness only,
// not on the block's contents.
func requiresReplaceOnIpsecPresenceChange(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = req.ConfigValue.IsNull() != req.StateValue.IsNull()
}

// ModifyPlan fills an unset tenant_ids with the provider's own Security Cloud tenant when the
// gateway is being created, so the plan shows the ID the create will send.
//
// Only a create resolves it. On every later plan the attribute's UseStateForUnknown keeps the
// stored value, so removing tenant_ids from the configuration of an existing gateway changes
// nothing rather than re-granting it. The lookup is skipped while the client is unconfigured,
// which leaves the value unknown for Create to resolve.
func (r *GatewayResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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
