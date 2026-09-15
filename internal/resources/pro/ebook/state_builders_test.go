// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func setLen(s types.Set) int { return len(s.Elements()) }

func TestFlattenEbookGeneral_ServerDerivedAndCategory(t *testing.T) {
	g := &proclassic.EbookGeneral{
		ID:             new(79),
		Name:           new("Field Guide"),
		URL:            new("https://example.org/guide.pdf"),
		FileType:       new("PDF"),
		Version:        new("1.0"),
		DeploymentType: new(deploymentTypeSelfService),
		Free:           new(false),
		Category:       &proclassic.CategoryObject{ID: new(-1), Name: new("No category assigned")},
		Site:           &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
	}
	// Empty prior state → adopt the API values.
	state := &EbookGeneralModel{}
	flattenEbookGeneral(g, state)

	if state.ID.ValueString() != "79" {
		t.Errorf("id not flattened: %q", state.ID.ValueString())
	}
	if state.FileType.ValueString() != "PDF" {
		t.Errorf("file_type not flattened: %q", state.FileType.ValueString())
	}
	if state.CategoryID.ValueString() != "-1" {
		t.Errorf("category_id not flattened: %q", state.CategoryID.ValueString())
	}
	// Sentinel category/site (id -1): derived names null, not the flaky echo.
	if !state.CategoryName.IsNull() {
		t.Errorf("category_name should be null on the sentinel, got %q", state.CategoryName.ValueString())
	}
	if !state.SiteName.IsNull() {
		t.Errorf("site_name should be null on the sentinel, got %q", state.SiteName.ValueString())
	}
}

// TestFlattenEbookScope_DualTargetRoundTrip hydrates the full dual-target
// union from a zero model (includeUnmanaged=true — the shape Update uses to
// build the read-merge-write base and Read uses on first-time import).
func TestFlattenEbookScope_DualTargetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := &proclassic.EbookScope{
		AllComputers:     new(false),
		AllMobileDevices: new(false),
		AllJssUsers:      new(false),
		Computers: &proclassic.EbookScopeComputers{
			Computer: &[]proclassic.EbookScopeComputersComputerItem{{ID: new(11)}, {ID: new(12)}},
		},
		MobileDevices: &proclassic.EbookScopeMobileDevices{
			MobileDevice: &[]proclassic.EbookScopeMobileDevicesMobileDeviceItem{{ID: new(21)}},
		},
		MobileDeviceGroups: &proclassic.EbookScopeMobileDeviceGroups{
			MobileDeviceGroup: &[]proclassic.IDName{{ID: new(6)}},
		},
		Classes: &proclassic.EbookScopeClasses{
			Class: &[]proclassic.IDName{{ID: new(41)}, {ID: new(42)}},
		},
		Limitations: &proclassic.EbookScopeLimitations{
			Users: &proclassic.EbookScopeLimitationsUsers{User: &[]proclassic.IDName{{Name: new("alice")}}},
		},
		Exclusions: &proclassic.EbookScopeExclusions{
			MobileDevices: &proclassic.EbookScopeExclusionsMobileDevices{
				MobileDevice: &[]proclassic.EbookScopeExclusionsMobileDevicesMobileDeviceItem{{ID: new(99)}},
			},
			UserGroups: &proclassic.EbookScopeExclusionsUserGroups{UserGroup: &[]proclassic.IDName{{Name: new("Admins")}}},
		},
	}

	state := &EbookScopeModel{}
	flattenEbookScope(ctx, s, state, true)

	if setLen(state.Targets.ComputerIDs) != 2 {
		t.Errorf("computer_ids: want 2, got %d", setLen(state.Targets.ComputerIDs))
	}
	if setLen(state.Targets.MobileDeviceIDs) != 1 {
		t.Errorf("mobile_device_ids: want 1, got %d", setLen(state.Targets.MobileDeviceIDs))
	}
	if setLen(state.Targets.MobileDeviceGroupIDs) != 1 {
		t.Errorf("mobile_device_group_ids: want 1, got %d", setLen(state.Targets.MobileDeviceGroupIDs))
	}
	if setLen(state.Targets.ClassIDs) != 2 {
		t.Errorf("class_ids: want 2, got %d", setLen(state.Targets.ClassIDs))
	}
	if setLen(state.Limitations.DirectoryServiceOrLocalUserNames) != 1 {
		t.Errorf("limitations user names: want 1, got %d", setLen(state.Limitations.DirectoryServiceOrLocalUserNames))
	}
	if setLen(state.Exclusions.MobileDeviceIDs) != 1 {
		t.Errorf("exclusions mobile_device_ids: want 1, got %d", setLen(state.Exclusions.MobileDeviceIDs))
	}
	if setLen(state.Exclusions.DirectoryServiceUserGroupNames) != 1 {
		t.Errorf("exclusions user_group names: want 1, got %d", setLen(state.Exclusions.DirectoryServiceUserGroupNames))
	}
	if state.Targets.AllComputers.IsNull() || state.Targets.AllComputers.ValueBool() {
		t.Errorf("all_computers should be false, got %v", state.Targets.AllComputers)
	}
	// Wire-absent categories hydrate to empty (never null) under hydrate-all.
	if state.Targets.BuildingIDs.IsNull() || setLen(state.Targets.BuildingIDs) != 0 {
		t.Errorf("wire-absent building_ids must hydrate to an empty set, got %v", state.Targets.BuildingIDs)
	}
}

