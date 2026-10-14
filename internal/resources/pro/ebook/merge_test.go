// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestMergeEbookScope_NilPlanStaysNil(t *testing.T) {
	if got := mergeEbookScope(nil, &EbookScopeModel{}); got != nil {
		t.Fatalf("expected nil for unmanaged scope, got %+v", got)
	}
}

func TestMergeEbookScope_DeclaredWinsOmittedPreserved(t *testing.T) {
	plan := &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			ComputerGroupIDs: idSet("10"), // declared: wins
			BuildingIDs:      idSet(),     // declared []: clears
			// everything else omitted: preserved from the live scope.
		},
		// limitations + exclusions omitted entirely: preserved.
	}
	server := &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			AllComputers:         types.BoolValue(false),
			AllMobileDevices:     types.BoolValue(false),
			AllJssUsers:          types.BoolValue(false),
			ComputerIDs:          scope.EmptyStringSet(),
			ComputerGroupIDs:     idSet("99"),
			MobileDeviceIDs:      idSet("21"),
			MobileDeviceGroupIDs: scope.EmptyStringSet(),
			BuildingIDs:          idSet("7"),
			DepartmentIDs:        idSet("3"),
			UserIDs:              scope.EmptyStringSet(),
			UserGroupIDs:         scope.EmptyStringSet(),
			ClassIDs:             idSet("41"),
		},
		Exclusions: &EbookScopeExclusionsModel{
			MobileDeviceIDs: idSet("55"),
		},
	}

	got := mergeEbookScope(plan, server)

	if v := got.Targets.ComputerGroupIDs.Elements(); len(v) != 1 {
		t.Errorf("declared category should win: got %v", v)
	}
	if got.Targets.BuildingIDs.IsNull() || len(got.Targets.BuildingIDs.Elements()) != 0 {
		t.Error("declared [] must merge as a non-null empty set (emits the empty wrapper)")
	}
	if v := got.Targets.DepartmentIDs.Elements(); len(v) != 1 {
		t.Errorf("omitted category should preserve live members: got %v", v)
	}
	// The mobile side of the union survives a computer-only declaration — the
	// whole point of the merge on ebook's single replace unit.
	if v := got.Targets.MobileDeviceIDs.Elements(); len(v) != 1 {
		t.Errorf("omitted mobile category should preserve live members: got %v", v)
	}
	if v := got.Targets.ClassIDs.Elements(); len(v) != 1 {
		t.Errorf("omitted class_ids should preserve live members: got %v", v)
	}
	if got.Exclusions == nil {
		t.Fatal("omitted exclusions block must still merge from the live scope")
	}
	if v := got.Exclusions.MobileDeviceIDs.Elements(); len(v) != 1 {
		t.Errorf("omitted exclusions category should preserve live members: got %v", v)
	}
	// Every merged field must be non-null so the builder emits the full
	// explicit skeleton.
	if got.Targets.ComputerIDs.IsNull() || got.Limitations == nil ||
		got.Limitations.NetworkSegmentIDs.IsNull() ||
		got.Exclusions.ComputerIDs.IsNull() ||
		got.Exclusions.DirectoryServiceUserGroupNames.IsNull() {
		t.Error("merged output must have every category non-null")
	}
	if got.Targets.AllComputers.IsNull() || got.Targets.AllMobileDevices.IsNull() || got.Targets.AllJssUsers.IsNull() {
		t.Error("merged all-flags must be non-null")
	}
}

func TestMergeEbookScope_NilServerTreatedAsEmpty(t *testing.T) {
	plan := &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			ComputerGroupIDs: idSet("10"),
		},
	}
	got := mergeEbookScope(plan, nil)
	if v := got.Targets.ComputerGroupIDs.Elements(); len(v) != 1 {
		t.Errorf("expected declared members, got %v", v)
	}
	if got.Targets.DepartmentIDs.IsNull() || len(got.Targets.DepartmentIDs.Elements()) != 0 {
		t.Error("expected empty non-null set for omitted category with no live value")
	}
	if got.Targets.AllComputers.IsNull() || got.Targets.AllComputers.ValueBool() {
		t.Error("expected all-flags to default false, non-null")
	}
}

