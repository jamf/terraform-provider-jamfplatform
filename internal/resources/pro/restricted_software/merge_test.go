// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func strSet(t *testing.T, values ...string) types.Set {
	t.Helper()
	if len(values) == 0 {
		// A nil slice reflects to a null set; zero args means a known empty set.
		return scope.EmptyStringSet()
	}
	out, diags := types.SetValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("strSet: %v", diags)
	}
	return out
}

func TestMergeRestrictedSoftwareScope_NilPlanStaysNil(t *testing.T) {
	if got := mergeRestrictedSoftwareScope(nil, &RestrictedSoftwareScopeModel{}); got != nil {
		t.Fatalf("expected nil for unmanaged scope, got %+v", got)
	}
}

func TestMergeRestrictedSoftwareScope_DeclaredWinsOmittedPreserved(t *testing.T) {
	plan := &RestrictedSoftwareScopeModel{
		Targets: &RestrictedSoftwareScopeTargetsModel{
			ComputerGroupIDs: strSet(t, "10"), // declared: wins
			BuildingIDs:      strSet(t),       // declared []: clears
			// department_ids omitted: preserved from the live scope.
		},
		// exclusions omitted entirely: preserved.
	}
	server := &RestrictedSoftwareScopeModel{
		Targets: &RestrictedSoftwareScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			ComputerIDs:      scope.EmptyStringSet(),
			ComputerGroupIDs: strSet(t, "99"),
			BuildingIDs:      strSet(t, "7"),
			DepartmentIDs:    strSet(t, "3"),
		},
		Exclusions: &RestrictedSoftwareScopeExclusionsModel{
			DirectoryServiceOrLocalUserNames: strSet(t, "alice"),
		},
	}

	got := mergeRestrictedSoftwareScope(plan, server)

	if v := got.Targets.ComputerGroupIDs.Elements(); len(v) != 1 {
		t.Errorf("declared category should win: got %v", v)
	}
	if got.Targets.BuildingIDs.IsNull() || len(got.Targets.BuildingIDs.Elements()) != 0 {
		t.Error("declared [] must merge as a non-null empty set (emits the empty wrapper)")
	}
	if v := got.Targets.DepartmentIDs.Elements(); len(v) != 1 {
		t.Errorf("omitted category should preserve live members: got %v", v)
	}
	if got.Exclusions == nil {
		t.Fatal("omitted exclusions block must still merge from the live scope")
	}
	if v := got.Exclusions.DirectoryServiceOrLocalUserNames.Elements(); len(v) != 1 {
		t.Errorf("omitted exclusions category should preserve live members: got %v", v)
	}
	// Every merged field must be non-null so the builder emits the full
	// explicit skeleton — and the merge must not invent a limitations block
	// (the shape has none).
	if got.Targets.ComputerIDs.IsNull() || got.Targets.AllComputers.IsNull() ||
		got.Exclusions.ComputerIDs.IsNull() || got.Exclusions.ComputerGroupIDs.IsNull() ||
		got.Exclusions.BuildingIDs.IsNull() || got.Exclusions.DepartmentIDs.IsNull() {
		t.Error("merged output must have every category non-null")
	}
}

func TestMergeRestrictedSoftwareScope_NilServerTreatedAsEmpty(t *testing.T) {
	plan := &RestrictedSoftwareScopeModel{
		Targets: &RestrictedSoftwareScopeTargetsModel{
			ComputerGroupIDs: strSet(t, "10"),
		},
	}
	got := mergeRestrictedSoftwareScope(plan, nil)
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

// TestMergeRestrictedSoftwareScope_AllComputersPrecedence pins the flag rule: a
// merged true all_computers empties exactly the four target categories its
// validator names — never the exclusions, which coexist with the flag.
func TestMergeRestrictedSoftwareScope_AllComputersPrecedence(t *testing.T) {
	plan := &RestrictedSoftwareScopeModel{
		Targets: &RestrictedSoftwareScopeTargetsModel{
			AllComputers: types.BoolValue(true),
		},
	}
	server := &RestrictedSoftwareScopeModel{
		Targets: &RestrictedSoftwareScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			ComputerIDs:      strSet(t, "1"),
			ComputerGroupIDs: strSet(t, "99"),
			BuildingIDs:      strSet(t, "7"),
			DepartmentIDs:    strSet(t, "3"),
		},
		Exclusions: &RestrictedSoftwareScopeExclusionsModel{
			DirectoryServiceOrLocalUserNames: strSet(t, "alice"),
			BuildingIDs:                      strSet(t, "55"),
		},
	}
	got := mergeRestrictedSoftwareScope(plan, server)
	if !got.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers=true")
	}
	for label, set := range map[string]types.Set{
		"computer_ids":       got.Targets.ComputerIDs,
		"computer_group_ids": got.Targets.ComputerGroupIDs,
		"building_ids":       got.Targets.BuildingIDs,
		"department_ids":     got.Targets.DepartmentIDs,
	} {
		if set.IsNull() || len(set.Elements()) != 0 {
			t.Errorf("expected target %s emptied under all_computers=true, got %v", label, set)
		}
	}
	if len(got.Exclusions.DirectoryServiceOrLocalUserNames.Elements()) != 1 || len(got.Exclusions.BuildingIDs.Elements()) != 1 {
		t.Error("expected exclusions preserved under all_computers=true")
	}
}

// This resource honours the admin UI, which offers no target at all once All
// Computers is selected: unlike every other computer scope, buildings and
// departments pair with the flag too. So a declared building turns an
// unmanaged flag off, exactly as a declared computer group does.
func TestMergeRestrictedSoftwareScope_DeclaredTargetBeatsPreservedAllComputers(t *testing.T) {
	cases := map[string]RestrictedSoftwareScopeTargetsModel{
		"computer":       {ComputerIDs: strSet(t, "5")},
		"computer group": {ComputerGroupIDs: strSet(t, "58")},
		"building":       {BuildingIDs: strSet(t, "7")},
		"department":     {DepartmentIDs: strSet(t, "3")},
	}
	for name, targets := range cases {
		t.Run(name, func(t *testing.T) {
			targets := targets
			plan := &RestrictedSoftwareScopeModel{Targets: &targets}
			server := &RestrictedSoftwareScopeModel{Targets: &RestrictedSoftwareScopeTargetsModel{
				AllComputers: types.BoolValue(true),
			}}
			got := mergeRestrictedSoftwareScope(plan, server)
			if got.Targets.AllComputers.IsNull() || got.Targets.AllComputers.ValueBool() {
				t.Errorf("expected all_computers=false for a declared %s, got %v", name, got.Targets.AllComputers)
			}
		})
	}
}

func TestMergeRestrictedSoftwareScope_DeclaredEmptyKeepsPreservedAllComputers(t *testing.T) {
	plan := &RestrictedSoftwareScopeModel{Targets: &RestrictedSoftwareScopeTargetsModel{
		BuildingIDs: strSet(t),
	}}
	server := &RestrictedSoftwareScopeModel{Targets: &RestrictedSoftwareScopeTargetsModel{
		AllComputers: types.BoolValue(true),
	}}
	if got := mergeRestrictedSoftwareScope(plan, server); !got.Targets.AllComputers.ValueBool() {
		t.Error("expected the preserved all_computers=true to stay")
	}
}
