// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestFlattenGeneral_RenameMapping verifies wire fields land on the UI-aligned
// attributes and that the configured-value-wins echo guard holds.
func TestFlattenGeneral_RenameMapping(t *testing.T) {
	g := &proclassic.RestrictedSoftwareGeneral{
		ID:                    new(42),
		Name:                  new("Block Chess"),
		ProcessName:           new("Chess.app"),
		MatchExactProcessName: new(true),
		SendNotification:      new(false),
		KillProcess:           new(true),
		DeleteExecutable:      new(false),
		DisplayMessage:        new("blocked"),
		Site:                  &proclassic.SiteObject{ID: new(-1), Name: new("None")},
	}
	// Fresh state (all null) → adopt server values.
	state := &RestrictedSoftwareGeneralModel{}
	flattenGeneral(g, state)

	if state.ID.ValueString() != "42" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Block Chess" || state.ProcessName.ValueString() != "Chess.app" {
		t.Errorf("name/process_name not flattened")
	}
	if !state.RestrictExactProcessName.ValueBool() {
		t.Errorf("restrict_exact_process_name must reflect match_exact_process_name=true")
	}
	if state.SendEmailNotificationOnViolation.ValueBool() {
		t.Errorf("send_email_notification_on_violation must reflect send_notification=false")
	}
	if !state.KillProcess.ValueBool() {
		t.Errorf("kill_process not flattened")
	}
	if state.DeleteApplication.ValueBool() {
		t.Errorf("delete_application must reflect delete_executable=false")
	}
	// Sentinel site (id -1): derived name nulls, not the flaky server echo.
	if state.SiteID.ValueString() != "-1" || !state.SiteName.IsNull() {
		t.Errorf("site not flattened: id=%q name=%q (name should be null on the sentinel)", state.SiteID.ValueString(), state.SiteName.ValueString())
	}
}

// TestFlattenGeneral_ReportsDrift confirms the wire value wins over a divergent
// state value, so a change made outside Terraform is reported as drift. Every
// field on this resource is echoed faithfully by the classic
// /restrictedsoftware GET, display_message included — that is the exact case
// issue #387 was filed on, where the value changed server-side and
// `terraform plan` reported no change indefinitely.
func TestFlattenGeneral_ReportsDrift(t *testing.T) {
	g := &proclassic.RestrictedSoftwareGeneral{
		Name:                  new("x"),
		ProcessName:           new("y"),
		DisplayMessage:        new("server-message"),
		MatchExactProcessName: new(true),
		KillProcess:           new(true),
		DeleteExecutable:      new(true),
		SendNotification:      new(true),
	}
	state := &RestrictedSoftwareGeneralModel{
		DisplayMessage:                   types.StringValue("user-message"),
		RestrictExactProcessName:         types.BoolValue(false),
		KillProcess:                      types.BoolValue(false),
		DeleteApplication:                types.BoolValue(false),
		SendEmailNotificationOnViolation: types.BoolValue(false),
	}
	flattenGeneral(g, state)
	if state.DisplayMessage.ValueString() != "server-message" {
		t.Errorf("display_message: wire value must win, got %q", state.DisplayMessage.ValueString())
	}
	for _, tc := range []struct {
		name string
		got  types.Bool
	}{
		{"restrict_exact_process_name", state.RestrictExactProcessName},
		{"kill_process", state.KillProcess},
		{"delete_application", state.DeleteApplication},
		{"send_email_notification_on_violation", state.SendEmailNotificationOnViolation},
	} {
		if !tc.got.ValueBool() {
			t.Errorf("%s: wire value must win, got false", tc.name)
		}
	}
}

// TestAssignRestrictedSoftwareResourceModel_IncludeUnmanagedHydratesFromScratch
// pins the config-generation contract: with includeUnmanaged set and an empty
// starting model, the wire-present scope is allocated and hydrated from the
// server.
func TestAssignRestrictedSoftwareResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	state := &RestrictedSoftwareResourceModel{}
	src := &proclassic.RestrictedSoftware{
		ID:      new(9),
		General: &proclassic.RestrictedSoftwareGeneral{Name: new("blocked"), ProcessName: new("evil.app")},
		Scope: &proclassic.RestrictedSoftwareScope{
			AllComputers: new(true),
			ComputerGroups: &proclassic.RestrictedSoftwareScopeComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(11)}, {ID: new(22)}},
			},
			Exclusions: &proclassic.RestrictedSoftwareScopeExclusions{
				Users: &proclassic.RestrictedSoftwareScopeExclusionsUsers{
					User: &[]proclassic.IDName{{Name: new("alice")}},
				},
			},
		},
	}
	diags := assignRestrictedSoftwareResourceModel(context.Background(), state, src, true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected scope.targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers hydrated true")
	}
	if got := len(state.Scope.Targets.ComputerGroupIDs.Elements()); got != 2 {
		t.Fatalf("expected 2 computer_group_ids, got %d", got)
	}
	if state.Scope.Exclusions == nil {
		t.Fatal("expected exclusions allocated when wire-present")
	}
	if got := len(state.Scope.Exclusions.DirectoryServiceOrLocalUserNames.Elements()); got != 1 {
		t.Fatalf("expected 1 excluded user, got %d", got)
	}
}

