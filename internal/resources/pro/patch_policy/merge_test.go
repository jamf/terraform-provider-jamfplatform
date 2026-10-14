// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestMergePatchPolicyScope_NilPlanStaysNil(t *testing.T) {
	if got := mergePatchPolicyScope(nil, &PatchPolicyScopeModel{}); got != nil {
		t.Fatalf("expected nil for unmanaged scope, got %+v", got)
	}
}

func TestMergePatchPolicyScope_DeclaredWinsOmittedPreserved(t *testing.T) {
	plan := &PatchPolicyScopeModel{
		Targets: &PatchPolicyScopeTargetsModel{
			ComputerGroupIDs: idSet(t, "10"), // declared: wins
			BuildingIDs:      idSet(t),       // declared []: clears
			// department_ids omitted: preserved from the live scope.
		},
		// limitations + exclusions omitted entirely: preserved.
	}
	server := &PatchPolicyScopeModel{
		Targets: &PatchPolicyScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			ComputerIDs:      scope.EmptyStringSet(),
			ComputerGroupIDs: idSet(t, "99"),
			BuildingIDs:      idSet(t, "7"),
			DepartmentIDs:    idSet(t, "3"),
		},
		Limitations: &PatchPolicyScopeLimitationsModel{
			NetworkSegmentIDs: idSet(t, "50"),
		},
		Exclusions: &PatchPolicyScopeExclusionsModel{
			IbeaconIDs: idSet(t, "75"),
		},
	}

	got := mergePatchPolicyScope(plan, server)

	if v := got.Targets.ComputerGroupIDs.Elements(); len(v) != 1 {
		t.Errorf("declared category should win: got %v", v)
	}
	if got.Targets.BuildingIDs.IsNull() || len(got.Targets.BuildingIDs.Elements()) != 0 {
		t.Error("declared [] must merge as a non-null empty set (emits the empty wrapper)")
	}
	if v := got.Targets.DepartmentIDs.Elements(); len(v) != 1 {
		t.Errorf("omitted category should preserve live members: got %v", v)
	}
	if got.Limitations == nil || len(got.Limitations.NetworkSegmentIDs.Elements()) != 1 {
		t.Error("omitted limitations block must still merge from the live scope")
	}
	if got.Exclusions == nil || len(got.Exclusions.IbeaconIDs.Elements()) != 1 {
		t.Error("omitted exclusions block must still merge from the live scope")
	}
	// Every merged field must be non-null so the builder emits the full
	// explicit skeleton.
	if got.Targets.ComputerIDs.IsNull() || got.Targets.AllComputers.IsNull() ||
		got.Limitations.IbeaconIDs.IsNull() ||
		got.Exclusions.ComputerIDs.IsNull() || got.Exclusions.NetworkSegmentIDs.IsNull() {
		t.Error("merged output must have every category non-null")
	}
}

func TestMergePatchPolicyScope_NilServerTreatedAsEmpty(t *testing.T) {
	plan := &PatchPolicyScopeModel{
		Targets: &PatchPolicyScopeTargetsModel{
			ComputerGroupIDs: idSet(t, "10"),
		},
	}
	got := mergePatchPolicyScope(plan, nil)
	if v := got.Targets.ComputerGroupIDs.Elements(); len(v) != 1 {
		t.Errorf("expected declared members, got %v", v)
	}
	if got.Targets.DepartmentIDs.IsNull() || len(got.Targets.DepartmentIDs.Elements()) != 0 {
		t.Error("expected empty non-null set for omitted category with no live value")
	}
	if got.Targets.AllComputers.IsNull() || got.Targets.AllComputers.ValueBool() {
		t.Error("expected all_computers to default false, non-null")
	}
}

// TestMergePatchPolicyScope_AllComputersPrecedence pins the flag rule: a merged
// true all_computers empties exactly the categories it pairs with — computers
// and computer groups. Buildings and departments, limitations and exclusions
// all coexist with the flag (wire-probed 2026-10-14) and are preserved.
func TestMergePatchPolicyScope_AllComputersPrecedence(t *testing.T) {
	plan := &PatchPolicyScopeModel{
		Targets: &PatchPolicyScopeTargetsModel{
			AllComputers: types.BoolValue(true),
		},
	}
	server := &PatchPolicyScopeModel{
		Targets: &PatchPolicyScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			ComputerIDs:      idSet(t, "1"),
			ComputerGroupIDs: idSet(t, "99"),
			BuildingIDs:      idSet(t, "7"),
			DepartmentIDs:    idSet(t, "3"),
		},
		Limitations: &PatchPolicyScopeLimitationsModel{
			NetworkSegmentIDs: idSet(t, "50"),
		},
		Exclusions: &PatchPolicyScopeExclusionsModel{
			BuildingIDs: idSet(t, "55"),
		},
	}
	got := mergePatchPolicyScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers=true")
	}
	for label, set := range map[string]types.Set{
		"computer_ids":       got.Targets.ComputerIDs,
		"computer_group_ids": got.Targets.ComputerGroupIDs,
	} {
		if set.IsNull() || len(set.Elements()) != 0 {
			t.Errorf("expected target %s emptied under all_computers=true, got %v", label, set)
		}
	}
	for label, set := range map[string]types.Set{
		"building_ids":   got.Targets.BuildingIDs,
		"department_ids": got.Targets.DepartmentIDs,
	} {
		if len(set.Elements()) != 1 {
			t.Errorf("expected target %s preserved under all_computers=true, got %v", label, set)
		}
	}
	if len(got.Limitations.NetworkSegmentIDs.Elements()) != 1 {
		t.Error("expected limitations preserved under all_computers=true")
	}
	if len(got.Exclusions.BuildingIDs.Elements()) != 1 {
		t.Error("expected exclusions preserved under all_computers=true")
	}
}

// TestMergePatchPolicyScope_DeclaredGroupBeatsPreservedAllComputers is the #467
// shape on this endpoint: a declared group turns the unmanaged flag off.
func TestMergePatchPolicyScope_DeclaredGroupBeatsPreservedAllComputers(t *testing.T) {
	plan := &PatchPolicyScopeModel{Targets: &PatchPolicyScopeTargetsModel{
		ComputerGroupIDs: idSet(t, "58"),
	}}
	server := &PatchPolicyScopeModel{Targets: &PatchPolicyScopeTargetsModel{
		AllComputers: types.BoolValue(true),
	}}
	got := mergePatchPolicyScope(plan, server)
	if got.Targets.AllComputers.ValueBool() {
		t.Error("expected all_computers=false")
	}
	if len(got.Targets.ComputerGroupIDs.Elements()) != 1 {
		t.Errorf("expected declared group kept, got %v", got.Targets.ComputerGroupIDs)
	}
}

func TestMergePatchPolicyScope_DeclaredBuildingKeepsPreservedAllComputers(t *testing.T) {
	plan := &PatchPolicyScopeModel{Targets: &PatchPolicyScopeTargetsModel{
		BuildingIDs: idSet(t, "7"),
	}}
	server := &PatchPolicyScopeModel{Targets: &PatchPolicyScopeTargetsModel{
		AllComputers: types.BoolValue(true),
	}}
	got := mergePatchPolicyScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the preserved all_computers=true to stay")
	}
	if len(got.Targets.BuildingIDs.Elements()) != 1 {
		t.Errorf("expected declared building kept, got %v", got.Targets.BuildingIDs)
	}
}
