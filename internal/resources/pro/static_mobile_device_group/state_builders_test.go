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

func TestAssignResourceModel_TakesTheJamfProIdentifierFromTheBody(t *testing.T) {
	// Create addresses the group by its platform identifier and learns the
	// numeric one from the read that follows, so the body's identifier has to
	// land in state.
	state := StaticMobileDeviceGroupResourceModel{
		ID:          types.StringNull(),
		Description: types.StringNull(),
	}
	assignResourceModel(&state, &pro.StaticGroup{
		Count:            4,
		GroupID:          "17",
		GroupName:        "Loaners",
		GroupDescription: "Spares desk",
		SiteID:           "3",
	})

	if state.ID.ValueString() != "17" {
		t.Errorf("id = %q, want 17", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Loaners" {
		t.Errorf("name = %q", state.Name.ValueString())
	}
	if state.Description.ValueString() != "Spares desk" {
		t.Errorf("description = %q", state.Description.ValueString())
	}
	if state.SiteID.ValueString() != "3" {
		t.Errorf("site_id = %q", state.SiteID.ValueString())
	}
	if state.MemberCount.ValueInt64() != 4 {
		t.Errorf("member_count = %d, want 4", state.MemberCount.ValueInt64())
	}
}

func TestAssignResourceModel_NormalisesTheNoSiteSentinel(t *testing.T) {
	state := StaticMobileDeviceGroupResourceModel{Description: types.StringNull()}
	assignResourceModel(&state, &pro.StaticGroup{GroupID: "1", GroupName: "x", SiteID: ""})

	if state.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("site_id = %q, want %q", state.SiteID.ValueString(), progroups.NoSiteID)
	}
}

func TestAssignResourceModel_KeepsAnExplicitEmptyDescription(t *testing.T) {
	// An empty echo means "no description". Collapsing it to null would wipe a
	// practitioner's deliberate empty string every refresh.
	state := StaticMobileDeviceGroupResourceModel{Description: types.StringValue("")}
	assignResourceModel(&state, &pro.StaticGroup{GroupID: "1", GroupName: "x", GroupDescription: ""})

	if state.Description.IsNull() {
		t.Error("an authored empty description must survive an empty echo")
	}
}

func TestAssignResourceModel_ReportsAnUnsetDescriptionAsNull(t *testing.T) {
	state := StaticMobileDeviceGroupResourceModel{Description: types.StringNull()}
	assignResourceModel(&state, &pro.StaticGroup{GroupID: "1", GroupName: "x", GroupDescription: ""})

	if !state.Description.IsNull() {
		t.Errorf("description = %q, want null", state.Description.ValueString())
	}
}

func TestAssignResourceModel_IgnoresANilBody(t *testing.T) {
	state := StaticMobileDeviceGroupResourceModel{Name: types.StringValue("kept")}
	assignResourceModel(&state, nil)
	if state.Name.ValueString() != "kept" {
		t.Error("a nil body must leave state alone")
	}
}

func TestAssignMembership(t *testing.T) {
	tests := []struct {
		name      string
		ids       []string
		manage    bool
		hydrating bool
		wantNull  bool
		wantLen   int
	}{
		{
			name:     "an unmanaged group stays null however many members Jamf Pro reports",
			ids:      []string{"1", "2"},
			wantNull: true,
		},
		{
			name:    "a managed group takes the reported members",
			ids:     []string{"1", "2"},
			manage:  true,
			wantLen: 2,
		},
		{
			name:    "a managed group with no members stores the empty set",
			ids:     nil,
			manage:  true,
			wantLen: 0,
		},
		{
			name:      "an import adopts the members Jamf Pro reports",
			ids:       []string{"5"},
			hydrating: true,
			wantLen:   1,
		},
		{
			name:      "an import of a memberless group keeps it null",
			ids:       nil,
			hydrating: true,
			wantNull:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := StaticMobileDeviceGroupResourceModel{}
			diags := assignMembership(context.Background(), &state, tc.ids, tc.manage, tc.hydrating)
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}
			if tc.wantNull {
				if !state.AssignedMobileDeviceIDs.IsNull() {
					t.Fatalf("assigned_mobile_device_ids = %v, want null", state.AssignedMobileDeviceIDs)
				}
				return
			}
			if state.AssignedMobileDeviceIDs.IsNull() {
				t.Fatal("assigned_mobile_device_ids must not be null")
			}
			if got := len(state.AssignedMobileDeviceIDs.Elements()); got != tc.wantLen {
				t.Errorf("membership size = %d, want %d", got, tc.wantLen)
			}
		})
	}
}

