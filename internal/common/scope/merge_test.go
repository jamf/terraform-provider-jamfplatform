// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMergeComputerScopeNoIbeacons_NilPlanStaysNil(t *testing.T) {
	if got := MergeComputerScopeNoIbeacons(nil, &ComputerScopeModelNoIbeacons{}); got != nil {
		t.Fatalf("expected nil for unmanaged scope, got %+v", got)
	}
}

func TestMergeComputerScopeNoIbeacons_DeclaredWinsOmittedPreserved(t *testing.T) {
	plan := &ComputerScopeModelNoIbeacons{
		Targets: &ComputerScopeTargetsModel{
			AllComputers:     types.BoolNull(),
			AllJssUsers:      types.BoolNull(),
			ComputerIDs:      types.SetNull(types.StringType),
			ComputerGroupIDs: newStringSet(t, []string{"10"}), // declared: wins
			BuildingIDs:      newStringSet(t, nil),            // declared []: clears
			DepartmentIDs:    types.SetNull(types.StringType), // omitted: server's
			UserIDs:          types.SetNull(types.StringType),
			UserGroupIDs:     types.SetNull(types.StringType),
		},
		// limitations + exclusions omitted entirely: preserved from server.
	}
	server := &ComputerScopeModelNoIbeacons{
		Targets: &ComputerScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			AllJssUsers:      types.BoolValue(false),
			ComputerIDs:      EmptyStringSet(),
			ComputerGroupIDs: newStringSet(t, []string{"99"}),
			BuildingIDs:      newStringSet(t, []string{"7"}),
			DepartmentIDs:    newStringSet(t, []string{"3"}),
			UserIDs:          EmptyStringSet(),
			UserGroupIDs:     EmptyStringSet(),
		},
		Exclusions: &ComputerScopeExclusionsModelNoIbeacons{
			BuildingIDs: newStringSet(t, []string{"55"}),
		},
	}

	got := MergeComputerScopeNoIbeacons(plan, server)

	if v := sortedSetValues(t, got.Targets.ComputerGroupIDs); len(v) != 1 || v[0] != "10" {
		t.Errorf("declared category should win: got %v", v)
	}
	if v := sortedSetValues(t, got.Targets.BuildingIDs); len(v) != 0 {
		t.Errorf("declared [] should clear: got %v", v)
	}
	if got.Targets.BuildingIDs.IsNull() {
		t.Error("declared [] must stay non-null (emits the empty wrapper)")
	}
	if v := sortedSetValues(t, got.Targets.DepartmentIDs); len(v) != 1 || v[0] != "3" {
		t.Errorf("omitted category should preserve server members: got %v", v)
	}
	if got.Exclusions == nil {
		t.Fatal("omitted exclusions block must still merge from server")
	}
	if v := sortedSetValues(t, got.Exclusions.BuildingIDs); len(v) != 1 || v[0] != "55" {
		t.Errorf("omitted exclusions category should preserve server members: got %v", v)
	}
	// Every merged field must be non-null so the builder emits the full
	// explicit skeleton.
	if got.Targets.ComputerIDs.IsNull() || got.Limitations == nil ||
		got.Limitations.NetworkSegmentIDs.IsNull() ||
		got.Exclusions.ComputerIDs.IsNull() {
		t.Error("merged output must have every category non-null")
	}
	if got.Targets.AllComputers.IsNull() || got.Targets.AllJssUsers.IsNull() {
		t.Error("merged all-flags must be non-null")
	}
}

func TestMergeComputerScopeNoIbeacons_NilServerTreatedAsEmpty(t *testing.T) {
	plan := &ComputerScopeModelNoIbeacons{
		Targets: &ComputerScopeTargetsModel{
			ComputerGroupIDs: newStringSet(t, []string{"10"}),
		},
	}
	got := MergeComputerScopeNoIbeacons(plan, nil)
	if v := sortedSetValues(t, got.Targets.ComputerGroupIDs); len(v) != 1 || v[0] != "10" {
		t.Errorf("expected declared members, got %v", v)
	}
	if got.Targets.DepartmentIDs.IsNull() || len(got.Targets.DepartmentIDs.Elements()) != 0 {
		t.Errorf("expected empty non-null set for omitted category with no server value")
	}
}

