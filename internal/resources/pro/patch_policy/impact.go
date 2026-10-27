// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// A patch policy carries a limited computer scope: no user-based categories,
// because the classic endpoint never returns user-based patch-policy scope even
// when it is set. The categories it does not model are left unset and reported
// as nothing.
func patchPolicyImpactScope(ctx context.Context, m *PatchPolicyResourceModel) impact.Scope {
	if m == nil || m.Scope == nil {
		return impact.Scope{DeviceType: impact.DeviceTypeComputer}
	}
	in := scope.ImpactInputs{
		DeviceType: impact.DeviceTypeComputer,
		DeviceAttr: "computer_ids",
		GroupAttr:  "computer_group_ids",
	}
	if t := m.Scope.Targets; t != nil {
		in.All = t.AllComputers
		in.DeviceIDs = t.ComputerIDs
		in.GroupIDs = t.ComputerGroupIDs
		in.BuildingIDs = t.BuildingIDs
		in.DepartmentIDs = t.DepartmentIDs
	}
	if l := m.Scope.Limitations; l != nil {
		in.LimitNetworkSegmentIDs = l.NetworkSegmentIDs
		in.LimitIbeaconIDs = l.IbeaconIDs
	}
	if e := m.Scope.Exclusions; e != nil {
		in.ExcludeDeviceIDs = e.ComputerIDs
		in.ExcludeGroupIDs = e.ComputerGroupIDs
		in.ExcludeBuildingIDs = e.BuildingIDs
		in.ExcludeDepartmentIDs = e.DepartmentIDs
		in.ExcludeNetworkSegmentIDs = e.NetworkSegmentIDs
		in.ExcludeIbeaconIDs = e.IbeaconIDs
	}
	return scope.BuildImpactScope(ctx, in)
}

// reportScopeImpact emits the plan-time impact alert for a scope change. A patch policy
// is a deployable object in Jamf Pro's terms, so the alert reports how many
// devices the change reaches.
func (r *PatchPolicyResource) reportScopeImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	impact.ReportPlan(ctx, req, resp, impact.PlanReport{
		Cache: r.impact,
		Path:  path.Root("scope"),
		Label: "patch policy",
	}, patchPolicyImpactScope)
}

// ModifyPlan emits the impact alert for a scope change. This resource has no
// other plan-time work, so this is the whole of it.
func (r *PatchPolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
	r.reportScopeImpact(ctx, req, resp)
}
