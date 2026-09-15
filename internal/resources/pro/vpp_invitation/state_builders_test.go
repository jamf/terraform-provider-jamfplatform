// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func sampleAPI() *proclassic.VppInvitation {
	id, acct := 2, 3
	name := "Test"
	dm := "Make available in Self Service only"
	autoReg, allUsers := true, false
	groupID := 1
	groupName := "Group One"
	limName := "LDAP Admins"
	exclName := "Excluded LDAP"
	usageID := 3
	usageName := "user@example.com"
	usageStatus := "registered"
	epoch := 1778575794328

	return &proclassic.VppInvitation{
		ID: &id,
		General: &proclassic.VppInvitationGeneral{
			ID:                       &id,
			Name:                     &name,
			DistributionMethod:       &dm,
			AutoRegisterManagedUsers: &autoReg,
			VppAccount:               &proclassic.VppInvitationGeneralVppAccount{ID: &acct},
		},
		Scope: &proclassic.VppInvitationScope{
			AllJssUsers:   &allUsers,
			JssUserGroups: &proclassic.VppInvitationScopeJssUserGroups{UserGroup: &[]proclassic.IDName{{ID: &groupID, Name: &groupName}}},
			Limitations: &proclassic.VppInvitationScopeLimitations{
				UserGroups: &proclassic.VppInvitationScopeLimitationsUserGroups{UserGroup: &[]proclassic.IDName{{Name: &limName}}},
			},
			Exclusions: &proclassic.VppInvitationScopeExclusions{
				UserGroups: &proclassic.VppInvitationScopeExclusionsUserGroups{UserGroup: &[]proclassic.VppInvitationScopeExclusionsUserGroupsUserGroupItem{{Name: &exclName}}},
			},
		},
		InvitationUsages: &proclassic.VppInvitationInvitationUsages{
			Usage: &[]proclassic.VppInvitationInvitationUsagesUsageItem{
				{ID: &usageID, Name: &usageName, Status: &usageStatus, LastActionDateEpoch: &epoch},
			},
		},
	}
}

func TestAssignResourceModel_GeneralAndUsages(t *testing.T) {
	state := &VPPInvitationResourceModel{}
	assignVPPInvitationResourceModel(context.Background(), state, sampleAPI(), false)

	if state.ID.ValueString() != "2" {
		t.Errorf("id = %q", state.ID.ValueString())
	}
	if state.VPPAccountID.ValueString() != "3" {
		t.Errorf("vpp_account_id = %q", state.VPPAccountID.ValueString())
	}
	if !state.AutoRegisterManagedUsers.ValueBool() {
		t.Error("auto_register_managed_users not set")
	}
	// invitation_usages is always refreshed (Computed), even with nil scope state.
	if state.InvitationUsages.IsNull() || len(state.InvitationUsages.Elements()) != 1 {
		t.Errorf("invitation_usages = %v", state.InvitationUsages)
	}
}

func TestAssignResourceModel_ScopeOnlyWhenManaged(t *testing.T) {
	// nil Scope in state → scope not populated (server always echoes <scope>).
	state := &VPPInvitationResourceModel{}
	assignVPPInvitationResourceModel(context.Background(), state, sampleAPI(), false)
	if state.Scope != nil {
		t.Error("unmanaged scope block must stay nil")
	}

	// Managed scope (state.Scope non-nil) → managed (non-null) categories are
	// refreshed, name-keyed groups by name; undeclared (null) categories inside
	// a managed block must stay null.
	managed := &VPPInvitationResourceModel{Scope: &scope.UserScopeModel{
		Targets:     &scope.UserScopeTargetsModel{JssUserGroupIDs: scope.EmptyStringSet()},
		Limitations: &scope.UserScopeLimitationsModel{DirectoryServiceUserGroupNames: scope.EmptyStringSet()},
		Exclusions:  &scope.UserScopeExclusionsModel{DirectoryServiceUserGroupNames: scope.EmptyStringSet()},
	}}
	assignVPPInvitationResourceModel(context.Background(), managed, sampleAPI(), false)
	if managed.Scope.Targets.JssUserGroupIDs.IsNull() || len(managed.Scope.Targets.JssUserGroupIDs.Elements()) != 1 {
		t.Errorf("jss_user_group_ids = %v", managed.Scope.Targets.JssUserGroupIDs)
	}
	if managed.Scope.Limitations.DirectoryServiceUserGroupNames.IsNull() {
		t.Error("limitations DS names should be populated by name")
	}
	if managed.Scope.Exclusions.DirectoryServiceUserGroupNames.IsNull() {
		t.Error("exclusions DS names should be populated by name")
	}
	if !managed.Scope.Targets.JssUserIDs.IsNull() {
		t.Errorf("undeclared jss_user_ids must stay null, got %v", managed.Scope.Targets.JssUserIDs)
	}
	if !managed.Scope.Targets.AllJssUsers.IsNull() {
		t.Errorf("undeclared all_jss_users must stay null, got %v", managed.Scope.Targets.AllJssUsers)
	}
}

