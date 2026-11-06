// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

const (
	groupA = "4bae0c78-1c40-4d7b-a388-30303893b77f"
	groupB = "32743179-0e40-42fd-9e0d-7cf404314888"
)

func uuidSet(t *testing.T, vals ...string) types.Set {
	t.Helper()
	out, diags := types.SetValueFrom(context.Background(), types.StringType, vals)
	if diags.HasError() {
		t.Fatalf("building set: %v", diags)
	}
	return out
}

func TestBlueprintImpactScopeCountsDeviceGroups(t *testing.T) {
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: uuidSet(t, groupA),
	})
	if got.DeviceType != impact.DeviceTypeAny {
		t.Fatalf("a blueprint can target either estate, so the device type must span both: %v", got.DeviceType)
	}
	if len(got.PlatformGroupIDs) != 1 || got.PlatformGroupIDs[0] != groupA {
		t.Fatalf("device_groups must be counted: %v", got.PlatformGroupIDs)
	}
	if len(got.Unresolvable) != 0 {
		t.Fatalf("nothing is unresolvable here, got %+v", got.Unresolvable)
	}
}

func TestBlueprintImpactScopeNamesActivationConditionGroupsWithoutCountingThem(t *testing.T) {
	// An activation condition can require a group or rule one out, so its groups
	// are surfaced by name but must not move the figure.
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: uuidSet(t, groupA),
		ComponentBlocks: []ComponentBlockModel{
			{ActivationConditions: types.StringValue("ANY @property(jamf.device.groups) IN {'" + groupB + "'}")},
		},
	})
	if len(got.PlatformGroupIDs) != 1 || got.PlatformGroupIDs[0] != groupA {
		t.Fatalf("only device_groups may be counted: %v", got.PlatformGroupIDs)
	}
	if len(got.MentionedPlatformIDs) != 1 || got.MentionedPlatformIDs[0] != groupB {
		t.Fatalf("the expression's group must be surfaced as mentioned: %v", got.MentionedPlatformIDs)
	}
	if len(got.Unresolvable) != 1 || got.Unresolvable[0].Effect != impact.Ambiguous {
		t.Fatalf("an expression bounds the figure from neither side, got %+v", got.Unresolvable)
	}
	if got.Unresolvable[0].Path != "activation_conditions" {
		t.Fatalf("caveat path wrong: %q", got.Unresolvable[0].Path)
	}
}

func TestBlueprintImpactScopeCollectsComponentBlockConditions(t *testing.T) {
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: uuidSet(t),
		ComponentBlocks: []ComponentBlockModel{
			{Name: types.StringValue("one"), ActivationConditions: types.StringValue("ANY @property(jamf.device.groups) IN {'" + groupA + "'}")},
			{Name: types.StringValue("two"), ActivationConditions: types.StringNull()},
		},
	})
	if len(got.MentionedPlatformIDs) != 1 || got.MentionedPlatformIDs[0] != groupA {
		t.Fatalf("a block's condition groups must be surfaced: %v", got.MentionedPlatformIDs)
	}
}

func TestBlueprintImpactScopeDeduplicatesRepeatedMentions(t *testing.T) {
	expr := "ANY @property(jamf.device.groups) IN {'" + groupA + "','" + groupA + "'}"
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: uuidSet(t),
		ComponentBlocks: []ComponentBlockModel{
			{ActivationConditions: types.StringValue(expr)},
			{ActivationConditions: types.StringValue(expr)},
		},
	})
	if len(got.MentionedPlatformIDs) != 1 {
		t.Fatalf("a repeated group must be reported once, got %v", got.MentionedPlatformIDs)
	}
	if got.Unresolvable[0].Values != 1 {
		t.Fatalf("the caveat count must match the deduplicated list, got %d", got.Unresolvable[0].Values)
	}
}

func TestBlueprintImpactScopeFlagsConditionsWithNoGroups(t *testing.T) {
	// An OS-version condition names no group yet still gates which devices the
	// blueprint applies to, so the figure must carry the ambiguity caveat.
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: uuidSet(t, groupA),
		ComponentBlocks: []ComponentBlockModel{
			{Name: types.StringValue("one"), ActivationConditions: types.StringValue("ANY @property(jamf.device.osVersion) >= '15.0'")},
			{Name: types.StringValue("two"), ActivationConditions: types.StringValue("ANY @property(jamf.device.modelName) CONTAINS 'MacBook'")},
			{Name: types.StringValue("three"), ActivationConditions: types.StringNull()},
		},
	})
	if len(got.MentionedPlatformIDs) != 0 {
		t.Fatalf("a condition naming no group must surface no mentioned groups, got %v", got.MentionedPlatformIDs)
	}
	if len(got.Unresolvable) != 1 || got.Unresolvable[0].Effect != impact.Ambiguous {
		t.Fatalf("a group-free condition still gates the audience, got %+v", got.Unresolvable)
	}
	if got.Unresolvable[0].Path != "activation_conditions" {
		t.Fatalf("caveat path wrong: %q", got.Unresolvable[0].Path)
	}
	if got.Unresolvable[0].Values != 2 {
		t.Fatalf("with no groups the caveat counts the non-empty expressions, got %d", got.Unresolvable[0].Values)
	}
}

func TestBlueprintImpactScopeIgnoresBlankConditions(t *testing.T) {
	// An expression that is empty or whitespace gates nothing and must not
	// caveat the figure.
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: uuidSet(t, groupA),
		ComponentBlocks: []ComponentBlockModel{
			{Name: types.StringValue("one"), ActivationConditions: types.StringValue("   ")},
			{Name: types.StringValue("two"), ActivationConditions: types.StringValue("")},
		},
	})
	if len(got.Unresolvable) != 0 {
		t.Fatalf("a blank expression must not add a caveat, got %+v", got.Unresolvable)
	}
}

func TestBlueprintImpactScopeUnknownDeviceGroupsIsPending(t *testing.T) {
	got := blueprintImpactScope(context.Background(), &BlueprintResourceModel{
		DeviceGroups: types.SetUnknown(types.StringType),
	})
	if len(got.PendingPaths) != 1 || got.PendingPaths[0] != "device_groups" {
		t.Fatalf("a group created by this plan must be recorded as pending, got %v", got.PendingPaths)
	}
}

func TestBlueprintImpactScopeNilModel(t *testing.T) {
	if got := blueprintImpactScope(context.Background(), nil); !got.Empty() {
		t.Fatalf("a nil model must yield an empty scope, got %+v", got)
	}
}
