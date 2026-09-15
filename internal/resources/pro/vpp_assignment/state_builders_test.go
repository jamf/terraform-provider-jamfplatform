// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Pointer locals (v := …; &v) are used throughout instead of intp/strp helper
// funcs: a trivial `func intp(i int) *int { return &i }` attracts the `go fix`
// inliner (it rewrites the body to the invalid `return new(i)` and is not
// idempotent), so the helpers are deliberately omitted.

func sampleAPI() *proclassic.VppAssignment {
	id, acct := 2, 3
	name := "Test"
	acctName := "Apple Business Manager"
	allUsers := false
	groupID := 1
	groupName := "Group One"
	limName := "LDAP Admins"
	exclName := "Excluded LDAP"
	iosAdam, iosName := 6444539476, "Jamf Self Service"
	macAdam, macName := 409203825, "Numbers"

	return &proclassic.VppAssignment{
		ID: &id,
		General: &proclassic.VppAssignmentGeneral{
			ID:                  &id,
			Name:                &name,
			VppAdminAccountID:   &acct,
			VppAdminAccountName: &acctName,
		},
		IosApps: &proclassic.VppAssignmentIosApps{
			IosApp: &[]proclassic.VppAssignmentIosAppsIosAppItem{
				{AdamID: &iosAdam, Name: &iosName},
			},
		},
		MacApps: &proclassic.VppAssignmentMacApps{
			MacApp: &[]proclassic.VppAssignmentMacAppsMacAppItem{
				{AdamID: &macAdam, Name: &macName},
			},
		},
		Scope: &proclassic.VppAssignmentScope{
			AllJssUsers:   &allUsers,
			JssUserGroups: &proclassic.VppAssignmentScopeJssUserGroups{UserGroup: &[]proclassic.IDName{{ID: &groupID, Name: &groupName}}},
			Limitations: &proclassic.VppAssignmentScopeLimitations{
				UserGroups: &proclassic.VppAssignmentScopeLimitationsUserGroups{UserGroup: &[]proclassic.IDName{{Name: &limName}}},
			},
			Exclusions: &proclassic.VppAssignmentScopeExclusions{
				UserGroups: &proclassic.VppAssignmentScopeExclusionsUserGroups{UserGroup: &[]proclassic.VppAssignmentScopeExclusionsUserGroupsUserGroupItem{{Name: &exclName}}},
			},
		},
	}
}

func TestAssignResourceModel_General(t *testing.T) {
	state := &VPPAssignmentResourceModel{
		IosAppAdamIDs: types.SetNull(types.Int64Type),
		MacAppAdamIDs: types.SetNull(types.Int64Type),
		EbookAdamIDs:  types.SetNull(types.Int64Type),
	}
	assignVPPAssignmentResourceModel(context.Background(), state, sampleAPI(), false)

	if state.ID.ValueString() != "2" {
		t.Errorf("id = %q", state.ID.ValueString())
	}
	if state.VPPAdminAccountID.ValueString() != "3" {
		t.Errorf("vpp_admin_account_id = %q", state.VPPAdminAccountID.ValueString())
	}
	if state.VPPAdminAccountName.ValueString() != "Apple Business Manager" {
		t.Errorf("vpp_admin_account_name = %q", state.VPPAdminAccountName.ValueString())
	}
}

// TestAssignResourceModel_ContentOptOut: an unmanaged (null) content set stays
// null even though the server returns the collection; a managed set is refreshed.
func TestAssignResourceModel_ContentOptOut(t *testing.T) {
	// All content unmanaged → stays null.
	unmanaged := &VPPAssignmentResourceModel{
		IosAppAdamIDs: types.SetNull(types.Int64Type),
		MacAppAdamIDs: types.SetNull(types.Int64Type),
		EbookAdamIDs:  types.SetNull(types.Int64Type),
	}
	assignVPPAssignmentResourceModel(context.Background(), unmanaged, sampleAPI(), false)
	if !unmanaged.IosAppAdamIDs.IsNull() {
		t.Error("unmanaged ios_app_adam_ids must stay null (don't fabricate management)")
	}

	// ios managed, mac/ebook unmanaged → only ios refreshed.
	managed := &VPPAssignmentResourceModel{
		IosAppAdamIDs: mustInt64Set(t, 1), // any non-null marks it managed
		MacAppAdamIDs: types.SetNull(types.Int64Type),
		EbookAdamIDs:  types.SetNull(types.Int64Type),
	}
	assignVPPAssignmentResourceModel(context.Background(), managed, sampleAPI(), false)
	if managed.IosAppAdamIDs.IsNull() || len(managed.IosAppAdamIDs.Elements()) != 1 {
		t.Errorf("managed ios_app_adam_ids should be refreshed: %v", managed.IosAppAdamIDs)
	}
	if !managed.MacAppAdamIDs.IsNull() {
		t.Error("unmanaged mac_app_adam_ids must stay null")
	}
}