// TestFlattenScope_ManagedRefreshUnmanagedStaysNull pins the granular
// ownership gate: a managed (non-null) category refreshes from the live scope;
// an unmanaged (null) category stays null so members maintained in the admin
// UI never enter state.
func TestFlattenScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	ctx := context.Background()
	s := &proclassic.RestrictedSoftwareScope{
		AllComputers: new(false),
		ComputerGroups: &proclassic.RestrictedSoftwareScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(11), Name: new("All Managed")}},
		},
		Departments: &proclassic.RestrictedSoftwareScopeDepartments{
			Department: &[]proclassic.IDName{{ID: new(3)}},
		},
		Exclusions: &proclassic.RestrictedSoftwareScopeExclusions{
			Users: &proclassic.RestrictedSoftwareScopeExclusionsUsers{
				User: &[]proclassic.IDName{{Name: new("alice")}, {Name: new("bob")}},
			},
			ComputerGroups: &proclassic.RestrictedSoftwareScopeExclusionsComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(7)}},
			},
		},
	}
	state := &RestrictedSoftwareScopeModel{
		Targets: &RestrictedSoftwareScopeTargetsModel{
			AllComputers:     types.BoolValue(true), // managed, drift-refreshes to false
			ComputerGroupIDs: strSet(t, "99"),       // managed, drift-refreshes
			BuildingIDs:      strSet(t),             // managed [], stays empty (never null)
		},
		Exclusions: &RestrictedSoftwareScopeExclusionsModel{
			DirectoryServiceOrLocalUserNames: strSet(t), // managed
		},
	}
	flattenScope(ctx, s, state, false)

	if state.Targets.AllComputers.IsNull() || state.Targets.AllComputers.ValueBool() {
		t.Errorf("managed all_computers should drift-refresh to false, got %v", state.Targets.AllComputers)
	}
	if l := len(state.Targets.ComputerGroupIDs.Elements()); l != 1 {
		t.Errorf("managed computer_group_ids should drift-refresh: got %d members", l)
	}
	if state.Targets.BuildingIDs.IsNull() || len(state.Targets.BuildingIDs.Elements()) != 0 {
		t.Errorf("managed empty building_ids must stay an empty set, got %v", state.Targets.BuildingIDs)
	}
	// Unmanaged categories stay null even though the live scope has members.
	if !state.Targets.DepartmentIDs.IsNull() {
		t.Errorf("unmanaged department_ids must stay null, got %v", state.Targets.DepartmentIDs)
	}
	if !state.Targets.ComputerIDs.IsNull() {
		t.Errorf("unmanaged computer_ids must stay null, got %v", state.Targets.ComputerIDs)
	}
	if l := len(state.Exclusions.DirectoryServiceOrLocalUserNames.Elements()); l != 2 {
		t.Errorf("managed excluded users should refresh: got %d", l)
	}
	if !state.Exclusions.ComputerGroupIDs.IsNull() {
		t.Errorf("unmanaged exclusion computer_group_ids must stay null, got %v", state.Exclusions.ComputerGroupIDs)
	}
}

// TestFlattenScope_HydrateAllForMergeBase pins the includeUnmanaged bypass:
// every wire-present category hydrates into a zero model — the shape Update
// uses to build the read-merge-write base.
func TestFlattenScope_HydrateAllForMergeBase(t *testing.T) {
	ctx := context.Background()
	s := &proclassic.RestrictedSoftwareScope{
		AllComputers: new(false),
		ComputerGroups: &proclassic.RestrictedSoftwareScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(11)}},
		},
		Exclusions: &proclassic.RestrictedSoftwareScopeExclusions{
			Users: &proclassic.RestrictedSoftwareScopeExclusionsUsers{
				User: &[]proclassic.IDName{{Name: new("alice")}},
			},
		},
	}
	state := &RestrictedSoftwareScopeModel{}
	flattenScope(ctx, s, state, true)

	if state.Targets == nil || state.Targets.AllComputers.IsNull() || state.Targets.AllComputers.ValueBool() {
		t.Fatalf("expected all_computers hydrated false, got %+v", state.Targets)
	}
	if l := len(state.Targets.ComputerGroupIDs.Elements()); l != 1 {
		t.Errorf("expected computer_group_ids hydrated, got %d members", l)
	}
	// Wire-absent categories hydrate to empty (never null) under hydrate-all.
	if state.Targets.DepartmentIDs.IsNull() || len(state.Targets.DepartmentIDs.Elements()) != 0 {
		t.Errorf("wire-absent department_ids must hydrate to an empty set, got %v", state.Targets.DepartmentIDs)
	}
	if state.Exclusions == nil {
		t.Fatal("expected exclusions allocated")
	}
	if l := len(state.Exclusions.DirectoryServiceOrLocalUserNames.Elements()); l != 1 {
		t.Errorf("expected excluded users hydrated, got %d", l)
	}
}
