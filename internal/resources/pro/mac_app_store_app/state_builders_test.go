// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestFlattenMacAppGeneral_RoundTrip(t *testing.T) {
	state := &MacAppGeneralModel{}
	g := &proclassic.MacApplicationGeneral{
		ID:             new(42),
		Name:           new("iMovie"),
		Version:        new("10.4"),
		BundleID:       new("com.apple.iMovieApp"),
		DeploymentType: new(deploymentTypeSelfService),
		IsFree:         new(true),
		Category:       &proclassic.CategoryObject{ID: new(7), Name: new("Productivity")},
		Site:           &proclassic.SiteObject{ID: new(3), Name: new("HQ")},
	}
	flattenMacAppGeneral(g, state)

	if state.ID.ValueString() != "42" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "iMovie" {
		t.Errorf("name: got %q", state.Name.ValueString())
	}
	if state.DeploymentType.ValueString() != deploymentTypeSelfService {
		t.Errorf("deployment_type: got %q", state.DeploymentType.ValueString())
	}
	if state.CategoryID.ValueString() != "7" || state.CategoryName.ValueString() != "Productivity" {
		t.Errorf("category not flattened: %q / %q", state.CategoryID.ValueString(), state.CategoryName.ValueString())
	}
	if state.SiteID.ValueString() != "3" || state.SiteName.ValueString() != "HQ" {
		t.Errorf("site not flattened")
	}
}

// TestAssignMacApp_GuardedBlocks is the core echo-guard test: a minimal app
// created without scope / self_service / vpp blocks must keep those blocks null
// in state even though the server echoes them on GET. Populating an unmanaged
// block would trip the framework's "produced inconsistent result after apply".
func TestAssignMacApp_GuardedBlocks(t *testing.T) {
	state := &MacAppResourceModel{
		General: &MacAppGeneralModel{Name: types.StringValue("iMovie")},
		// Scope / SelfService / Vpp intentionally nil (unmanaged).
	}
	server := &proclassic.MacApplication{
		ID:      new(42),
		General: &proclassic.MacApplicationGeneral{ID: new(42), Name: new("iMovie")},
		Scope:   &proclassic.MacApplicationScope{AllComputers: new(false), AllJssUsers: new(false)},
		SelfService: &proclassic.MacApplicationSelfService{
			InstallButtonText: new("Install"),
		},
		Vpp: &proclassic.MacApplicationVpp{VppAdminAccountID: new(-1)},
	}

	diags := assignMacAppResourceModel(context.Background(), state, server, false)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if state.Scope != nil {
		t.Errorf("unmanaged scope was populated: %+v", state.Scope)
	}
	if state.SelfService != nil {
		t.Errorf("unmanaged self_service was populated: %+v", state.SelfService)
	}
	if state.Vpp != nil {
		t.Errorf("unmanaged vpp was populated: %+v", state.Vpp)
	}
	if state.ID.ValueString() != "42" {
		t.Errorf("id not set: %q", state.ID.ValueString())
	}
}

// TestAssignMacAppResourceModel_IncludeUnmanagedHydratesFromScratch pins the
// config-generation contract: with includeUnmanaged set and an empty starting
// model, every wire-present section is allocated and hydrated from the server.
func TestAssignMacAppResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	state := &MacAppResourceModel{}
	server := &proclassic.MacApplication{
		ID:      new(42),
		General: &proclassic.MacApplicationGeneral{ID: new(42), Name: new("iMovie")},
		Scope: &proclassic.MacApplicationScope{
			AllComputers: new(true),
			ComputerGroups: &proclassic.MacApplicationScopeComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(5)}, {ID: new(6)}},
			},
			Exclusions: &proclassic.MacApplicationScopeExclusions{
				Users: &proclassic.MacApplicationScopeExclusionsUsers{
					User: &[]proclassic.MacApplicationScopeExclusionsUsersUserItem{{Name: new("alice")}},
				},
			},
		},
		SelfService: &proclassic.MacApplicationSelfService{InstallButtonText: new("Install")},
		Vpp:         &proclassic.MacApplicationVpp{AssignVppDeviceBasedLicenses: new(true)},
	}
	diags := assignMacAppResourceModel(context.Background(), state, server, true)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected scope.targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers hydrated true")
	}
	if got := len(state.Scope.Targets.ComputerGroupIDs.Elements()); got != 2 {
		t.Fatalf("expected 2 computer_group_ids, got %d", got)
	}
	if state.Scope.Exclusions == nil {
		t.Fatal("expected exclusions allocated when wire-present")
	}
	if state.Scope.Limitations != nil {
		t.Fatalf("expected limitations nil when wire-absent; got %+v", state.Scope.Limitations)
	}
	if state.SelfService == nil || state.SelfService.InstallButtonText.ValueString() != "Install" {
		t.Fatalf("expected self_service hydrated; got %+v", state.SelfService)
	}
	if state.Vpp == nil || !state.Vpp.AssignVppDeviceBasedLicenses.ValueBool() {
		t.Fatalf("expected vpp hydrated; got %+v", state.Vpp)
	}
}

func TestFlattenMacAppScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	ctx := context.Background()
	// Managed categories carry a non-null current value (declared in config);
	// everything else is null = unmanaged and must stay null.
	state := &scope.ComputerScopeModelNoIbeacons{
		Targets: &scope.ComputerScopeTargetsModel{
			ComputerIDs: scope.EmptyStringSet(), // managed, refreshes from wire
		},
		Limitations: &scope.ComputerScopeLimitationsModelNoIbeacons{
			NetworkSegmentIDs: idSet("9"), // managed, drift-refreshes
		},
		Exclusions: &scope.ComputerScopeExclusionsModelNoIbeacons{
			DirectoryServiceOrLocalUserNames: scope.EmptyStringSet(), // managed
		},
	}
	s := &proclassic.MacApplicationScope{
		AllComputers: new(false),
		Computers: &proclassic.MacApplicationScopeComputers{
			Computer: &[]proclassic.MacApplicationScopeComputersComputerItem{{ID: new(11)}, {ID: new(12)}},
		},
		ComputerGroups: &proclassic.MacApplicationScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Limitations: &proclassic.MacApplicationScopeLimitations{
			NetworkSegments: &proclassic.MacApplicationScopeLimitationsNetworkSegments{
				NetworkSegment: &[]proclassic.IDName{{ID: new(2)}},
			},
		},
		Exclusions: &proclassic.MacApplicationScopeExclusions{
			Users: &proclassic.MacApplicationScopeExclusionsUsers{
				User: &[]proclassic.MacApplicationScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}
	flattenMacAppScope(ctx, s, state, false)

	var computerIDs []string
	state.Targets.ComputerIDs.ElementsAs(ctx, &computerIDs, false)
	if len(computerIDs) != 2 {
		t.Errorf("managed computer_ids should refresh from wire: got %v", computerIDs)
	}
	if !state.Targets.ComputerGroupIDs.IsNull() {
		t.Errorf("unmanaged computer_group_ids must stay null, got %v", state.Targets.ComputerGroupIDs)
	}
	if !state.Targets.AllComputers.IsNull() {
		t.Errorf("unmanaged all_computers must stay null, got %v", state.Targets.AllComputers)
	}
	var segIDs []string
	state.Limitations.NetworkSegmentIDs.ElementsAs(ctx, &segIDs, false)
	if len(segIDs) != 1 || segIDs[0] != "2" {
		t.Errorf("managed limitations network_segment_ids should drift-refresh: got %v", segIDs)
	}
	var exclUsers []string
	state.Exclusions.DirectoryServiceOrLocalUserNames.ElementsAs(ctx, &exclUsers, false)
	if len(exclUsers) != 1 || exclUsers[0] != "alice" {
		t.Errorf("managed exclusion user names should refresh: got %v", exclUsers)
	}
	if !state.Exclusions.ComputerGroupIDs.IsNull() {
		t.Errorf("unmanaged exclusion computer_group_ids must stay null, got %v", state.Exclusions.ComputerGroupIDs)
	}
}

func TestFlattenMacAppScope_HydrateAllForMergeBase(t *testing.T) {
	ctx := context.Background()
	// includeUnmanaged=true hydrates every wire-present category into a zero
	// model — the shape Update uses to build the read-merge-write base.
	state := &scope.ComputerScopeModelNoIbeacons{}
	s := &proclassic.MacApplicationScope{
		AllComputers: new(false),
		ComputerGroups: &proclassic.MacApplicationScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Exclusions: &proclassic.MacApplicationScopeExclusions{
			Users: &proclassic.MacApplicationScopeExclusionsUsers{
				User: &[]proclassic.MacApplicationScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}
	flattenMacAppScope(ctx, s, state, true)

	if state.Targets == nil || state.Targets.AllComputers.IsNull() || state.Targets.AllComputers.ValueBool() {
		t.Fatalf("expected all_computers hydrated false, got %+v", state.Targets)
	}
	var groupIDs []string
	state.Targets.ComputerGroupIDs.ElementsAs(ctx, &groupIDs, false)
	if len(groupIDs) != 1 || groupIDs[0] != "5" {
		t.Errorf("expected computer_group_ids hydrated, got %v", groupIDs)
	}
	if state.Exclusions == nil {
		t.Fatal("expected exclusions allocated")
	}
	var exclUsers []string
	state.Exclusions.DirectoryServiceOrLocalUserNames.ElementsAs(ctx, &exclUsers, false)
	if len(exclUsers) != 1 || exclUsers[0] != "alice" {
		t.Errorf("expected exclusion user names hydrated, got %v", exclUsers)
	}
}

func TestFlattenMacAppVpp_Counts(t *testing.T) {
	state := &MacAppVppModel{}
	v := &proclassic.MacApplicationVpp{
		AssignVppDeviceBasedLicenses: new(true),
		VppAdminAccountID:            new(4),
		TotalVppLicenses:             new(50),
		RemainingVppLicenses:         new(20),
		UsedVppLicenses:              new(30),
	}
	flattenMacAppVpp(v, state)
	if !state.AssignVppDeviceBasedLicenses.ValueBool() {
		t.Errorf("assign not flattened")
	}
	if state.VppAdminAccountID.ValueString() != "4" {
		t.Errorf("vpp_admin_account_id: got %q", state.VppAdminAccountID.ValueString())
	}
	if state.TotalVppLicenses.ValueInt64() != 50 || state.RemainingVppLicenses.ValueInt64() != 20 || state.UsedVppLicenses.ValueInt64() != 30 {
		t.Errorf("license counts not flattened: %d/%d/%d", state.TotalVppLicenses.ValueInt64(), state.RemainingVppLicenses.ValueInt64(), state.UsedVppLicenses.ValueInt64())
	}
}

func TestFlattenMacAppSelfService_Notification(t *testing.T) {
	state := &MacAppSelfServiceModel{}
	ss := &proclassic.MacApplicationSelfService{
		Notification: &proclassic.NotificationValue{Enabled: new(true), Method: new("Self Service")},
	}
	flattenMacAppSelfService(ss, state)
	if !state.NotificationEnabled.ValueBool() {
		t.Errorf("notification_enabled not flattened")
	}
	if state.NotificationMethod.ValueString() != "Self Service" {
		t.Errorf("notification_method: got %q", state.NotificationMethod.ValueString())
	}
}
