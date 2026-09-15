// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAssignResourceModel_GeneralSplit verifies the writable/server-derived
// split: writable fields land on their attributes (configured-value-wins echo
// guard), and the server-derived fields (release_date / incremental_update /
// reboot / minimum_os) are adopted verbatim.
func TestAssignResourceModel_GeneralSplit(t *testing.T) {
	p := &proclassic.PatchPolicy{
		ID:                           new(7),
		SoftwareTitleConfigurationID: new(42),
		General: &proclassic.PatchPolicyGeneral{
			Name:               new("8x8 Work latest"),
			TargetVersion:      new("8.33.2.2"),
			Enabled:            new(true),
			DistributionMethod: new("selfservice"),
			AllowDowngrade:     new(false),
			PatchUnknown:       new(true),
			// Server-derived.
			ReleaseDate:       new(1700000000000),
			IncrementalUpdate: new(true),
			Reboot:            new(true),
			MinimumOs:         new("12.0"),
		},
	}

	state := &PatchPolicyResourceModel{}
	diags := assignPatchPolicyResourceModel(context.Background(), state, p, false)
	if diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}

	if state.ID.ValueString() != "7" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.SoftwareTitleConfigurationID.ValueString() != "42" {
		t.Errorf("software_title_configuration_id: got %q", state.SoftwareTitleConfigurationID.ValueString())
	}
	if state.Name.ValueString() != "8x8 Work latest" || state.TargetVersion.ValueString() != "8.33.2.2" {
		t.Errorf("name/target_version not flattened")
	}
	if !state.Enabled.ValueBool() || state.DistributionMethod.ValueString() != "selfservice" {
		t.Errorf("enabled/distribution_method not flattened")
	}
	if !state.PatchUnknown.ValueBool() {
		t.Errorf("patch_unknown not flattened")
	}
	// Server-derived adopted verbatim.
	if state.ReleaseDate.ValueInt64() != 1700000000000 {
		t.Errorf("release_date: got %d", state.ReleaseDate.ValueInt64())
	}
	if !state.IncrementalUpdate.ValueBool() {
		t.Errorf("incremental_update not flattened")
	}
	if !state.Reboot.ValueBool() {
		t.Errorf("reboot not flattened")
	}
	if state.MinimumOS.ValueString() != "12.0" {
		t.Errorf("minimum_os: got %q", state.MinimumOS.ValueString())
	}
}

// TestFlattenKillApps reads the server-derived kill_apps list.
func TestFlattenKillApps(t *testing.T) {
	ka := &proclassic.PatchPolicyGeneralKillApps{
		KillApp: &[]proclassic.PatchPolicyGeneralKillAppsKillAppItem{
			{KillAppName: new("Jamf Composer.app"), KillAppBundleID: new("com.jamfsoftware.Composer")},
		},
	}
	list, diags := flattenKillApps(context.Background(), ka)
	if diags.HasError() {
		t.Fatalf("flatten diags: %v", diags)
	}
	if list.IsNull() {
		t.Fatal("kill_apps must not be null when present")
	}
	elems := list.Elements()
	if len(elems) != 1 {
		t.Fatalf("expected 1 kill_app, got %d", len(elems))
	}
	obj := elems[0].(types.Object)
	attrs := obj.Attributes()
	if attrs["kill_app_name"].(types.String).ValueString() != "Jamf Composer.app" {
		t.Errorf("kill_app_name not flattened")
	}
	if attrs["kill_app_bundle_id"].(types.String).ValueString() != "com.jamfsoftware.Composer" {
		t.Errorf("kill_app_bundle_id not flattened")
	}

	// Absent → null list.
	empty, _ := flattenKillApps(context.Background(), nil)
	if !empty.IsNull() {
		t.Errorf("nil kill_apps must flatten to a null list")
	}
}

