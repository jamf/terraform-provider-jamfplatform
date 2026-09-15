// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestAssignUserGroupResourceModel_Static_PopulatesMembers(t *testing.T) {
	id := 3
	ug := &proclassic.UserGroup{
		ID:               &id,
		Name:             new("Excluded Users"),
		IsSmart:          new(false),
		IsNotifyOnChange: new(false),
		Site:             &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		Users: &proclassic.UserGroupUsers{User: &[]proclassic.UserGroupUsersUserItem{
			{ID: new(9), Username: new("david@example.com"), FullName: new("David Norris"), EmailAddress: new("david@example.com")},
		}},
	}

	initialMembers, mDiags := types.SetValueFrom(context.Background(), types.StringType, []string{"99"})
	if mDiags.HasError() {
		t.Fatalf("initial members: %v", mDiags)
	}
	state := &UserGroupResourceModel{Members: initialMembers}
	diags := assignUserGroupResourceModel(context.Background(), state, ug, true, false)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "3" {
		t.Errorf("ID expected 3, got %s", state.ID)
	}
	if state.GroupType.ValueString() != "static" {
		t.Errorf("GroupType expected static, got %s", state.GroupType)
	}
	if state.SiteID.ValueString() != "-1" {
		t.Errorf("SiteID expected -1, got %s", state.SiteID)
	}
	// Sentinel site (id -1): derived name nulls, not the flaky server echo.
	if !state.SiteName.IsNull() {
		t.Errorf("SiteName expected null on the sentinel, got %s", state.SiteName)
	}
	if state.MemberCount.ValueInt64() != 1 {
		t.Errorf("MemberCount expected 1, got %d", state.MemberCount.ValueInt64())
	}
	if state.Members.IsNull() {
		t.Fatal("Members must be non-null when manageMembers=true on static group")
	}
}

func TestAssignUserGroupResourceModel_Smart_MembersAlwaysNull(t *testing.T) {
	id := 2
	ug := &proclassic.UserGroup{
		ID:      &id,
		Name:    new("Smart Group"),
		IsSmart: new(true),
		Site:    &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		Criteria: &proclassic.UserGroupCriteria{Criterion: &[]proclassic.Criterion{
			{Name: new("User Group"), Priority: new(0), AndOr: new("and"), SearchType: new("member of"), Value: new("All Managed Apple IDs"), OpeningParen: new(false), ClosingParen: new(false)},
		}},
		Users: &proclassic.UserGroupUsers{User: &[]proclassic.UserGroupUsersUserItem{
			{ID: new(6), Username: new("a@b.com")},
			{ID: new(7), Username: new("c@d.com")},
		}},
	}

	state := &UserGroupResourceModel{}
	diags := assignUserGroupResourceModel(context.Background(), state, ug, true, false) // manageMembers irrelevant for smart
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.GroupType.ValueString() != "smart" {
		t.Errorf("GroupType expected smart")
	}
	if !state.Members.IsNull() {
		t.Errorf("Members must be null for smart group, got %v", state.Members)
	}
	if state.MemberCount.ValueInt64() != 2 {
		t.Errorf("MemberCount still surfaces from <users> for smart groups: expected 2, got %d", state.MemberCount.ValueInt64())
	}
	if len(state.Criteria) != 1 {
		t.Errorf("Criteria expected 1, got %d", len(state.Criteria))
	}
}

// TestAssignUserGroupResourceModel_EmptyCriterionValueIsEmptyNotNull asserts
// that a criterion the server returns with an empty or absent value (e.g. an
// unset "before (yyyy-mm-dd)" date, as seen on a real smart group) round-trips
// to "" rather than null. The value attribute is Required, so null would fail
// validation on a faithful export.
func TestAssignUserGroupResourceModel_EmptyCriterionValueIsEmptyNotNull(t *testing.T) {
	id := 87
	ug := &proclassic.UserGroup{
		ID:      &id,
		Name:    new("Int and Date"),
		IsSmart: new(true),
		Criteria: &proclassic.UserGroupCriteria{Criterion: &[]proclassic.Criterion{
			{Name: new("Date EA"), Priority: new(0), AndOr: new("and"), SearchType: new("before (yyyy-mm-dd)"), Value: new(""), OpeningParen: new(false), ClosingParen: new(false)},
			{Name: new("Integer EA"), Priority: new(1), AndOr: new("and"), SearchType: new("is"), Value: nil, OpeningParen: new(false), ClosingParen: new(false)},
		}},
	}

	state := &UserGroupResourceModel{}
	if diags := assignUserGroupResourceModel(context.Background(), state, ug, false, false); diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(state.Criteria) != 2 {
		t.Fatalf("Criteria expected 2, got %d", len(state.Criteria))
	}
	for i, c := range state.Criteria {
		if c.Value.IsNull() {
			t.Errorf("criterion[%d].value must be empty string, not null", i)
		}
		if c.Value.ValueString() != "" {
			t.Errorf("criterion[%d].value expected \"\", got %q", i, c.Value.ValueString())
		}
	}
}