func TestMergeComputerScope_AllComputersTrueEmptiesConflictingTargets(t *testing.T) {
	plan := &ComputerScopeModel{
		Targets: &ComputerScopeTargetsModel{
			AllComputers: types.BoolValue(true),
		},
	}
	server := &ComputerScopeModel{
		Targets: &ComputerScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			ComputerGroupIDs: newStringSet(t, []string{"99"}),
			BuildingIDs:      newStringSet(t, []string{"7"}),
			UserIDs:          newStringSet(t, []string{"1"}),
		},
		Exclusions: &ComputerScopeExclusionsModel{
			BuildingIDs: newStringSet(t, []string{"55"}),
		},
	}
	got := MergeComputerScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers=true")
	}
	// The categories the flag pairs with are emptied (the wire wipes them when
	// the flag is set)...
	if len(got.Targets.ComputerGroupIDs.Elements()) != 0 {
		t.Error("expected computer groups emptied under all_computers=true")
	}
	// ...but buildings, user-side targets and exclusions coexist with the flag
	// (wire-probed 2026-10-14) and are preserved.
	if v := sortedSetValues(t, got.Targets.BuildingIDs); len(v) != 1 || v[0] != "7" {
		t.Errorf("expected buildings preserved under all_computers=true, got %v", v)
	}
	if v := sortedSetValues(t, got.Targets.UserIDs); len(v) != 1 || v[0] != "1" {
		t.Errorf("expected user targets preserved under all_computers=true, got %v", v)
	}
	if v := sortedSetValues(t, got.Exclusions.BuildingIDs); len(v) != 1 || v[0] != "55" {
		t.Errorf("expected exclusions preserved under all_computers=true, got %v", v)
	}
}

func TestMergeMobileScopeNoIbeacons_AllMobileDevicesPrecedence(t *testing.T) {
	plan := &MobileScopeModelNoIbeacons{
		Targets: &MobileScopeTargetsModel{
			AllMobileDevices: types.BoolValue(true),
		},
	}
	server := &MobileScopeModelNoIbeacons{
		Targets: &MobileScopeTargetsModel{
			MobileDeviceGroupIDs: newStringSet(t, []string{"12"}),
			BuildingIDs:          newStringSet(t, []string{"7"}),
			UserGroupIDs:         newStringSet(t, []string{"4"}),
		},
	}
	got := MergeMobileScopeNoIbeacons(plan, server)
	if len(got.Targets.MobileDeviceGroupIDs.Elements()) != 0 {
		t.Error("expected device-side categories emptied under all_mobile_devices=true")
	}
	if v := sortedSetValues(t, got.Targets.BuildingIDs); len(v) != 1 || v[0] != "7" {
		t.Errorf("expected buildings preserved under all_mobile_devices=true, got %v", v)
	}
	if v := sortedSetValues(t, got.Targets.UserGroupIDs); len(v) != 1 || v[0] != "4" {
		t.Errorf("expected user-side categories preserved, got %v", v)
	}
}

func TestMergeUserScope_DeclaredWinsOmittedPreserved(t *testing.T) {
	plan := &UserScopeModel{
		Targets: &UserScopeTargetsModel{
			JssUserIDs: newStringSet(t, []string{"279"}),
		},
	}
	server := &UserScopeModel{
		Targets: &UserScopeTargetsModel{
			AllJssUsers:     types.BoolValue(false),
			JssUserIDs:      newStringSet(t, []string{"1"}),
			JssUserGroupIDs: newStringSet(t, []string{"928"}),
		},
		Exclusions: &UserScopeExclusionsModel{
			JssUserIDs: newStringSet(t, []string{"280"}),
		},
	}
	got := MergeUserScope(plan, server)
	if v := sortedSetValues(t, got.Targets.JssUserIDs); len(v) != 1 || v[0] != "279" {
		t.Errorf("declared should win: got %v", v)
	}
	if v := sortedSetValues(t, got.Targets.JssUserGroupIDs); len(v) != 1 || v[0] != "928" {
		t.Errorf("omitted should preserve server: got %v", v)
	}
	if v := sortedSetValues(t, got.Exclusions.JssUserIDs); len(v) != 1 || v[0] != "280" {
		t.Errorf("omitted exclusions should preserve server: got %v", v)
	}
	if got.Limitations == nil || got.Limitations.DirectoryServiceUserGroupNames.IsNull() {
		t.Error("merged output must have every category non-null")
	}
}