// TestFlattenScope_EntityByID confirms managed scope categories read back by ID
// (id-only sets — the server-augmented names/UDIDs are discarded), while
// categories the caller did not declare stay null under the granular ownership
// gate even though the live scope has members.
func TestFlattenScope_EntityByID(t *testing.T) {
	p := &proclassic.PatchPolicy{
		ID:      new(1),
		General: &proclassic.PatchPolicyGeneral{Name: new("p"), TargetVersion: new("1.0")},
		Scope: &proclassic.PatchPolicyScope{
			Computers: &proclassic.PatchPolicyScopeComputers{
				Computer: &[]proclassic.PatchPolicyScopeComputersComputerItem{
					{ID: new(10), Name: new("Mac-10"), UDID: new("udid-10")},
				},
			},
			ComputerGroups: &proclassic.PatchPolicyScopeComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(20), Name: new("Group-20")}},
			},
			Buildings: &proclassic.PatchPolicyScopeBuildings{
				Building: &[]proclassic.IDName{{ID: new(30), Name: new("HQ")}},
			},
			Limitations: &proclassic.PatchPolicyScopeLimitations{
				NetworkSegments: &proclassic.PatchPolicyScopeLimitationsNetworkSegments{
					NetworkSegment: &[]proclassic.IDName{{ID: new(50), Name: new("Net-50")}},
				},
			},
			Exclusions: &proclassic.PatchPolicyScopeExclusions{
				Ibeacons: &proclassic.PatchPolicyScopeExclusionsIbeacons{
					Ibeacon: &[]proclassic.IDName{{ID: new(75), Name: new("Beacon-75")}},
				},
			},
		},
	}

	// Caller manages the specific categories below (non-null); building_ids and
	// the exclusion computer categories are undeclared → must stay null.
	state := &PatchPolicyResourceModel{
		Scope: &PatchPolicyScopeModel{
			Targets: &PatchPolicyScopeTargetsModel{
				ComputerIDs:      idSet(t),
				ComputerGroupIDs: idSet(t, "999"),
			},
			Limitations: &PatchPolicyScopeLimitationsModel{
				NetworkSegmentIDs: idSet(t),
			},
			Exclusions: &PatchPolicyScopeExclusionsModel{
				IbeaconIDs: idSet(t),
			},
		},
	}
	diags := assignPatchPolicyResourceModel(context.Background(), state, p, false)
	if diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}

	if got := setStrings(t, state.Scope.Targets.ComputerIDs); len(got) != 1 || got[0] != "10" {
		t.Errorf("targets.computer_ids: got %v", got)
	}
	if got := setStrings(t, state.Scope.Targets.ComputerGroupIDs); len(got) != 1 || got[0] != "20" {
		t.Errorf("targets.computer_group_ids: got %v", got)
	}
	if !state.Scope.Targets.BuildingIDs.IsNull() {
		t.Errorf("unmanaged targets.building_ids must stay null, got %v", state.Scope.Targets.BuildingIDs)
	}
	if !state.Scope.Targets.AllComputers.IsNull() {
		t.Errorf("unmanaged targets.all_computers must stay null, got %v", state.Scope.Targets.AllComputers)
	}
	if got := setStrings(t, state.Scope.Limitations.NetworkSegmentIDs); len(got) != 1 || got[0] != "50" {
		t.Errorf("limitations.network_segment_ids: got %v", got)
	}
	if !state.Scope.Limitations.IbeaconIDs.IsNull() {
		t.Errorf("unmanaged limitations.ibeacon_ids must stay null, got %v", state.Scope.Limitations.IbeaconIDs)
	}
	if got := setStrings(t, state.Scope.Exclusions.IbeaconIDs); len(got) != 1 || got[0] != "75" {
		t.Errorf("exclusions.ibeacon_ids: got %v", got)
	}
	if !state.Scope.Exclusions.ComputerGroupIDs.IsNull() {
		t.Errorf("unmanaged exclusions.computer_group_ids must stay null, got %v", state.Scope.Exclusions.ComputerGroupIDs)
	}
}

// TestFlattenScope_UnmanagedNotRefreshed confirms an unmanaged scope block (nil
// in state) is not populated even though the server echoes <scope>.
func TestFlattenScope_UnmanagedNotRefreshed(t *testing.T) {
	p := &proclassic.PatchPolicy{
		ID:      new(1),
		General: &proclassic.PatchPolicyGeneral{Name: new("p"), TargetVersion: new("1.0")},
		Scope: &proclassic.PatchPolicyScope{
			Computers: &proclassic.PatchPolicyScopeComputers{
				Computer: &[]proclassic.PatchPolicyScopeComputersComputerItem{{ID: new(10)}},
			},
		},
	}
	state := &PatchPolicyResourceModel{} // Scope nil → unmanaged.
	if diags := assignPatchPolicyResourceModel(context.Background(), state, p, false); diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}
	if state.Scope != nil {
		t.Errorf("unmanaged scope must stay nil, got %+v", state.Scope)
	}
}

// TestFlattenUserInteraction_NestedRoundTrip confirms the user_interaction block
// reads back when managed.
func TestFlattenUserInteraction_NestedRoundTrip(t *testing.T) {
	p := &proclassic.PatchPolicy{
		ID:      new(1),
		General: &proclassic.PatchPolicyGeneral{Name: new("p"), TargetVersion: new("1.0")},
		UserInteraction: &proclassic.PatchPolicyUserInteraction{
			InstallButtonText: new("Update"),
			SelfServiceIcon:   &proclassic.PatchPolicyUserInteractionSelfServiceIcon{ID: new(99)},
			Notifications: &proclassic.PatchPolicyUserInteractionNotifications{
				NotificationEnabled: new(true),
				NotificationSubject: new("subj"),
				Reminders: &proclassic.PatchPolicyUserInteractionNotificationsReminders{
					NotificationRemindersEnabled:  new(true),
					NotificationReminderFrequency: new(24),
				},
			},
			Deadlines: &proclassic.PatchPolicyUserInteractionDeadlines{
				DeadlineEnabled: new(true),
				DeadlinePeriod:  new(7),
			},
			GracePeriod: &proclassic.PatchPolicyUserInteractionGracePeriod{
				GracePeriodDuration:       new(15),
				NotificationCenterSubject: new("Important"),
			},
		},
	}

	state := &PatchPolicyResourceModel{
		UserInteraction: &PatchPolicyUserInteractionModel{
			Notifications: &PatchPolicyUserInteractionNotificationsModel{
				Reminders: &PatchPolicyUserInteractionNotificationsRemindersModel{},
			},
			Deadlines:   &PatchPolicyUserInteractionDeadlinesModel{},
			GracePeriod: &PatchPolicyUserInteractionGracePeriodModel{},
		},
	}
	if diags := assignPatchPolicyResourceModel(context.Background(), state, p, false); diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}

	ui := state.UserInteraction
	if ui.InstallButtonText.ValueString() != "Update" {
		t.Errorf("install_button_text: got %q", ui.InstallButtonText.ValueString())
	}
	if ui.SelfServiceIconID.ValueString() != "99" {
		t.Errorf("self_service_icon_id: got %q", ui.SelfServiceIconID.ValueString())
	}
	if !ui.Notifications.Enabled.ValueBool() || ui.Notifications.Subject.ValueString() != "subj" {
		t.Errorf("notifications not flattened")
	}
	if ui.Notifications.Reminders.Frequency.ValueInt64() != 24 {
		t.Errorf("reminders frequency: got %d", ui.Notifications.Reminders.Frequency.ValueInt64())
	}
	if ui.Deadlines.Period.ValueInt64() != 7 {
		t.Errorf("deadlines period: got %d", ui.Deadlines.Period.ValueInt64())
	}
	if ui.GracePeriod.Duration.ValueInt64() != 15 || ui.GracePeriod.NotificationCenterSubject.ValueString() != "Important" {
		t.Errorf("grace_period not flattened")
	}
}