func TestAssignUserGroupDataSourceModel_PopulatesUsersBlock(t *testing.T) {
	id := 2
	ug := &proclassic.UserGroup{
		ID:      &id,
		Name:    new("Smart Group"),
		IsSmart: new(true),
		Users: &proclassic.UserGroupUsers{User: &[]proclassic.UserGroupUsersUserItem{
			{ID: new(6), Username: new("a@b.com"), FullName: new("Person A"), EmailAddress: new("a@b.com")},
		}},
	}

	state := &UserGroupDataSourceModel{}
	assignUserGroupDataSourceModel(state, ug)
	if len(state.Users) != 1 {
		t.Fatalf("Users expected 1, got %d", len(state.Users))
	}
	if state.Users[0].ID.ValueString() != "6" {
		t.Errorf("Users[0].ID expected 6, got %s", state.Users[0].ID)
	}
	if state.Users[0].FullName.ValueString() != "Person A" {
		t.Errorf("Users[0].FullName expected 'Person A'")
	}
}

func TestGroupTypeFromIsSmart(t *testing.T) {
	tests := []struct {
		in   *bool
		want string
	}{
		{new(true), "smart"},
		{new(false), "static"},
	}
	for _, tt := range tests {
		got := groupTypeFromIsSmart(tt.in)
		if got.ValueString() != tt.want {
			t.Errorf("isSmart=%v: expected %q, got %q", tt.in, tt.want, got.ValueString())
		}
	}
	if !groupTypeFromIsSmart(nil).IsNull() {
		t.Errorf("nil IsSmart must yield null")
	}
}

func TestFlattenSite(t *testing.T) {
	id, name := scope.FlattenSiteObject(&proclassic.SiteObject{ID: new(5), Name: new("Site5")})
	if id == nil || *id != "5" || name == nil || *name != "Site5" {
		t.Errorf("unexpected: id=%v name=%v", id, name)
	}
	id, name = scope.FlattenSiteObject(nil)
	if id != nil || name != nil {
		t.Errorf("nil site must yield (nil, nil)")
	}
}

// TestAssignUserGroupResourceModel_HydratesMembersOnImport covers the
// first-time import path: members arrives null (never declared, because there
// is no config yet) and must be adopted from the wire so a declared members
// list does not plan as an addition on the first plan after import.
func TestAssignUserGroupResourceModel_HydratesMembersOnImport(t *testing.T) {
	id := 4
	ug := &proclassic.UserGroup{
		ID:      &id,
		Name:    new("Imported Static"),
		IsSmart: new(false),
		Users: &proclassic.UserGroupUsers{User: &[]proclassic.UserGroupUsersUserItem{
			{ID: new(9), Username: new("david@example.com")},
			{ID: new(11), Username: new("erin@example.com")},
		}},
	}

	state := &UserGroupResourceModel{Members: types.SetNull(types.StringType)}
	diags := assignUserGroupResourceModel(context.Background(), state, ug, false, importHydration(false, state.Name))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.Members.IsNull() {
		t.Fatal("members must be hydrated from the wire on import")
	}
	if got := len(state.Members.Elements()); got != 2 {
		t.Errorf("members expected 2 elements, got %d", got)
	}
}

// TestAssignUserGroupResourceModel_ImportLeavesEmptyMembersNull asserts the
// non-empty rule: adopting an empty member list would store [] where a create
// that never declared the attribute stores null, which breaks
// ImportStateVerify on a memberless static group.
func TestAssignUserGroupResourceModel_ImportLeavesEmptyMembersNull(t *testing.T) {
	id := 5
	ug := &proclassic.UserGroup{
		ID:      &id,
		Name:    new("Imported Empty Static"),
		IsSmart: new(false),
		Users:   &proclassic.UserGroupUsers{User: &[]proclassic.UserGroupUsersUserItem{}},
	}

	state := &UserGroupResourceModel{Members: types.SetNull(types.StringType)}
	diags := assignUserGroupResourceModel(context.Background(), state, ug, false, importHydration(false, state.Name))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !state.Members.IsNull() {
		t.Errorf("members must stay null when the server returns none, got %v", state.Members)
	}
}

// TestAssignUserGroupResourceModel_ImportKeepsSmartMembersNull asserts the
// smart-group rule survives hydration: server-resolved membership is
// informational and would drift forever if stored.
func TestAssignUserGroupResourceModel_ImportKeepsSmartMembersNull(t *testing.T) {
	id := 6
	ug := &proclassic.UserGroup{
		ID:      &id,
		Name:    new("Imported Smart"),
		IsSmart: new(true),
		Users: &proclassic.UserGroupUsers{User: &[]proclassic.UserGroupUsersUserItem{
			{ID: new(9), Username: new("david@example.com")},
		}},
	}

	state := &UserGroupResourceModel{Members: types.SetNull(types.StringType)}
	diags := assignUserGroupResourceModel(context.Background(), state, ug, false, importHydration(false, state.Name))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !state.Members.IsNull() {
		t.Errorf("smart-group members must stay null on import, got %v", state.Members)
	}
}

func TestImportHydration(t *testing.T) {
	if !importHydration(true, types.StringValue("managed")) {
		t.Error("identity-only import (no prior state) must hydrate")
	}
	if !importHydration(false, types.StringNull()) {
		t.Error("passthrough import (sparse id-only state) must hydrate")
	}
	if importHydration(false, types.StringValue("managed")) {
		t.Error("a genuine refresh must not hydrate")
	}
}
