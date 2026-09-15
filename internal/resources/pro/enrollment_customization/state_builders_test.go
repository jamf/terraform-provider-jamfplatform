// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignParentToResource_PopulatesAllFields(t *testing.T) {
	id := "42"
	got := &pro.EnrollmentCustomizationV2{
		ID:          &id,
		DisplayName: "Welcome",
		Description: "desc",
		SiteID:      "-1",
		EnrollmentCustomizationBrandingSettings: pro.EnrollmentCustomizationBrandingSettings{
			TextColor:       "111111",
			ButtonColor:     "222222",
			ButtonTextColor: "333333",
			BackgroundColor: "444444",
			IconURL:         "https://x/i.png",
		},
	}
	var state EnrollmentCustomizationResourceModel
	assignParentToResource(&state, got)

	if state.ID.ValueString() != "42" {
		t.Fatalf("ID = %q", state.ID.ValueString())
	}
	if state.SiteID.ValueString() != "-1" {
		t.Fatalf("SiteID = %q", state.SiteID.ValueString())
	}
	if state.BrandingSettings == nil || state.BrandingSettings.BodyTextColor.ValueString() != "111111" {
		t.Fatalf("BodyTextColor mapping wrong: %+v", state.BrandingSettings)
	}
	if state.BrandingSettings.IconURL.ValueString() != "https://x/i.png" {
		t.Fatalf("IconURL = %q", state.BrandingSettings.IconURL.ValueString())
	}
}

func TestAssignSsoPanel_EnrollmentAccessBoolToEnum(t *testing.T) {
	// true → "specific_group".
	specific := assignSsoPanel(&pro.GetEnrollmentCustomizationPanelSsoAuth{
		ID:                             5,
		DisplayName:                    "sso",
		Rank:                           1,
		IsGroupEnrollmentAccessEnabled: true,
		GroupEnrollmentAccessName:      "Admins",
		IsUseJamfConnect:               true,
		ShortNameAttribute:             "uid",
		LongNameAttribute:              "name",
	})
	if specific.EnrollmentAccess.ValueString() != enrollmentAccessSpecificGroup {
		t.Fatalf("true should reverse to %q, got %q", enrollmentAccessSpecificGroup, specific.EnrollmentAccess.ValueString())
	}
	if specific.AccessGroupName.IsNull() || specific.AccessGroupName.ValueString() != "Admins" {
		t.Fatalf("AccessGroupName = %v", specific.AccessGroupName)
	}

	// false → "any_idp_user", empty group name → null.
	any := assignSsoPanel(&pro.GetEnrollmentCustomizationPanelSsoAuth{
		ID:                             6,
		DisplayName:                    "sso2",
		Rank:                           2,
		IsGroupEnrollmentAccessEnabled: false,
		GroupEnrollmentAccessName:      "",
	})
	if any.EnrollmentAccess.ValueString() != enrollmentAccessAnyIdpUser {
		t.Fatalf("false should reverse to %q, got %q", enrollmentAccessAnyIdpUser, any.EnrollmentAccess.ValueString())
	}
	if !any.AccessGroupName.IsNull() {
		t.Fatalf("AccessGroupName should be null when empty wire value, got %v", any.AccessGroupName)
	}
}

func TestAssignLdapPanel_GroupAccessRoundTrip(t *testing.T) {
	name1 := "g1"
	id1 := 7
	got := assignLdapPanel(&pro.GetEnrollmentCustomizationPanelLdapAuth{
		ID:                 9,
		DisplayName:        "ldap",
		Rank:               0,
		Title:              "t",
		UsernameLabel:      "u",
		PasswordLabel:      "p",
		BackButtonText:     "prev",
		ContinueButtonText: "login",
		LdapGroupAccess: []pro.EnrollmentCustomizationLdapGroupAccess{
			{GroupName: &name1, LdapServerID: &id1},
		},
	})
	if got.LoginButtonText.ValueString() != "login" {
		t.Fatalf("continueButtonText should map to login_button_text, got %q", got.LoginButtonText.ValueString())
	}
	if len(got.DirectoryServiceGroups) != 1 {
		t.Fatalf("group access len = %d", len(got.DirectoryServiceGroups))
	}
	g := got.DirectoryServiceGroups[0]
	if g.GroupName.ValueString() != "g1" || g.DirectoryServiceServerID.ValueInt64() != 7 {
		t.Fatalf("group mapping wrong: %+v", g)
	}
}

func TestSortByRank_StableTieBreakOnID(t *testing.T) {
	panes := []textPaneModel{
		{ID: types.StringValue("3"), Rank: types.Int64Value(1)},
		{ID: types.StringValue("1"), Rank: types.Int64Value(0)},
		{ID: types.StringValue("2"), Rank: types.Int64Value(1)},
	}
	sortTextByRank(panes)
	if panes[0].ID.ValueString() != "1" {
		t.Fatalf("rank 0 should come first; got %q", panes[0].ID.ValueString())
	}
	// Ties on rank=1 should sort 2 < 3 lexicographically.
	if panes[1].ID.ValueString() != "2" || panes[2].ID.ValueString() != "3" {
		t.Fatalf("tie-break order wrong: %q,%q", panes[1].ID.ValueString(), panes[2].ID.ValueString())
	}
}
