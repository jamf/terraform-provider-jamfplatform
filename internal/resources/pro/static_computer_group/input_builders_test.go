// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

func stringSet(t *testing.T, values ...string) types.Set {
	t.Helper()
	set, diags := types.SetValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building a set from %v: %v", values, diags)
	}
	return set
}

func TestBuildStaticComputerGroupInput_EmitsEveryScalar(t *testing.T) {
	got := buildStaticComputerGroupInput(StaticComputerGroupResourceModel{
		Name:        types.StringValue("Lab Macs"),
		Description: types.StringValue("Ground floor"),
		SiteID:      types.StringValue("7"),
	}, []string{"3", "4"})

	if got.Name != "Lab Macs" {
		t.Errorf("name = %q, want %q", got.Name, "Lab Macs")
	}
	if got.Description == nil || *got.Description != "Ground floor" {
		t.Errorf("description = %v, want a pointer to %q", got.Description, "Ground floor")
	}
	if got.SiteID == nil || *got.SiteID != "7" {
		t.Errorf("siteId = %v, want a pointer to %q", got.SiteID, "7")
	}
	if got.Assignments == nil || len(*got.Assignments) != 2 {
		t.Fatalf("assignments = %v, want two entries", got.Assignments)
	}
	// ID is the caller's path parameter, never part of the body.
	if got.ID != nil {
		t.Errorf("id must not be sent in the body, got %v", got.ID)
	}
}

// TestBuildStaticComputerGroupInput_UnsetScalarsStillTravel pins the
// always-emit rule: these endpoints full-replace their scalars, so a body that
// omits description clears it and one that omits siteId clears the site.
func TestBuildStaticComputerGroupInput_UnsetScalarsStillTravel(t *testing.T) {
	got := buildStaticComputerGroupInput(StaticComputerGroupResourceModel{
		Name:        types.StringValue("Lab Macs"),
		Description: types.StringUnknown(),
		SiteID:      types.StringNull(),
	}, nil)

	if got.Description == nil {
		t.Error("description must be sent even when the plan carries no value")
	} else if *got.Description != "" {
		t.Errorf("an unset description must travel as the empty string, got %q", *got.Description)
	}
	if got.SiteID == nil || *got.SiteID != progroups.NoSiteID {
		t.Errorf("an unset site must travel as the no-site sentinel, got %v", got.SiteID)
	}
}

// TestBuildStaticComputerGroupInput_AssignmentsAreNeverOmitted pins the reason
// membership is synthesised rather than omitted: a body without the assignments
// key answers 500 and applies nothing.
func TestBuildStaticComputerGroupInput_AssignmentsAreNeverOmitted(t *testing.T) {
	got := buildStaticComputerGroupInput(StaticComputerGroupResourceModel{Name: types.StringValue("Lab Macs")}, nil)
	if got.Assignments == nil {
		t.Fatal("assignments must always be present in the body")
	}
	if len(*got.Assignments) != 0 {
		t.Errorf("a nil membership must travel as an empty list, got %v", *got.Assignments)
	}
}

func TestConfiguredAssignments(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		set  types.Set
		want []string
	}{
		{"an unmanaged set empties the list", types.SetNull(types.StringType), []string{}},
		{"an unknown set empties the list", types.SetUnknown(types.StringType), []string{}},
		{"an explicitly empty set empties the list", stringSet(t), []string{}},
		{"a populated set is carried through", stringSet(t, "11", "12"), []string{"11", "12"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := configuredAssignments(ctx, tc.set)
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}
			if got == nil {
				t.Fatal("result must never be nil — the write has to carry an explicit list")
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("element %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