// TestFlattenScope_ManagedRefreshUnmanagedStaysNull pins the per-category
// read gate: a managed (non-null) category drift-refreshes from the wire; an
// unmanaged (null) sibling in the same sub-block stays null so members
// maintained in the admin UI never enter state.
func TestFlattenScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	ctx := context.Background()
	state := &scope.UserScopeModel{
		Targets: &scope.UserScopeTargetsModel{
			JssUserGroupIDs: mustStringSet(t, "999"), // managed, drift-refreshes
		},
		Exclusions: &scope.UserScopeExclusionsModel{
			DirectoryServiceUserGroupNames: scope.EmptyStringSet(), // managed
		},
	}
	flattenScope(ctx, sampleAPI().Scope, state, false)

	var groupIDs []string
	state.Targets.JssUserGroupIDs.ElementsAs(ctx, &groupIDs, false)
	if len(groupIDs) != 1 || groupIDs[0] != "1" {
		t.Errorf("managed jss_user_group_ids should drift-refresh from wire: got %v", groupIDs)
	}
	if !state.Targets.JssUserIDs.IsNull() {
		t.Errorf("unmanaged jss_user_ids must stay null, got %v", state.Targets.JssUserIDs)
	}
	if !state.Targets.AllJssUsers.IsNull() {
		t.Errorf("unmanaged all_jss_users must stay null, got %v", state.Targets.AllJssUsers)
	}
	var exclNames []string
	state.Exclusions.DirectoryServiceUserGroupNames.ElementsAs(ctx, &exclNames, false)
	if len(exclNames) != 1 || exclNames[0] != "Excluded LDAP" {
		t.Errorf("managed exclusion DS names should refresh: got %v", exclNames)
	}
	if !state.Exclusions.JssUserIDs.IsNull() {
		t.Errorf("unmanaged exclusion jss_user_ids must stay null, got %v", state.Exclusions.JssUserIDs)
	}
	if state.Limitations != nil {
		t.Errorf("undeclared limitations block must stay nil, got %+v", state.Limitations)
	}
}

// TestFlattenScope_HydrateAllForMergeBase pins the includeUnmanaged path: a
// zero model hydrates every wire-present category — the shape Update uses to
// build the read-merge-write base (and import / config generation use for
// full visibility).
func TestFlattenScope_HydrateAllForMergeBase(t *testing.T) {
	ctx := context.Background()
	state := &scope.UserScopeModel{}
	flattenScope(ctx, sampleAPI().Scope, state, true)

	if state.Targets == nil || state.Targets.AllJssUsers.IsNull() || state.Targets.AllJssUsers.ValueBool() {
		t.Fatalf("expected all_jss_users hydrated false, got %+v", state.Targets)
	}
	var groupIDs []string
	state.Targets.JssUserGroupIDs.ElementsAs(ctx, &groupIDs, false)
	if len(groupIDs) != 1 || groupIDs[0] != "1" {
		t.Errorf("expected jss_user_group_ids hydrated, got %v", groupIDs)
	}
	if state.Targets.JssUserIDs.IsNull() || len(state.Targets.JssUserIDs.Elements()) != 0 {
		t.Errorf("expected wire-absent jss_user_ids hydrated to a known empty set, got %v", state.Targets.JssUserIDs)
	}
	if state.Limitations == nil || state.Limitations.DirectoryServiceUserGroupNames.IsNull() {
		t.Fatalf("expected limitations hydrated, got %+v", state.Limitations)
	}
	if state.Exclusions == nil {
		t.Fatal("expected exclusions allocated")
	}
	var exclNames []string
	state.Exclusions.DirectoryServiceUserGroupNames.ElementsAs(ctx, &exclNames, false)
	if len(exclNames) != 1 || exclNames[0] != "Excluded LDAP" {
		t.Errorf("expected exclusion DS names hydrated, got %v", exclNames)
	}
}

// TestAssignVPPInvitationResourceModel_IncludeUnmanagedHydratesFromScratch pins
// the config-generation contract: with includeUnmanaged set and an empty
// starting model, the wire-present scope is allocated and hydrated from the
// server.
func TestAssignVPPInvitationResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	state := &VPPInvitationResourceModel{}
	assignVPPInvitationResourceModel(context.Background(), state, sampleAPI(), true)

	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected scope.targets hydrated from scratch; got %+v", state.Scope)
	}
	if state.Scope.Targets.JssUserGroupIDs.IsNull() || len(state.Scope.Targets.JssUserGroupIDs.Elements()) != 1 {
		t.Errorf("expected jss_user_group_ids hydrated; got %v", state.Scope.Targets.JssUserGroupIDs)
	}
	if state.Scope.Limitations == nil || state.Scope.Limitations.DirectoryServiceUserGroupNames.IsNull() {
		t.Fatalf("expected limitations hydrated when wire-present; got %+v", state.Scope.Limitations)
	}
	if state.Scope.Exclusions == nil || state.Scope.Exclusions.DirectoryServiceUserGroupNames.IsNull() {
		t.Fatalf("expected exclusions hydrated when wire-present; got %+v", state.Scope.Exclusions)
	}
	if state.InvitationUsages.IsNull() || len(state.InvitationUsages.Elements()) != 1 {
		t.Errorf("expected invitation_usages hydrated; got %v", state.InvitationUsages)
	}
}

func TestFlattenUsages_EmptyKnown(t *testing.T) {
	out := flattenUsages(nil)
	if out.IsNull() || out.IsUnknown() {
		t.Error("nil usages must yield a known empty list, not null/unknown")
	}
	if len(out.Elements()) != 0 {
		t.Errorf("expected 0 elements, got %d", len(out.Elements()))
	}
}

func TestAssignDataSourceModel_PopulatesScopeAndUsages(t *testing.T) {
	state := &VPPInvitationDataSourceModel{}
	assignVPPInvitationDataSourceModel(context.Background(), state, sampleAPI())
	if state.Scope == nil {
		t.Fatal("DS must always populate scope")
	}
	if state.Scope.Targets.JssUserGroupIDs.IsNull() {
		t.Error("DS scope jss_user_group_ids should be populated")
	}
	if len(state.InvitationUsages.Elements()) != 1 {
		t.Errorf("DS usages = %v", state.InvitationUsages)
	}
}
