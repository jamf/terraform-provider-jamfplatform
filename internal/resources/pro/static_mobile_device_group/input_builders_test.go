// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// stringSet builds a configured set for a test plan.
func stringSet(t *testing.T, ids ...string) types.Set {
	t.Helper()
	values := make([]types.String, 0, len(ids))
	for _, id := range ids {
		values = append(values, types.StringValue(id))
	}
	set, diags := types.SetValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building set: %v", diags)
	}
	return set
}

// entry renders one delta entry as a comparable pair.
type entry struct {
	id       string
	selected bool
}

// entries flattens a delta for assertion.
func entries(t *testing.T, in []pro.Assignment) []entry {
	t.Helper()
	out := make([]entry, 0, len(in))
	for _, a := range in {
		if a.MobileDeviceID == nil || a.Selected == nil {
			t.Fatalf("assignment with a nil field: %+v", a)
		}
		out = append(out, entry{id: *a.MobileDeviceID, selected: *a.Selected})
	}
	return out
}

func TestBuildGroupInput_AlwaysEmitsEveryScalarAndTheAssignmentsKey(t *testing.T) {
	plan := StaticMobileDeviceGroupResourceModel{
		Name:        types.StringValue("Loaners"),
		Description: types.StringNull(),
		SiteID:      types.StringValue(progroups.NoSiteID),
	}

	got := buildGroupInput(plan, nil)

	if got.GroupName != "Loaners" {
		t.Errorf("groupName = %q, want %q", got.GroupName, "Loaners")
	}
	if got.GroupDescription == nil {
		t.Fatal("groupDescription must be emitted: omitting it empties the description")
	}
	if *got.GroupDescription != "" {
		t.Errorf("groupDescription = %q, want the empty string", *got.GroupDescription)
	}
	if got.SiteID == nil {
		t.Fatal("siteId must be emitted: omitting it is refused outright on this endpoint")
	}
	if *got.SiteID != progroups.NoSiteID {
		t.Errorf("siteId = %q, want %q", *got.SiteID, progroups.NoSiteID)
	}
	if got.Assignments == nil {
		t.Fatal("assignments must be emitted even when empty: omitting it applies nothing at all")
	}
	if len(*got.Assignments) != 0 {
		t.Errorf("assignments = %v, want empty", *got.Assignments)
	}
}

func TestBuildGroupInput_UnknownDescriptionSendsTheEmptyString(t *testing.T) {
	plan := StaticMobileDeviceGroupResourceModel{
		Name:        types.StringValue("Loaners"),
		Description: types.StringUnknown(),
		SiteID:      types.StringValue("3"),
	}

	got := buildGroupInput(plan, nil)
	if got.GroupDescription == nil || *got.GroupDescription != "" {
		t.Errorf("an unconfigured description must send the empty string, got %v", got.GroupDescription)
	}
	if got.SiteID == nil || *got.SiteID != "3" {
		t.Errorf("siteId = %v, want 3", got.SiteID)
	}
}

func TestBuildGroupInput_CarriesTheSuppliedDelta(t *testing.T) {
	plan := StaticMobileDeviceGroupResourceModel{
		Name:        types.StringValue("Loaners"),
		Description: types.StringValue("Spares desk"),
		SiteID:      types.StringValue(progroups.NoSiteID),
	}

	got := buildGroupInput(plan, assignmentsFor([]string{"7"}, false))
	if *got.GroupDescription != "Spares desk" {
		t.Errorf("groupDescription = %q", *got.GroupDescription)
	}
	want := []entry{{id: "7", selected: false}}
	if diff := entries(t, *got.Assignments); !equalEntries(diff, want) {
		t.Errorf("assignments = %v, want %v", diff, want)
	}
}

// TestBuildCreateAssignments_ManagedMembershipIsAllAdditions also pins the
// ordering, which is the provider's own: a new group holds nobody, so every
// planned identifier is an addition and the request is sorted so it reproduces.
func TestBuildCreateAssignments_ManagedMembershipIsAllAdditions(t *testing.T) {
	got, diags := buildCreateAssignments(context.Background(), stringSet(t, "9", "2"))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	want := []entry{{id: "2", selected: true}, {id: "9", selected: true}}
	if diff := entries(t, got); !equalEntries(diff, want) {
		t.Errorf("assignments = %v, want %v", diff, want)
	}
}

func TestBuildCreateAssignments_UnmanagedMembershipIsAnEmptySlice(t *testing.T) {
	for name, set := range map[string]types.Set{
		"null":    types.SetNull(types.StringType),
		"unknown": types.SetUnknown(types.StringType),
	} {
		t.Run(name, func(t *testing.T) {
			got, diags := buildCreateAssignments(context.Background(), set)
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}
			if got == nil {
				t.Fatal("must be an empty slice, never nil: the key has to reach the wire")
			}
			if len(got) != 0 {
				t.Errorf("assignments = %v, want empty", got)
			}
		})
	}
}

func TestBuildCreateAssignments_EmptySetIsAnEmptyDelta(t *testing.T) {
	got, diags := buildCreateAssignments(context.Background(), stringSet(t))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(got) != 0 {
		t.Errorf("assignments = %v, want empty", got)
	}
}

func TestAssignmentsForUpdate(t *testing.T) {
	tests := []struct {
		name    string
		planned []string
		current []string
		want    []entry
	}{
		{
			name:    "an unchanged membership sends nothing",
			planned: []string{"1", "2"},
			current: []string{"2", "1"},
			want:    nil,
		},
		{
			name:    "a device the plan adds is selected",
			planned: []string{"1", "2"},
			current: []string{"1"},
			want:    []entry{{id: "2", selected: true}},
		},
		{
			name:    "a device the plan drops is deselected",
			planned: []string{"1"},
			current: []string{"1", "2"},
			want:    []entry{{id: "2", selected: false}},
		},
		{
			name:    "a swap sends both halves, additions first",
			planned: []string{"3"},
			current: []string{"1"},
			want:    []entry{{id: "3", selected: true}, {id: "1", selected: false}},
		},
		{
			name:    "clearing a group deselects every member",
			planned: nil,
			current: []string{"2", "1"},
			want:    []entry{{id: "1", selected: false}, {id: "2", selected: false}},
		},
		{
			name:    "filling an empty group selects every member",
			planned: []string{"2", "1"},
			current: nil,
			want:    []entry{{id: "1", selected: true}, {id: "2", selected: true}},
		},
		{
			name:    "a duplicated current member is named once",
			planned: nil,
			current: []string{"4", "4"},
			want:    []entry{{id: "4", selected: false}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := assignmentsForUpdate(tc.planned, tc.current)
			if got == nil {
				t.Fatal("must be an empty slice, never nil: the key has to reach the wire")
			}
			if diff := entries(t, got); !equalEntries(diff, tc.want) {
				t.Errorf("assignments = %v, want %v", diff, tc.want)
			}
		})
	}
}

func TestSortedUnique_DropsBlanksAndDuplicates(t *testing.T) {
	got := sortedUnique([]string{"3", "", "1", "3", "2"})
	want := []string{"1", "2", "3"}
	if len(got) != len(want) {
		t.Fatalf("sortedUnique = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedUnique = %v, want %v", got, want)
		}
	}
}

// equalEntries compares two deltas positionally, treating nil and empty as the
// same thing.
func equalEntries(a, b []entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