// TestMergeEbookScope_AllFlagPrecedence pins the flag rule: a merged true flag
// empties exactly the target categories its validator names — never the other
// union's targets, buildings/departments/classes, or limitations/exclusions.
func TestMergeEbookScope_AllFlagPrecedence(t *testing.T) {
	plan := &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			AllComputers: types.BoolValue(true),
		},
	}
	server := &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			ComputerIDs:          idSet("1"),
			ComputerGroupIDs:     idSet("99"),
			MobileDeviceIDs:      idSet("21"),
			MobileDeviceGroupIDs: idSet("6"),
			BuildingIDs:          idSet("7"),
			ClassIDs:             idSet("41"),
			UserIDs:              idSet("31"),
		},
		Limitations: &EbookScopeLimitationsModel{
			NetworkSegmentIDs: idSet("2"),
		},
		Exclusions: &EbookScopeExclusionsModel{
			BuildingIDs: idSet("55"),
		},
	}
	got := mergeEbookScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers=true")
	}
	if len(got.Targets.ComputerIDs.Elements()) != 0 || len(got.Targets.ComputerGroupIDs.Elements()) != 0 {
		t.Error("expected computer target categories emptied under all_computers=true")
	}
	// The mobile union, buildings, classes, users, limitations, and exclusions
	// all coexist with the flag and are preserved.
	if len(got.Targets.MobileDeviceIDs.Elements()) != 1 || len(got.Targets.MobileDeviceGroupIDs.Elements()) != 1 {
		t.Error("expected mobile targets preserved under all_computers=true")
	}
	if len(got.Targets.BuildingIDs.Elements()) != 1 || len(got.Targets.ClassIDs.Elements()) != 1 || len(got.Targets.UserIDs.Elements()) != 1 {
		t.Error("expected non-conflicting targets preserved under all_computers=true")
	}
	if len(got.Limitations.NetworkSegmentIDs.Elements()) != 1 {
		t.Error("expected limitations preserved under all_computers=true")
	}
	if len(got.Exclusions.BuildingIDs.Elements()) != 1 {
		t.Error("expected exclusions preserved under all_computers=true")
	}
}

func TestMergeEbookScope_DeclaredPairedCategoryBeatsPreservedFlag(t *testing.T) {
	cases := map[string]struct {
		targets EbookScopeTargetsModel
		flag    func(*EbookScopeTargetsModel) bool
	}{
		"computer group": {EbookScopeTargetsModel{ComputerGroupIDs: idSet("58")}, func(t *EbookScopeTargetsModel) bool { return t.AllComputers.ValueBool() }},
		"mobile group":   {EbookScopeTargetsModel{MobileDeviceGroupIDs: idSet("12")}, func(t *EbookScopeTargetsModel) bool { return t.AllMobileDevices.ValueBool() }},
		"user group":     {EbookScopeTargetsModel{UserGroupIDs: idSet("9")}, func(t *EbookScopeTargetsModel) bool { return t.AllJssUsers.ValueBool() }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			targets := tc.targets
			plan := &EbookScopeModel{Targets: &targets}
			server := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
				AllComputers:     types.BoolValue(true),
				AllMobileDevices: types.BoolValue(true),
				AllJssUsers:      types.BoolValue(true),
			}}
			got := mergeEbookScope(plan, server)
			if tc.flag(got.Targets) {
				t.Errorf("expected the flag paired with the declared %s turned off", name)
			}
		})
	}
}

// Only the paired flag turns off: the other two preserved flags stay as the
// admin UI left them.
func TestMergeEbookScope_DeclaredPairedCategoryLeavesOtherFlags(t *testing.T) {
	plan := &EbookScopeModel{Targets: &EbookScopeTargetsModel{ComputerGroupIDs: idSet("58")}}
	server := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		AllComputers:     types.BoolValue(true),
		AllMobileDevices: types.BoolValue(true),
	}}
	got := mergeEbookScope(plan, server)
	if got.Targets.AllComputers.ValueBool() {
		t.Error("expected all_computers=false")
	}
	if !got.Targets.AllMobileDevices.ValueBool() {
		t.Error("expected the unrelated all_mobile_devices=true to stay")
	}
}

