// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// A compliance benchmark targets device groups by their Platform identifier, the
// same way a blueprint does, and can reach either estate — so its figure is
// measured against the whole managed estate.
func benchmarkImpactScope(ctx context.Context, m *BenchmarkResourceModel) impact.Scope {
	b := impact.NewScopeBuilder(ctx, impact.DeviceTypeAny)
	if m == nil {
		return b.Scope()
	}
	b.PlatformGroups("target_device_groups", m.TargetDeviceGroups)
	return b.Scope()
}

// reportScopeImpact emits the plan-time impact alert for a change to this
// benchmark's device group targeting.
func (r *BenchmarkResource) reportScopeImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	impact.ReportPlan(ctx, req, resp, impact.PlanReport{
		Cache: r.impact,
		Path:  path.Root("target_device_groups"),
		Label: "compliance benchmark",
	}, benchmarkImpactScope)
}

// ModifyPlan reports a title another benchmark already holds, and emits the impact
// alert for a change to device group targeting.
func (r *BenchmarkResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportTitleCollision(ctx, req, resp)
	r.reportScopeImpact(ctx, req, resp)
}