// TestAssignPatchPolicyResourceModel_IncludeUnmanagedHydratesFromScratch pins
// the config-generation contract: with includeUnmanaged set and an empty
// starting model, every wire-present section (scope + its sub-blocks,
// user_interaction + its sub-blocks) is allocated and hydrated from the server.
func TestAssignPatchPolicyResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	p := &proclassic.PatchPolicy{
		ID:      new(1),
		General: &proclassic.PatchPolicyGeneral{Name: new("p"), TargetVersion: new("1.0")},
		Scope: &proclassic.PatchPolicyScope{
			AllComputers: new(true),
			ComputerGroups: &proclassic.PatchPolicyScopeComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(20)}, {ID: new(21)}},
			},
			Exclusions: &proclassic.PatchPolicyScopeExclusions{
				Ibeacons: &proclassic.PatchPolicyScopeExclusionsIbeacons{
					Ibeacon: &[]proclassic.IDName{{ID: new(75)}},
				},
			},
		},
		UserInteraction: &proclassic.PatchPolicyUserInteraction{
			InstallButtonText: new("Update"),
			Notifications: &proclassic.PatchPolicyUserInteractionNotifications{
				NotificationEnabled: new(true),
				Reminders: &proclassic.PatchPolicyUserInteractionNotificationsReminders{
					NotificationReminderFrequency: new(24),
				},
			},
			Deadlines: &proclassic.PatchPolicyUserInteractionDeadlines{DeadlinePeriod: new(7)},
		},
	}

	state := &PatchPolicyResourceModel{}
	diags := assignPatchPolicyResourceModel(context.Background(), state, p, true)
	if diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}

	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected scope.targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers hydrated true")
	}
	if got := setStrings(t, state.Scope.Targets.ComputerGroupIDs); len(got) != 2 {
		t.Fatalf("expected 2 computer_group_ids, got %v", got)
	}
	if state.Scope.Exclusions == nil || len(setStrings(t, state.Scope.Exclusions.IbeaconIDs)) != 1 {
		t.Fatalf("expected exclusions ibeacon hydrated; got %+v", state.Scope.Exclusions)
	}
	if state.Scope.Limitations != nil {
		t.Fatalf("expected limitations nil when wire-absent; got %+v", state.Scope.Limitations)
	}
	if state.UserInteraction == nil || state.UserInteraction.InstallButtonText.ValueString() != "Update" {
		t.Fatalf("expected user_interaction hydrated; got %+v", state.UserInteraction)
	}
	if state.UserInteraction.Notifications == nil || !state.UserInteraction.Notifications.Enabled.ValueBool() {
		t.Fatalf("expected notifications hydrated; got %+v", state.UserInteraction.Notifications)
	}
	if state.UserInteraction.Notifications.Reminders == nil || state.UserInteraction.Notifications.Reminders.Frequency.ValueInt64() != 24 {
		t.Fatalf("expected reminders hydrated; got %+v", state.UserInteraction.Notifications.Reminders)
	}
	if state.UserInteraction.Deadlines == nil || state.UserInteraction.Deadlines.Period.ValueInt64() != 7 {
		t.Fatalf("expected deadlines hydrated; got %+v", state.UserInteraction.Deadlines)
	}
	if state.UserInteraction.GracePeriod != nil {
		t.Fatalf("expected grace_period nil when wire-absent; got %+v", state.UserInteraction.GracePeriod)
	}
}

func setStrings(t *testing.T, s types.Set) []string {
	t.Helper()
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	if diags := s.ElementsAs(context.Background(), &out, false); diags.HasError() {
		t.Fatalf("setStrings: %v", diags)
	}
	return out
}