// TestFlattenEbookScope_ManagedRefreshUnmanagedStaysNull pins the granular
// ownership gate: a managed (non-null) category refreshes from the live scope;
// an unmanaged (null) category stays null so members maintained in the admin UI
// never enter state.
func TestFlattenEbookScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	ctx := context.Background()
	state := &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			ComputerIDs:     idSet(),      // managed [], refreshes from the live scope
			MobileDeviceIDs: idSet("999"), // managed, drift-refreshes
		},
		Limitations: &EbookScopeLimitationsModel{
			DirectoryServiceOrLocalUserNames: idSetNames(), // managed
		},
		Exclusions: &EbookScopeExclusionsModel{
			DirectoryServiceUserGroupNames: idSetNames(), // managed
		},
	}
	s := &proclassic.EbookScope{
		AllComputers: new(false),
		Computers: &proclassic.EbookScopeComputers{
			Computer: &[]proclassic.EbookScopeComputersComputerItem{{ID: new(11)}, {ID: new(12)}},
		},
		MobileDevices: &proclassic.EbookScopeMobileDevices{
			MobileDevice: &[]proclassic.EbookScopeMobileDevicesMobileDeviceItem{{ID: new(21)}},
		},
		ComputerGroups: &proclassic.EbookScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Limitations: &proclassic.EbookScopeLimitations{
			Users: &proclassic.EbookScopeLimitationsUsers{User: &[]proclassic.IDName{{Name: new("alice")}}},
			NetworkSegments: &proclassic.EbookScopeLimitationsNetworkSegments{
				NetworkSegment: &[]proclassic.IDName{{ID: new(2)}},
			},
		},
		Exclusions: &proclassic.EbookScopeExclusions{
			UserGroups: &proclassic.EbookScopeExclusionsUserGroups{UserGroup: &[]proclassic.IDName{{Name: new("Admins")}}},
			ComputerGroups: &proclassic.EbookScopeExclusionsComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(7)}},
			},
		},
	}
	flattenEbookScope(ctx, s, state, false)

	if setLen(state.Targets.ComputerIDs) != 2 {
		t.Errorf("managed computer_ids should refresh from the live scope: got %v", state.Targets.ComputerIDs)
	}
	if setLen(state.Targets.MobileDeviceIDs) != 1 {
		t.Errorf("managed mobile_device_ids should drift-refresh: got %v", state.Targets.MobileDeviceIDs)
	}
	if !state.Targets.ComputerGroupIDs.IsNull() {
		t.Errorf("unmanaged computer_group_ids must stay null, got %v", state.Targets.ComputerGroupIDs)
	}
	if !state.Targets.AllComputers.IsNull() {
		t.Errorf("unmanaged all_computers must stay null, got %v", state.Targets.AllComputers)
	}
	if setLen(state.Limitations.DirectoryServiceOrLocalUserNames) != 1 {
		t.Errorf("managed limitation user names should refresh: got %v", state.Limitations.DirectoryServiceOrLocalUserNames)
	}
	if !state.Limitations.NetworkSegmentIDs.IsNull() {
		t.Errorf("unmanaged limitation network_segment_ids must stay null, got %v", state.Limitations.NetworkSegmentIDs)
	}
	if setLen(state.Exclusions.DirectoryServiceUserGroupNames) != 1 {
		t.Errorf("managed exclusion user_group names should refresh: got %v", state.Exclusions.DirectoryServiceUserGroupNames)
	}
	if !state.Exclusions.ComputerGroupIDs.IsNull() {
		t.Errorf("unmanaged exclusion computer_group_ids must stay null, got %v", state.Exclusions.ComputerGroupIDs)
	}
}

