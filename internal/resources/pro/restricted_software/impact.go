// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Restricted software carries a limited computer scope: targets and exclusions
// only, with no limitations block and no iBeacon or user-group categories. The
// omitted categories are simply left unset, which the shared builder treats as
// absent and reports nothing for.
func restrictedSoftwareImpactScope(ctx context.Context, m *RestrictedSoftwareResourceModel) impact.Scope {
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
	if e := m.Scope.Exclusions; e != nil {
		in.ExcludeDeviceIDs = e.ComputerIDs
		in.ExcludeGroupIDs = e.ComputerGroupIDs
		in.ExcludeBuildingIDs = e.BuildingIDs
		in.ExcludeDepartmentIDs = e.DepartmentIDs
		in.ExcludeUserNames = e.DirectoryServiceOrLocalUserNames
	}
	return scope.BuildImpactScope(ctx, in)
}

// reportScopeImpact emits the plan-time impact alert for a scope change. A restricted software record
// is a deployable object in Jamf Pro's terms, so the alert reports how many
// devices the change reaches.
func (r *RestrictedSoftwareResource) reportScopeImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	impact.ReportPlan(ctx, req, resp, impact.PlanReport{
		Cache: r.impact,
		Path:  path.Root("scope"),
		Label: "restricted software record",
	}, restrictedSoftwareImpactScope)
}

// ModifyPlan emits the impact alert for a scope change and the name collision check.
func (r *RestrictedSoftwareResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportScopeImpact(ctx, req, resp)
	r.reportNameCollision(ctx, req, resp)
}