// TestMergeComputerScope_DeclaredGroupBeatsPreservedAllComputers is the #467
// regression: the plan declares only computer_group_ids, the admin UI has
// since switched All Computers on, and the merge must send the flag false
// rather than inherit the true that would wipe the declared group.
func TestMergeComputerScope_DeclaredGroupBeatsPreservedAllComputers(t *testing.T) {
	plan := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		ComputerGroupIDs: newStringSet(t, []string{"58"}),
	}}
	server := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		AllComputers: types.BoolValue(true),
	}}
	got := MergeComputerScope(plan, server)
	if got.Targets.AllComputers.IsNull() || got.Targets.AllComputers.ValueBool() {
		t.Errorf("expected all_computers=false, got %v", got.Targets.AllComputers)
	}
	if v := sortedSetValues(t, got.Targets.ComputerGroupIDs); len(v) != 1 || v[0] != "58" {
		t.Errorf("expected declared group kept, got %v", v)
	}
}

func TestMergeComputerScope_DeclaredComputerBeatsPreservedAllComputers(t *testing.T) {
	plan := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		ComputerIDs: newStringSet(t, []string{"5"}),
	}}
	server := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{AllComputers: types.BoolValue(true)}}
	got := MergeComputerScope(plan, server)
	if got.Targets.AllComputers.ValueBool() {
		t.Error("expected all_computers=false")
	}
	if v := sortedSetValues(t, got.Targets.ComputerIDs); len(v) != 1 || v[0] != "5" {
		t.Errorf("expected declared computer kept, got %v", v)
	}
}

// A declared building or department is not paired with all_computers, so it
// neither turns the preserved flag off nor is it wiped by it.
func TestMergeComputerScope_DeclaredBuildingKeepsPreservedAllComputers(t *testing.T) {
	plan := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		BuildingIDs:   newStringSet(t, []string{"7"}),
		DepartmentIDs: newStringSet(t, []string{"3"}),
	}}
	server := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{AllComputers: types.BoolValue(true)}}
	got := MergeComputerScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the preserved all_computers=true to stay")
	}
	if v := sortedSetValues(t, got.Targets.BuildingIDs); len(v) != 1 || v[0] != "7" {
		t.Errorf("expected declared building kept, got %v", v)
	}
	if v := sortedSetValues(t, got.Targets.DepartmentIDs); len(v) != 1 || v[0] != "3" {
		t.Errorf("expected declared department kept, got %v", v)
	}
}

// A declared [] clears the category; it carries no members to conflict with, so
// it must not switch a preserved flag off.
func TestMergeComputerScope_DeclaredEmptyGroupsKeepPreservedAllComputers(t *testing.T) {
	plan := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		ComputerGroupIDs: newStringSet(t, nil),
	}}
	server := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{AllComputers: types.BoolValue(true)}}
	got := MergeComputerScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the preserved all_computers=true to stay")
	}
}

func TestMergeComputerScope_DeclaredFlagAlwaysWins(t *testing.T) {
	plan := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		AllComputers: types.BoolValue(false),
	}}
	server := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{AllComputers: types.BoolValue(true)}}
	if got := MergeComputerScope(plan, server); got.Targets.AllComputers.ValueBool() {
		t.Error("expected the declared false to win over the live true")
	}
	plan.Targets.AllComputers = types.BoolValue(true)
	server.Targets.AllComputers = types.BoolValue(false)
	if got := MergeComputerScope(plan, server); !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the declared true to win over the live false")
	}
}

// all_jss_users is the second flag on the computer scope and pairs with the
// user categories, independently of all_computers.
func TestMergeComputerScope_DeclaredUserGroupBeatsPreservedAllJssUsers(t *testing.T) {
	plan := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		UserGroupIDs: newStringSet(t, []string{"9"}),
	}}
	server := &ComputerScopeModel{Targets: &ComputerScopeTargetsModel{
		AllComputers: types.BoolValue(true),
		AllJssUsers:  types.BoolValue(true),
	}}
	got := MergeComputerScope(plan, server)
	if got.Targets.AllJssUsers.ValueBool() {
		t.Error("expected all_jss_users=false")
	}
	if !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the unrelated all_computers=true to stay")
	}
}