func TestFlattenEbookSelfService_CategoriesPreserveAuthoredValuesByID(t *testing.T) {
	ss := &proclassic.EbookSelfService{
		InstallButtonText: new("Install"),
		Notification:      &proclassic.NotificationValue{Enabled: new(true), Method: new("Self Service")},
		SelfServiceIcon:   &proclassic.EbookSelfServiceSelfServiceIcon{ID: new(805), URI: new("https://icons/h")},
		SelfServiceCategories: &proclassic.EbookSelfServiceSelfServiceCategories{
			Category: &[]proclassic.EbookSelfServiceSelfServiceCategoriesCategoryItem{
				{ID: new(3), Name: new("eBooks"), DisplayIn: new(true), FeatureIn: new(false)},
			},
		},
	}
	// Prior state declares category 3 with authored display_in/feature_in.
	state := &EbookSelfServiceModel{
		Categories: []EbookSelfServiceCategoryModel{
			{ID: types.StringValue("3"), DisplayIn: types.BoolValue(true), FeatureIn: types.BoolValue(false)},
		},
	}
	flattenEbookSelfService(ss, state)

	if state.NotificationEnabled.IsNull() || !state.NotificationEnabled.ValueBool() {
		t.Errorf("notification_enabled not flattened")
	}
	if state.NotificationMethod.ValueString() != "Self Service" {
		t.Errorf("notification_method not flattened: %q", state.NotificationMethod.ValueString())
	}
	if state.IconID.ValueString() != "805" {
		t.Errorf("icon_id not flattened: %q", state.IconID.ValueString())
	}
	if state.IconURI.ValueString() != "https://icons/h" {
		t.Errorf("icon_uri not flattened: %q", state.IconURI.ValueString())
	}
	if len(state.Categories) != 1 {
		t.Fatalf("categories: want 1, got %d", len(state.Categories))
	}
	c := state.Categories[0]
	if c.ID.ValueString() != "3" || !c.DisplayIn.ValueBool() || c.FeatureIn.ValueBool() {
		t.Errorf("category authored values not preserved: %+v", c)
	}
}

// TestAssignEbookResourceModel_IncludeUnmanagedHydratesFromScratch pins the
// config-generation contract: with includeUnmanaged set and an empty starting
// model, every wire-present section is allocated and hydrated from the server.
func TestAssignEbookResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	state := &EbookResourceModel{}
	src := &proclassic.Ebook{
		ID:      new(9),
		General: &proclassic.EbookGeneral{Name: new("tf-acc"), URL: new("https://example.org/g.pdf")},
		Scope: &proclassic.EbookScope{
			AllComputers: new(true),
			ComputerGroups: &proclassic.EbookScopeComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(11)}, {ID: new(22)}},
			},
			Exclusions: &proclassic.EbookScopeExclusions{
				UserGroups: &proclassic.EbookScopeExclusionsUserGroups{UserGroup: &[]proclassic.IDName{{Name: new("Admins")}}},
			},
		},
		SelfService: &proclassic.EbookSelfService{InstallButtonText: new("Install")},
	}
	diags := assignEbookResourceModel(context.Background(), state, src, true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected scope.targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers hydrated true")
	}
	if got := setLen(state.Scope.Targets.ComputerGroupIDs); got != 2 {
		t.Fatalf("expected 2 computer_group_ids, got %d", got)
	}
	if state.Scope.Exclusions == nil {
		t.Fatal("expected exclusions allocated when wire-present")
	}
	if setLen(state.Scope.Exclusions.DirectoryServiceUserGroupNames) != 1 {
		t.Fatalf("expected 1 excluded user_group name, got %d", setLen(state.Scope.Exclusions.DirectoryServiceUserGroupNames))
	}
	if state.Scope.Limitations != nil {
		t.Fatalf("expected limitations nil when wire-absent; got %+v", state.Scope.Limitations)
	}
	if state.SelfService == nil || state.SelfService.InstallButtonText.ValueString() != "Install" {
		t.Fatalf("expected self_service hydrated; got %+v", state.SelfService)
	}
}

func TestExtractEbookID_PrefersTopLevel(t *testing.T) {
	e := &proclassic.Ebook{ID: new(7), General: &proclassic.EbookGeneral{ID: new(9)}}
	if got := extractEbookID(e); got != "7" {
		t.Errorf("want top-level id 7, got %q", got)
	}
	e = &proclassic.Ebook{General: &proclassic.EbookGeneral{ID: new(9)}}
	if got := extractEbookID(e); got != "9" {
		t.Errorf("want general id 9 fallback, got %q", got)
	}
}
