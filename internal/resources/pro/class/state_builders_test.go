// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package class

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func setElems(t *testing.T, s types.Set) []string {
	t.Helper()
	out, d := stringSliceFromSet(context.Background(), s)
	if d.HasError() {
		t.Fatalf("set extract: %v", d)
	}
	return out
}

func TestAssignResourceModel_PopulatedAndResolvedIDs(t *testing.T) {
	c := &proclassic.Class{
		ID:                   new(3),
		Name:                 new("Test"),
		Description:          new("desc"),
		Source:               new("N/A"),
		Site:                 &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		Students:             &proclassic.ClassStudents{Student: new([]string{"kyle@jamf.com"})},
		StudentIds:           &proclassic.ClassStudentIds{ID: new([]int{18})},
		Teachers:             &proclassic.ClassTeachers{Teacher: new([]string{"david@x.co"})},
		TeacherIds:           &proclassic.ClassTeacherIds{ID: new([]int{9})},
		StudentGroupIds:      &proclassic.ClassStudentGroupIds{ID: new([]int{3})},
		MobileDeviceGroupIds: &proclassic.ClassMobileDeviceGroupIds{ID: new([]int{66})},
	}

	state := &ClassResourceModel{
		// Prior config casing must be preserved against the server's canonical echo.
		Students:        strSet(t, "Kyle@JAMF.com"),
		StudentGroupIDs: strSet(t, "3"),
	}
	diags := assignClassResourceModel(context.Background(), state, c)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "3" {
		t.Errorf("id = %q", state.ID.ValueString())
	}
	// Sentinel site (id -1): derived name nulls, not the flaky server echo.
	if state.SiteID.ValueString() != "-1" || !state.SiteName.IsNull() {
		t.Errorf("site = %q / %q (name should be null on the sentinel)", state.SiteID.ValueString(), state.SiteName.ValueString())
	}
	if state.Source.ValueString() != "N/A" {
		t.Errorf("source = %q", state.Source.ValueString())
	}
	// Username casing preserved (canonicalisation drift suppressed).
	if got := setElems(t, state.Students); len(got) != 1 || got[0] != "Kyle@JAMF.com" {
		t.Errorf("students should preserve config casing, got %v", got)
	}
	// Resolved IDs surfaced.
	if got := setElems(t, state.StudentIDs); len(got) != 1 || got[0] != "18" {
		t.Errorf("student_ids = %v", got)
	}
	if got := setElems(t, state.TeacherIDs); len(got) != 1 || got[0] != "9" {
		t.Errorf("teacher_ids = %v", got)
	}
}

func TestAssignResourceModel_PreservesNullVsEmptyShape(t *testing.T) {
	// Server returns no members for any collection.
	c := &proclassic.Class{
		ID:   new(7),
		Name: new("empty"),
		Site: &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
	}

	// students omitted (null) in config; teachers explicitly emptied ([]).
	state := &ClassResourceModel{
		Students: types.SetNull(types.StringType),
		Teachers: strSet(t), // empty set
	}
	diags := assignClassResourceModel(context.Background(), state, c)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !state.Students.IsNull() {
		t.Errorf("null students must stay null, got %v", state.Students)
	}
	if state.Teachers.IsNull() {
		t.Errorf("explicit empty teachers must stay an empty set, not null")
	}
	if len(setElems(t, state.Teachers)) != 0 {
		t.Errorf("teachers must be empty")
	}
	// Computed echoes go null when empty.
	if !state.StudentIDs.IsNull() {
		t.Errorf("empty student_ids must be null")
	}
}

func TestAssignDataSourceModel_NullWhenEmpty(t *testing.T) {
	c := &proclassic.Class{
		ID:       new(3),
		Name:     new("Test"),
		Site:     &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		Students: &proclassic.ClassStudents{Student: new([]string{"a@x.com", "b@x.com"})},
	}
	state := &ClassDataSourceModel{}
	diags := assignClassDataSourceModel(context.Background(), state, c)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got := setElems(t, state.Students); len(got) != 2 {
		t.Errorf("DS students = %v", got)
	}
	if !state.Teachers.IsNull() {
		t.Errorf("DS empty teachers must be null")
	}
}