func TestMergeComputerScopeNoIbeacons_DeclaredGroupBeatsPreservedAllComputers(t *testing.T) {
	plan := &ComputerScopeModelNoIbeacons{Targets: &ComputerScopeTargetsModel{
		ComputerGroupIDs: newStringSet(t, []string{"58"}),
	}}
	server := &ComputerScopeModelNoIbeacons{Targets: &ComputerScopeTargetsModel{AllComputers: types.BoolValue(true)}}
	got := MergeComputerScopeNoIbeacons(plan, server)
	if got.Targets.AllComputers.ValueBool() {
		t.Error("expected all_computers=false")
	}
}

func TestMergeMobileScope_DeclaredGroupBeatsPreservedAllMobileDevices(t *testing.T) {
	plan := &MobileScopeModel{Targets: &MobileScopeTargetsModel{
		MobileDeviceGroupIDs: newStringSet(t, []string{"12"}),
	}}
	server := &MobileScopeModel{Targets: &MobileScopeTargetsModel{AllMobileDevices: types.BoolValue(true)}}
	got := MergeMobileScope(plan, server)
	if got.Targets.AllMobileDevices.ValueBool() {
		t.Error("expected all_mobile_devices=false")
	}
	if v := sortedSetValues(t, got.Targets.MobileDeviceGroupIDs); len(v) != 1 || v[0] != "12" {
		t.Errorf("expected declared group kept, got %v", v)
	}
}

func TestMergeMobileScopeNoIbeacons_DeclaredBuildingKeepsPreservedAllMobileDevices(t *testing.T) {
	plan := &MobileScopeModelNoIbeacons{Targets: &MobileScopeTargetsModel{
		BuildingIDs: newStringSet(t, []string{"7"}),
	}}
	server := &MobileScopeModelNoIbeacons{Targets: &MobileScopeTargetsModel{AllMobileDevices: types.BoolValue(true)}}
	got := MergeMobileScopeNoIbeacons(plan, server)
	if !got.Targets.AllMobileDevices.ValueBool() {
		t.Error("expected the preserved all_mobile_devices=true to stay")
	}
	if v := sortedSetValues(t, got.Targets.BuildingIDs); len(v) != 1 || v[0] != "7" {
		t.Errorf("expected declared building kept, got %v", v)
	}
}

func TestMergeUserScope_DeclaredUserBeatsPreservedAllJssUsers(t *testing.T) {
	plan := &UserScopeModel{Targets: &UserScopeTargetsModel{
		JssUserIDs: newStringSet(t, []string{"279"}),
	}}
	server := &UserScopeModel{Targets: &UserScopeTargetsModel{AllJssUsers: types.BoolValue(true)}}
	got := MergeUserScope(plan, server)
	if got.Targets.AllJssUsers.ValueBool() {
		t.Error("expected all_jss_users=false")
	}
	if v := sortedSetValues(t, got.Targets.JssUserIDs); len(v) != 1 || v[0] != "279" {
		t.Errorf("expected declared user kept, got %v", v)
	}
}

func TestPickAllFlag(t *testing.T) {
	members := newStringSet(t, []string{"1"})
	empty := newStringSet(t, nil)
	null := types.SetNull(types.StringType)
	tests := []struct {
		name     string
		declared types.Bool
		server   types.Bool
		paired   []types.Set
		want     types.Bool
	}{
		{"declared true wins", types.BoolValue(true), types.BoolValue(false), nil, types.BoolValue(true)},
		{"declared false wins over live true", types.BoolValue(false), types.BoolValue(true), []types.Set{empty}, types.BoolValue(false)},
		{"declared true wins over declared members", types.BoolValue(true), types.BoolNull(), []types.Set{members}, types.BoolValue(true)},
		{"members turn a live true off", types.BoolNull(), types.BoolValue(true), []types.Set{members}, types.BoolValue(false)},
		{"any one populated set is enough", types.BoolNull(), types.BoolValue(true), []types.Set{null, members}, types.BoolValue(false)},
		{"empty set keeps live true", types.BoolNull(), types.BoolValue(true), []types.Set{empty}, types.BoolValue(true)},
		{"null set keeps live true", types.BoolNull(), types.BoolValue(true), []types.Set{null}, types.BoolValue(true)},
		{"unknown set keeps live true", types.BoolNull(), types.BoolValue(true), []types.Set{types.SetUnknown(types.StringType)}, types.BoolValue(true)},
		{"nothing declared keeps live false", types.BoolNull(), types.BoolValue(false), nil, types.BoolValue(false)},
		{"no live value defaults false", types.BoolNull(), types.BoolNull(), nil, types.BoolValue(false)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PickAllFlag(tc.declared, tc.server, tc.paired...); !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