func TestMembershipIDs_SkipsADeviceWithNoIdentifier(t *testing.T) {
	got := membershipIDs([]pro.InventoryListMobileDevice{
		{MobileDeviceID: "11"},
		{MobileDeviceID: ""},
		{MobileDeviceID: "12"},
	})
	if len(got) != 2 || got[0] != "11" || got[1] != "12" {
		t.Errorf("membershipIDs = %v, want [11 12]", got)
	}
}

func TestAssignDataSourceModel(t *testing.T) {
	data := StaticMobileDeviceGroupDataSourceModel{}
	assignDataSourceModel(&data, &pro.StaticGroup{
		Count:            2,
		GroupID:          "17",
		GroupName:        "Loaners",
		GroupDescription: "",
		SiteID:           "-1",
	})

	if data.ID.ValueString() != "17" {
		t.Errorf("id = %q", data.ID.ValueString())
	}
	if !data.Description.IsNull() {
		t.Error("a group with no description must read as null")
	}
	if data.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("site_id = %q", data.SiteID.ValueString())
	}
	if data.MemberCount.ValueInt64() != 2 {
		t.Errorf("member_count = %d", data.MemberCount.ValueInt64())
	}
}

func TestFlattenMembershipIDs_PreservesOrder(t *testing.T) {
	got := flattenMembershipIDs([]string{"9", "2"})
	if len(got) != 2 || got[0].ValueString() != "9" || got[1].ValueString() != "2" {
		t.Errorf("flattenMembershipIDs = %v, want the reported order", got)
	}
}

func TestPluralResultModel_TakesThePlatformIdentifierFromTheLookup(t *testing.T) {
	got := pluralResultModel(
		pro.StaticGroup{Count: 1, GroupID: "17", GroupName: "Loaners", SiteID: "-1"},
		map[string]string{"17": "0cf0f2e7-0000-4000-8000-000000000001"},
	)
	if got.PlatformID.ValueString() != "0cf0f2e7-0000-4000-8000-000000000001" {
		t.Errorf("platform_id = %q", got.PlatformID.ValueString())
	}
	if got.MemberCount.ValueInt64() != 1 {
		t.Errorf("member_count = %d", got.MemberCount.ValueInt64())
	}
}

func TestPluralResultModel_NullsAPlatformIdentifierTheLookupMissed(t *testing.T) {
	got := pluralResultModel(pro.StaticGroup{GroupID: "17", GroupName: "Loaners"}, nil)
	if !got.PlatformID.IsNull() {
		t.Errorf("platform_id = %q, want null", got.PlatformID.ValueString())
	}
}

func TestListResultModel_LeavesMembershipUnmanaged(t *testing.T) {
	// The list reads one page and never fans out a membership request, so a
	// generated configuration must not claim to own the group's members.
	got := listResultModel(pro.StaticGroup{GroupID: "17", GroupName: "Loaners", SiteID: "-1"}, nil)
	if !got.AssignedMobileDeviceIDs.IsNull() {
		t.Error("a list result must leave assigned_mobile_device_ids null")
	}
	if got.Timeouts.IsUnknown() {
		t.Error("a list result must carry a typed timeouts value")
	}
}

func TestDescriptionValue(t *testing.T) {
	if !descriptionValue("").IsNull() {
		t.Error("an empty description must read as null")
	}
	if got := descriptionValue("Spares desk"); got.ValueString() != "Spares desk" {
		t.Errorf("descriptionValue = %q", got.ValueString())
	}
}
