// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package class

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// A class is a scopeable object, but it reaches devices differently from a group:
// its mobile device groups decide which devices are involved, while its students
// and teachers decide who can use them. So the alert counts the device groups —
// the part that determines which devices a class touches — and records the people
// as inputs it cannot turn into a device figure.
//
// That is why this uses the same counting as a deployable object rather than the
// membership reporting used for groups: a class has no single membership count of
// its own to report.

const (
	reasonStudents = "students and teachers decide who can use the devices in this class, not which devices are in it"
)

// classImpactScope converts a class's device group targeting into the shape the
// impact package counts.
func classImpactScope(ctx context.Context, m *ClassResourceModel) impact.Scope {
	b := impact.NewScopeBuilder(ctx, impact.DeviceTypeMobile)
	if m == nil {
		return b.Scope()
	}
	b.ProGroups("mobile_device_group_ids", impact.DeviceTypeMobile, m.MobileDeviceGroupIDs)
	// People are recorded as broadening: they can only bring more devices into
	// play, never fewer than the device groups already account for.
	b.Broadens("student_ids", m.StudentIDs, reasonStudents)
	b.Broadens("teacher_ids", m.TeacherIDs, reasonStudents)
	b.Broadens("student_group_ids", m.StudentGroupIDs, reasonStudents)
	b.Broadens("teacher_group_ids", m.TeacherGroupIDs, reasonStudents)
	return b.Scope()
}

// reportImpact emits the plan-time impact alert for a change to this class's
// device group targeting.
func (r *ClassResource) reportImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	impact.ReportPlan(ctx, req, resp, impact.PlanReport{
		Cache: r.impact,
		Path:  path.Root("mobile_device_group_ids"),
		Kind:  impact.Scopeable,
		Label: "class",
	}, classImpactScope)
}

// ModifyPlan emits the impact alert for a class's device group targeting. The
// class resource has no other plan-time work, so this is the whole of it.
func (r *ClassResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
	r.reportImpact(ctx, req, resp)
}