// TestFlattenAdamSet_EmptyKnown: a managed set that the server returns empty must
// reconcile to a known empty Set (NOT null) — these are plain Optional attrs, so a
// null after an empty config triggers a "provider produced inconsistent result".
func TestFlattenAdamSet_EmptyKnown(t *testing.T) {
	out := flattenAdamSet(context.Background(), nil)
	if out.IsNull() || out.IsUnknown() {
		t.Error("empty content must yield a known empty Set, not null/unknown")
	}
	if len(out.Elements()) != 0 {
		t.Errorf("expected 0 elements, got %d", len(out.Elements()))
	}
}

func TestFlattenAdamSet_RoundTrip(t *testing.T) {
	a, b := 6444539476, 409203825
	out := flattenAdamSet(context.Background(), []*int{&a, nil, &b})
	if out.IsNull() || len(out.Elements()) != 2 {
		t.Errorf("adam set round-trip = %v (nil entries skipped)", out)
	}
}

func TestAssignResourceModel_ScopeOnlyWhenManaged(t *testing.T) {
	// nil Scope in state → scope not populated (server always echoes <scope>).
	state := &VPPAssignmentResourceModel{
		IosAppAdamIDs: types.SetNull(types.Int64Type),
		MacAppAdamIDs: types.SetNull(types.Int64Type),
		EbookAdamIDs:  types.SetNull(types.Int64Type),
	}
	assignVPPAssignmentResourceModel(context.Background(), state, sampleAPI(), false)
	if state.Scope != nil {
		t.Error("unmanaged scope block must stay nil")
	}

	// Managed categories carry a non-null current value (declared in config);
	// undeclared (null) categories inside a managed block must stay null.
	managed := &VPPAssignmentResourceModel{
		IosAppAdamIDs: types.SetNull(types.Int64Type),
		MacAppAdamIDs: types.SetNull(types.Int64Type),
		EbookAdamIDs:  types.SetNull(types.Int64Type),
		Scope: &scope.UserScopeModel{
			Targets:     &scope.UserScopeTargetsModel{JssUserGroupIDs: scope.EmptyStringSet()},
			Limitations: &scope.UserScopeLimitationsModel{DirectoryServiceUserGroupNames: scope.EmptyStringSet()},
			Exclusions:  &scope.UserScopeExclusionsModel{DirectoryServiceUserGroupNames: scope.EmptyStringSet()},
		},
	}
	assignVPPAssignmentResourceModel(context.Background(), managed, sampleAPI(), false)
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

func TestAssignDataSourceModel_PopulatesContentAndScope(t *testing.T) {
	state := &VPPAssignmentDataSourceModel{}
	assignVPPAssignmentDataSourceModel(context.Background(), state, sampleAPI())
	if state.Scope == nil {
		t.Fatal("DS must always populate scope")
	}
	if state.Scope.Targets.JssUserGroupIDs.IsNull() {
		t.Error("DS scope jss_user_group_ids should be populated")
	}
	// DS content lists always known (read-only).
	if state.IosApps.IsNull() || len(state.IosApps.Elements()) != 1 {
		t.Errorf("DS ios_apps = %v", state.IosApps)
	}
	if state.Ebooks.IsNull() || len(state.Ebooks.Elements()) != 0 {
		t.Errorf("DS ebooks must be a known empty list when absent, got %v", state.Ebooks)
	}
}

// TestAssignVPPAssignmentResourceModel_IncludeUnmanagedHydratesFromScratch pins
// the config-generation contract: with includeUnmanaged set and an empty
// starting model, the content sets and the wire-present scope are hydrated from
// the server.
func TestAssignVPPAssignmentResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	state := &VPPAssignmentResourceModel{}
	assignVPPAssignmentResourceModel(context.Background(), state, sampleAPI(), true)

	if state.IosAppAdamIDs.IsNull() || len(state.IosAppAdamIDs.Elements()) != 1 {
		t.Errorf("expected ios_app_adam_ids hydrated; got %v", state.IosAppAdamIDs)
	}
	if state.MacAppAdamIDs.IsNull() || len(state.MacAppAdamIDs.Elements()) != 1 {
		t.Errorf("expected mac_app_adam_ids hydrated; got %v", state.MacAppAdamIDs)
	}
	if state.EbookAdamIDs.IsNull() {
		t.Error("expected ebook_adam_ids hydrated to a known (empty) set")
	}
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
}

func TestIntStringOrNull_NegativeIsNull(t *testing.T) {
	neg, pos := -1, 3
	if !intStringOrNull(&neg).IsNull() {
		t.Error("vpp_admin_account_id = -1 (no valid account) must map to null")
	}
	if intStringOrNull(&pos).ValueString() != "3" {
		t.Error("valid account id must map to its decimal form")
	}
}
