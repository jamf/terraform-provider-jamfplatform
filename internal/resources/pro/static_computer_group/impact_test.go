// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

func TestMembershipChange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		planned types.Set
		current types.Set
		action  impact.Action

		wantCurrent      int64
		wantCurrentKnown bool
		wantNext         int64
		wantNextKnown    bool
		wantChanged      bool
	}{
		{
			name:          "a managed create counts the configured set",
			planned:       stringSet(t, "11", "12"),
			current:       types.SetNull(types.StringType),
			action:        impact.ActionCreate,
			wantNext:      2,
			wantNextKnown: true,
		},
		{
			// An unmanaged create still writes an empty assignment list, so the
			// new group reliably holds nothing.
			name:          "an unmanaged create reports an empty group",
			planned:       types.SetNull(types.StringType),
			current:       types.SetNull(types.StringType),
			action:        impact.ActionCreate,
			wantNext:      0,
			wantNextKnown: true,
		},
		{
			name:             "a managed update counts both sides",
			planned:          stringSet(t, "11"),
			current:          stringSet(t, "11", "12", "13"),
			action:           impact.ActionUpdate,
			wantCurrent:      3,
			wantCurrentKnown: true,
			wantNext:         1,
			wantNextKnown:    true,
			wantChanged:      true,
		},
		{
			// A rename or a description edit leaves the sets equal, and Jamf Pro
			// alerts on a membership edit rather than on a rename.
			name:             "an edit that leaves membership alone is not a change",
			planned:          stringSet(t, "11"),
			current:          stringSet(t, "11"),
			action:           impact.ActionUpdate,
			wantCurrent:      1,
			wantCurrentKnown: true,
			wantNext:         1,
			wantNextKnown:    true,
			wantChanged:      false,
		},
		{
			name:        "an unmanaged update knows nothing and changes nothing",
			planned:     types.SetNull(types.StringType),
			current:     types.SetNull(types.StringType),
			action:      impact.ActionUpdate,
			wantChanged: false,
		},
		{
			// Dropping the attribute hands membership back to Jamf Pro. Update
			// reads the current members and sends them straight back, so nothing
			// about the membership changes and no count is reportable, whatever
			// state still records from when it was managed.
			name:             "an update that stops managing membership changes nothing",
			planned:          types.SetNull(types.StringType),
			current:          stringSet(t, "11", "12"),
			action:           impact.ActionUpdate,
			wantCurrentKnown: false,
			wantChanged:      false,
		},
		{
			// Emptying the group is the edit that looks the same in the
			// configuration and is the opposite on the wire: `[]` is a managed
			// membership of none, and every member leaves.
			name:             "an update that empties the group is a change",
			planned:          types.SetValueMust(types.StringType, nil),
			current:          stringSet(t, "11", "12"),
			action:           impact.ActionUpdate,
			wantCurrent:      2,
			wantCurrentKnown: true,
			wantNext:         0,
			wantNextKnown:    true,
			wantChanged:      true,
		},
		{
			name:             "a delete counts what is leaving",
			planned:          types.SetNull(types.StringType),
			current:          stringSet(t, "11", "12"),
			action:           impact.ActionDelete,
			wantCurrent:      2,
			wantCurrentKnown: true,
		},
		{
			name:             "an unknown planned set reports no next count",
			planned:          types.SetUnknown(types.StringType),
			current:          stringSet(t, "11"),
			action:           impact.ActionUpdate,
			wantCurrent:      1,
			wantCurrentKnown: true,
			wantChanged:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := membershipChange(tc.planned, tc.current, tc.action)

			if got.Noun != memberNoun {
				t.Errorf("noun = %q, want %q", got.Noun, memberNoun)
			}
			// Nothing here is derived from criteria, so there is never an
			// undetermined membership to explain.
			if got.Undetermined != "" {
				t.Errorf("undetermined = %q, want empty for a static group", got.Undetermined)
			}
			if got.CurrentKnown != tc.wantCurrentKnown {
				t.Errorf("currentKnown = %v, want %v", got.CurrentKnown, tc.wantCurrentKnown)
			}
			if got.CurrentKnown && got.Current != tc.wantCurrent {
				t.Errorf("current = %d, want %d", got.Current, tc.wantCurrent)
			}
			if got.NextKnown != tc.wantNextKnown {
				t.Errorf("nextKnown = %v, want %v", got.NextKnown, tc.wantNextKnown)
			}
			if got.NextKnown && got.Next != tc.wantNext {
				t.Errorf("next = %d, want %d", got.Next, tc.wantNext)
			}
			if got.Changed != tc.wantChanged {
				t.Errorf("changed = %v, want %v", got.Changed, tc.wantChanged)
			}
		})
	}
}

// TestMembershipChange_UnmanagedUpdateRaisesNoAlert pins the consequence of the
// two unmanaged update rows above, which is what the shared reporter reads:
// Changed false takes it out on its first gate, and neither count being known
// takes it out on the second, so there is no path to a warning either way. That
// matters most for the group whose membership state still records, because there
// the reporter would otherwise have a count to print.
func TestMembershipChange_UnmanagedUpdateRaisesNoAlert(t *testing.T) {
	for _, current := range []types.Set{types.SetNull(types.StringType), stringSet(t, "11", "12")} {
		m := membershipChange(types.SetNull(types.StringType), current, impact.ActionUpdate)
		if m.Changed {
			t.Errorf("changed = true for an update that does not manage membership, current %v", current)
		}
		if m.CurrentKnown || m.NextKnown || m.Undetermined != "" {
			t.Errorf("expected nothing to report for current %v, got %+v", current, m)
		}
	}
}

// TestMemberNounIsSharedWithTheFamily keeps the counted noun coming from the
// shared renderer, so the four constructs cannot count in different words.
func TestMemberNounIsSharedWithTheFamily(t *testing.T) {
	if memberNoun != "computers" {
		t.Errorf("memberNoun = %q, want %q", memberNoun, "computers")
	}
	if groupLabel != "static computer group" {
		t.Errorf("groupLabel = %q, want %q", groupLabel, "static computer group")
	}
}
