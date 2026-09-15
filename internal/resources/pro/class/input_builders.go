// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package class

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildClassInput converts a plan model into the SDK payload used for Create and
// Update. The membership collections are emitted unconditionally: the classic
// API merges omitted fields (leaving the server value unchanged), so to make the
// Terraform config authoritative — and to let users clear members — the full
// representation is always sent. An empty wrapper element (e.g. <students/>)
// clears the corresponding collection.
//
// student_ids / teacher_ids are NOT sent: they are resolved by the server from
// the supplied usernames. source, the device list, and the primary mobile
// device group are server-derived and never sent.
func buildClassInput(ctx context.Context, plan ClassResourceModel) (*proclassic.ClassPost, diag.Diagnostics) {
	var diags diag.Diagnostics

	name := plan.Name.ValueString()

	students, d := stringSliceFromSet(ctx, plan.Students)
	diags.Append(d...)
	teachers, d := stringSliceFromSet(ctx, plan.Teachers)
	diags.Append(d...)
	studentGroups, d := intSliceFromSet(ctx, plan.StudentGroupIDs, "student_group_ids")
	diags.Append(d...)
	teacherGroups, d := intSliceFromSet(ctx, plan.TeacherGroupIDs, "teacher_group_ids")
	diags.Append(d...)
	mdGroups, d := intSliceFromSet(ctx, plan.MobileDeviceGroupIDs, "mobile_device_group_ids")
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	input := &proclassic.ClassPost{
		Name:                 &name,
		Description:          plan.Description.ValueStringPointer(),
		Site:                 scope.BuildSiteObject(plan.SiteID),
		Students:             &proclassic.ClassPostStudents{Student: &students},
		Teachers:             &proclassic.ClassPostTeachers{Teacher: &teachers},
		StudentGroupIds:      &proclassic.ClassPostStudentGroupIds{ID: &studentGroups},
		TeacherGroupIds:      &proclassic.ClassPostTeacherGroupIds{ID: &teacherGroups},
		MobileDeviceGroupIds: &proclassic.ClassPostMobileDeviceGroupIds{ID: &mdGroups},
	}

	return input, diags
}