// With all three flags on the admin UI offers no building, department or
// class, so the merge empties them (the server would keep them).
func TestMergeEbookScope_AllThreeFlagsEmptyResidualCategories(t *testing.T) {
	plan := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		AllComputers:     types.BoolValue(true),
		AllMobileDevices: types.BoolValue(true),
		AllJssUsers:      types.BoolValue(true),
	}}
	server := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		BuildingIDs:   idSet("7"),
		DepartmentIDs: idSet("3"),
		ClassIDs:      idSet("41"),
	}, Exclusions: &EbookScopeExclusionsModel{BuildingIDs: idSet("55")}}
	got := mergeEbookScope(plan, server)
	for label, set := range map[string]types.Set{
		"building_ids":   got.Targets.BuildingIDs,
		"department_ids": got.Targets.DepartmentIDs,
		"class_ids":      got.Targets.ClassIDs,
	} {
		if set.IsNull() || len(set.Elements()) != 0 {
			t.Errorf("expected %s emptied under all three flags, got %v", label, set)
		}
	}
	if len(got.Exclusions.BuildingIDs.Elements()) != 1 {
		t.Error("expected exclusions preserved under all three flags")
	}
}

// Two flags on is not the all-three case: buildings, departments and classes
// stay (the UI offers them).
func TestMergeEbookScope_TwoFlagsKeepResidualCategories(t *testing.T) {
	plan := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		AllComputers:     types.BoolValue(true),
		AllMobileDevices: types.BoolValue(true),
	}}
	server := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		BuildingIDs:   idSet("7"),
		DepartmentIDs: idSet("3"),
		ClassIDs:      idSet("41"),
	}}
	got := mergeEbookScope(plan, server)
	if len(got.Targets.BuildingIDs.Elements()) != 1 || len(got.Targets.DepartmentIDs.Elements()) != 1 || len(got.Targets.ClassIDs.Elements()) != 1 {
		t.Errorf("expected residual categories preserved with two flags on, got %v / %v / %v",
			got.Targets.BuildingIDs, got.Targets.DepartmentIDs, got.Targets.ClassIDs)
	}
}

// A declared building while the admin UI has all three flags on: the
// declaration wins, so every unmanaged flag is turned off rather than the
// building being wiped.
func TestMergeEbookScope_DeclaredResidualBeatsPreservedAllThreeFlags(t *testing.T) {
	plan := &EbookScopeModel{Targets: &EbookScopeTargetsModel{BuildingIDs: idSet("7")}}
	server := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		AllComputers:     types.BoolValue(true),
		AllMobileDevices: types.BoolValue(true),
		AllJssUsers:      types.BoolValue(true),
	}}
	got := mergeEbookScope(plan, server)
	if got.Targets.AllComputers.ValueBool() || got.Targets.AllMobileDevices.ValueBool() || got.Targets.AllJssUsers.ValueBool() {
		t.Errorf("expected every unmanaged flag turned off, got %v / %v / %v",
			got.Targets.AllComputers, got.Targets.AllMobileDevices, got.Targets.AllJssUsers)
	}
	if len(got.Targets.BuildingIDs.Elements()) != 1 {
		t.Errorf("expected the declared building kept, got %v", got.Targets.BuildingIDs)
	}
}

// Flags the plan declares are never touched: with one declared true and the
// other two preserved, only the preserved ones turn off.
func TestMergeEbookScope_DeclaredResidualSparesDeclaredFlags(t *testing.T) {
	plan := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		AllComputers: types.BoolValue(true),
		ClassIDs:     idSet("41"),
	}}
	server := &EbookScopeModel{Targets: &EbookScopeTargetsModel{
		AllMobileDevices: types.BoolValue(true),
		AllJssUsers:      types.BoolValue(true),
	}}
	got := mergeEbookScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the declared all_computers=true to stay")
	}
	if got.Targets.AllMobileDevices.ValueBool() || got.Targets.AllJssUsers.ValueBool() {
		t.Error("expected the preserved flags turned off")
	}
	if len(got.Targets.ClassIDs.Elements()) != 1 {
		t.Errorf("expected the declared class kept, got %v", got.Targets.ClassIDs)
	}
}
