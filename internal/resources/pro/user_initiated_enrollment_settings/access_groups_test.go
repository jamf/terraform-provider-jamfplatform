// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func builtInGroup() pro.EnrollmentAccessGroupPreview {
	return pro.EnrollmentAccessGroupPreview{
		ID: new("1"), GroupID: "-1", LdapServerID: "-1",
		Name: "All Directory Service Users", SiteID: new("-1"),
	}
}

// TestReconcile_NeverDeletesBuiltIn proves id=1 is never scheduled for delete,
// even when the planned set omits it entirely.
func TestReconcile_NeverDeletesBuiltIn(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		builtInGroup(),
		{ID: new("13"), GroupID: "31", LdapServerID: "7", Name: "Admins"},
	}
	// Plan declares only "Admins" (by id).
	planned := []accessGroupModel{
		{ID: types.StringValue("13"), DirectoryServiceGroupID: types.StringValue("31"), LdapServerID: types.StringValue("7"), Name: types.StringValue("Admins")},
	}

	ops := planAccessGroupReconcile(planned, current)
	for _, op := range ops {
		if op.Action == accessGroupDelete && op.ID == allDirectoryServiceUsersID {
			t.Fatal("built-in id=1 must never be deleted")
		}
	}
}

// TestReconcile_CreateUpdateDelete covers the three actions.
func TestReconcile_CreateUpdateDelete(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		builtInGroup(),
		{ID: new("13"), GroupID: "31", LdapServerID: "7", Name: "Admins"},
		{ID: new("14"), GroupID: "32", LdapServerID: "7", Name: "ToDelete"},
	}
	planned := []accessGroupModel{
		// Update id=13 (name change).
		{ID: types.StringValue("13"), DirectoryServiceGroupID: types.StringValue("31"), LdapServerID: types.StringValue("7"), Name: types.StringValue("Renamed")},
		// Create a brand-new group (no id).
		{DirectoryServiceGroupID: types.StringValue("99"), LdapServerID: types.StringValue("7"), Name: types.StringValue("NewGroup")},
		// id=14 omitted → delete.
	}

	ops := planAccessGroupReconcile(planned, current)

	var creates, updates, deletes int
	for _, op := range ops {
		switch op.Action {
		case accessGroupCreate:
			creates++
		case accessGroupUpdate:
			updates++
		case accessGroupDelete:
			if op.ID != "14" {
				t.Errorf("unexpected delete id %q", op.ID)
			}
			deletes++
		}
	}
	if creates != 1 || updates != 1 || deletes != 1 {
		t.Errorf("expected 1/1/1 create/update/delete, got %d/%d/%d", creates, updates, deletes)
	}
}

// TestReconcile_NaturalKeyMatch proves a planned group without an id matches an
// existing group by name + ldap_server_id (update, not duplicate create).
// directory_service_group_id is Computed (resolved at apply), so it is not part
// of the match key.
func TestReconcile_NaturalKeyMatch(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		{ID: new("13"), GroupID: "37158", LdapServerID: "7", Name: "Admins", EnterpriseEnrollmentEnabled: new(false)},
	}
	planned := []accessGroupModel{
		// Same name + server, no id, a toggle changed → should UPDATE id=13.
		{LdapServerID: types.StringValue("7"), Name: types.StringValue("Admins"), EnterpriseEnrollmentEnabled: types.BoolValue(true)},
	}

	ops := planAccessGroupReconcile(planned, current)
	if len(ops) != 1 || ops[0].Action != accessGroupUpdate || ops[0].ID != "13" {
		t.Fatalf("expected single update of id=13 via name natural key, got %+v", ops)
	}
}

// TestReconcile_NoChangeNoOps proves an unchanged plan yields no operations.
func TestReconcile_NoChangeNoOps(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		{ID: new("13"), GroupID: "31", LdapServerID: "7", Name: "Admins", RequireEula: new(true)},
	}
	planned := []accessGroupModel{
		{
			ID:                      types.StringValue("13"),
			DirectoryServiceGroupID: types.StringValue("31"),
			LdapServerID:            types.StringValue("7"),
			Name:                    types.StringValue("Admins"),
			// require_eula unset → treated as no change despite server true.
		},
	}
	if ops := planAccessGroupReconcile(planned, current); len(ops) != 0 {
		t.Errorf("expected no ops for unchanged plan, got %+v", ops)
	}
}

// TestProjectManagedAccessGroups_MatchesDeclaredCardinality proves the
// readback projection returns ONLY the declared subset (matched to the fresh
// list), never appending the always-present built-in group — so the applied
// set cardinality equals the planned set.
func TestProjectManagedAccessGroups_MatchesDeclaredCardinality(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		builtInGroup(), // id=1, undeclared
		{ID: new("13"), GroupID: "31", LdapServerID: "7", Name: "Admins"},
	}
	// User declares only "Admins" by id.
	declared := []accessGroupModel{
		{ID: types.StringValue("13"), DirectoryServiceGroupID: types.StringValue("31"), LdapServerID: types.StringValue("7"), Name: types.StringValue("Admins")},
	}
	got := projectManagedAccessGroups(declared, current)
	if len(got) != 1 {
		t.Fatalf("expected 1 managed group, got %d", len(got))
	}
	if pointerString(got[0].ID) != "13" {
		t.Errorf("expected id=13, got %q", pointerString(got[0].ID))
	}
}

// TestProjectManagedAccessGroups_NaturalKeyForNew proves a freshly-created
// group (declared without id, matched by natural key) is projected with its
// new server id.
func TestProjectManagedAccessGroups_NaturalKeyForNew(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		{ID: new("20"), GroupID: "99", LdapServerID: "7", Name: "NewGroup"},
	}
	declared := []accessGroupModel{
		{DirectoryServiceGroupID: types.StringValue("99"), LdapServerID: types.StringValue("7"), Name: types.StringValue("NewGroup")},
	}
	got := projectManagedAccessGroups(declared, current)
	if len(got) != 1 || pointerString(got[0].ID) != "20" {
		t.Fatalf("expected the created group with id=20, got %+v", got)
	}
}

// TestReconcile_UpdateBuiltInToggles proves declaring id=1 with a changed toggle
// schedules an update (not a delete/create).
func TestReconcile_UpdateBuiltInToggles(t *testing.T) {
	current := []pro.EnrollmentAccessGroupPreview{
		{ID: new("1"), GroupID: "-1", LdapServerID: "-1", Name: "All Directory Service Users", SiteID: new("-1"), EnterpriseEnrollmentEnabled: new(false)},
	}
	planned := []accessGroupModel{
		{
			ID:                          types.StringValue("1"),
			DirectoryServiceGroupID:     types.StringValue("-1"),
			LdapServerID:                types.StringValue("-1"),
			Name:                        types.StringValue("All Directory Service Users"),
			SiteID:                      types.StringValue("-1"),
			EnterpriseEnrollmentEnabled: types.BoolValue(true), // toggle on
		},
	}
	ops := planAccessGroupReconcile(planned, current)
	if len(ops) != 1 || ops[0].Action != accessGroupUpdate || ops[0].ID != allDirectoryServiceUsersID {
		t.Fatalf("expected update of built-in id=1, got %+v", ops)
	}
}
