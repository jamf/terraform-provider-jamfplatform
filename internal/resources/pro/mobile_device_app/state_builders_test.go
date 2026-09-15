// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestFlattenMobileAppGeneral_RoundTrip(t *testing.T) {
	state := &MobileAppGeneralModel{}
	g := &proclassic.MobileDeviceApplicationGeneral{
		ID:             new(42),
		Name:           new("Maps"),
		Version:        new("1.0"),
		BundleID:       new("com.apple.Maps"),
		OsType:         new(osTypeIOS),
		Description:    new("Apple Maps"),
		InternalApp:    new(true),
		Free:           new(true),
		DeploymentType: new(deploymentTypeSelfService),
		ExternalURL:    new("https://example.com/app.ipa"),
		ItunesStoreURL: new("https://apps.apple.com/app/id915056765"),
		ItunesSyncTime: new(1700000000),
		HostExternally: new(true),
		Category:       &proclassic.CategoryObject{ID: new(7), Name: new("Productivity")},
		Site:           &proclassic.SiteObject{ID: new(3), Name: new("HQ")},
	}
	flattenMobileAppGeneral(g, state)

	if state.ID.ValueString() != "42" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.OsType.ValueString() != osTypeIOS {
		t.Errorf("os_type: got %q", state.OsType.ValueString())
	}
	if state.Description.ValueString() != "Apple Maps" {
		t.Errorf("description not flattened: %q", state.Description.ValueString())
	}
	if !state.IsFree.ValueBool() {
		t.Errorf("is_free (free) not flattened")
	}
	if state.ExternalURL.ValueString() != "https://example.com/app.ipa" {
		t.Errorf("external_url not flattened")
	}
	if state.ItunesSyncTime.ValueInt64() != 1700000000 {
		t.Errorf("itunes_sync_time not flattened: %d", state.ItunesSyncTime.ValueInt64())
	}
	if !state.HostExternally.ValueBool() {
		t.Errorf("host_externally not flattened")
	}
	if state.CategoryID.ValueString() != "7" || state.CategoryName.ValueString() != "Productivity" {
		t.Errorf("category not flattened: %q / %q", state.CategoryID.ValueString(), state.CategoryName.ValueString())
	}
	if state.SiteID.ValueString() != "3" || state.SiteName.ValueString() != "HQ" {
		t.Errorf("site not flattened")
	}
}

// TestAssignMobileApp_GuardedBlocks is the core echo-guard test: a minimal app
// created without scope / self_service / vpp / app_configuration blocks must keep
// those blocks null in state even though the server echoes them on GET.
func TestAssignMobileApp_GuardedBlocks(t *testing.T) {
	state := &MobileAppResourceModel{
		General: &MobileAppGeneralModel{Name: types.StringValue("Maps")},
		// Scope / SelfService / Vpp / AppConfiguration intentionally nil (unmanaged).
	}
	server := &proclassic.MobileDeviceApplication{
		ID:      new(42),
		General: &proclassic.MobileDeviceApplicationGeneral{ID: new(42), Name: new("Maps"), OsType: new(osTypeIOS)},
		Scope:   &proclassic.MobileDeviceApplicationScope{AllMobileDevices: new(false), AllJssUsers: new(false)},
		SelfService: &proclassic.MobileDeviceApplicationSelfService{
			SelfServiceInstallButtonText: new("Install"),
		},
		Vpp:              &proclassic.MobileDeviceApplicationVpp{VppAdminAccountID: new(-1)},
		AppConfiguration: &proclassic.MobileDeviceApplicationAppConfiguration{Preferences: new("<dict/>")},
	}

	diags := assignMobileAppResourceModel(context.Background(), state, server, false)
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
	if state.AppConfiguration != nil {
		t.Errorf("unmanaged app_configuration was populated: %+v", state.AppConfiguration)
	}
	if state.ID.ValueString() != "42" {
		t.Errorf("id not set: %q", state.ID.ValueString())
	}
}

// TestAssignMobileAppResourceModel_IncludeUnmanagedHydratesFromScratch pins the
// config-generation contract: with includeUnmanaged set and an empty starting
// model, every wire-present section is allocated and hydrated from the server.
func TestAssignMobileAppResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	state := &MobileAppResourceModel{}
	server := &proclassic.MobileDeviceApplication{
		ID:      new(42),
		General: &proclassic.MobileDeviceApplicationGeneral{ID: new(42), Name: new("Maps"), OsType: new(osTypeIOS)},
		Scope: &proclassic.MobileDeviceApplicationScope{
			AllMobileDevices: new(true),
			MobileDeviceGroups: &proclassic.MobileDeviceApplicationScopeMobileDeviceGroups{
				MobileDeviceGroup: &[]proclassic.IDName{{ID: new(5)}, {ID: new(6)}},
			},
			Exclusions: &proclassic.MobileDeviceApplicationScopeExclusions{
				Users: &proclassic.MobileDeviceApplicationScopeExclusionsUsers{
					User: &[]proclassic.MobileDeviceApplicationScopeExclusionsUsersUserItem{{Name: new("alice")}},
				},
			},
		},
		SelfService:      &proclassic.MobileDeviceApplicationSelfService{SelfServiceInstallButtonText: new("Install")},
		Vpp:              &proclassic.MobileDeviceApplicationVpp{AssignVppDeviceBasedLicenses: new(true)},
		AppConfiguration: &proclassic.MobileDeviceApplicationAppConfiguration{Preferences: new("<dict/>")},
	}
	diags := assignMobileAppResourceModel(context.Background(), state, server, true)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected scope.targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllMobileDevices.ValueBool() {
		t.Fatal("expected all_mobile_devices hydrated true")
	}
	if got := len(state.Scope.Targets.MobileDeviceGroupIDs.Elements()); got != 2 {
		t.Fatalf("expected 2 mobile_device_group_ids, got %d", got)
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
	if state.AppConfiguration == nil || state.AppConfiguration.Preferences.ValueString() != "<dict/>" {
		t.Fatalf("expected app_configuration hydrated; got %+v", state.AppConfiguration)
	}
}

func TestFlattenMobileAppScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	ctx := context.Background()
	// Managed categories carry a non-null current value (declared in config);
	// everything else is null = unmanaged and must stay null.
	state := &scope.MobileScopeModelNoIbeacons{
		Targets: &scope.MobileScopeTargetsModel{
			MobileDeviceIDs: scope.EmptyStringSet(), // managed, refreshes from wire
		},
		Limitations: &scope.MobileScopeLimitationsModelNoIbeacons{
			NetworkSegmentIDs: idSet("9"), // managed, drift-refreshes
		},
		Exclusions: &scope.MobileScopeExclusionsModelNoIbeacons{
			DirectoryServiceOrLocalUserNames: scope.EmptyStringSet(), // managed
		},
	}
	s := &proclassic.MobileDeviceApplicationScope{
		AllMobileDevices: new(false),
		MobileDevices: &proclassic.MobileDeviceApplicationScopeMobileDevices{
			MobileDevice: &[]proclassic.MobileDeviceApplicationScopeMobileDevicesMobileDeviceItem{{ID: new(11)}, {ID: new(12)}},
		},
		MobileDeviceGroups: &proclassic.MobileDeviceApplicationScopeMobileDeviceGroups{
			MobileDeviceGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Limitations: &proclassic.MobileDeviceApplicationScopeLimitations{
			NetworkSegments: &proclassic.MobileDeviceApplicationScopeLimitationsNetworkSegments{
				NetworkSegment: &[]proclassic.IDName{{ID: new(2)}},
			},
		},
		Exclusions: &proclassic.MobileDeviceApplicationScopeExclusions{
			Users: &proclassic.MobileDeviceApplicationScopeExclusionsUsers{
				User: &[]proclassic.MobileDeviceApplicationScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}
	flattenMobileAppScope(ctx, s, state, false)

	var mdIDs []string
	state.Targets.MobileDeviceIDs.ElementsAs(ctx, &mdIDs, false)
	if len(mdIDs) != 2 {
		t.Errorf("managed mobile_device_ids should refresh from wire: got %v", mdIDs)
	}
	if !state.Targets.MobileDeviceGroupIDs.IsNull() {
		t.Errorf("unmanaged mobile_device_group_ids must stay null, got %v", state.Targets.MobileDeviceGroupIDs)
	}
	if !state.Targets.AllMobileDevices.IsNull() {
		t.Errorf("unmanaged all_mobile_devices must stay null, got %v", state.Targets.AllMobileDevices)
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
	if !state.Exclusions.MobileDeviceGroupIDs.IsNull() {
		t.Errorf("unmanaged exclusion mobile_device_group_ids must stay null, got %v", state.Exclusions.MobileDeviceGroupIDs)
	}
}

func TestFlattenMobileAppScope_HydrateAllForMergeBase(t *testing.T) {
	ctx := context.Background()
	// includeUnmanaged=true hydrates every wire-present category into a zero
	// model — the shape Update uses to build the read-merge-write base.
	state := &scope.MobileScopeModelNoIbeacons{}
	s := &proclassic.MobileDeviceApplicationScope{
		AllMobileDevices: new(false),
		MobileDeviceGroups: &proclassic.MobileDeviceApplicationScopeMobileDeviceGroups{
			MobileDeviceGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Exclusions: &proclassic.MobileDeviceApplicationScopeExclusions{
			Users: &proclassic.MobileDeviceApplicationScopeExclusionsUsers{
				User: &[]proclassic.MobileDeviceApplicationScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}
	flattenMobileAppScope(ctx, s, state, true)

	if state.Targets == nil || state.Targets.AllMobileDevices.IsNull() || state.Targets.AllMobileDevices.ValueBool() {
		t.Fatalf("expected all_mobile_devices hydrated false, got %+v", state.Targets)
	}
	var groupIDs []string
	state.Targets.MobileDeviceGroupIDs.ElementsAs(ctx, &groupIDs, false)
	if len(groupIDs) != 1 || groupIDs[0] != "5" {
		t.Errorf("expected mobile_device_group_ids hydrated, got %v", groupIDs)
	}
	if state.Exclusions == nil {
		t.Fatal("expected exclusions allocated")
	}
	var exclUsers []string
	state.Exclusions.DirectoryServiceOrLocalUserNames.ElementsAs(ctx, &exclUsers, false)
	if len(exclUsers) != 1 || exclUsers[0] != "alice" {
		t.Errorf("expected exclusion user names hydrated, got %v", exclUsers)
	}
	if state.Limitations != nil {
		t.Fatalf("expected limitations nil when wire-absent; got %+v", state.Limitations)
	}
}

func TestFlattenMobileAppVpp(t *testing.T) {
	state := &MobileAppVppModel{}
	v := &proclassic.MobileDeviceApplicationVpp{
		AssignVppDeviceBasedLicenses: new(true),
		VppAdminAccountID:            new(4),
	}
	flattenMobileAppVpp(v, state)
	if !state.AssignVppDeviceBasedLicenses.ValueBool() {
		t.Errorf("assign not flattened")
	}
	if state.VppAdminAccountID.ValueString() != "4" {
		t.Errorf("vpp_admin_account_id: got %q", state.VppAdminAccountID.ValueString())
	}
}

func TestFlattenMobileAppSelfService_Notification(t *testing.T) {
	state := &MobileAppSelfServiceModel{}
	ss := &proclassic.MobileDeviceApplicationSelfService{
		Notification: &proclassic.NotificationValue{Enabled: new(true)},
	}
	flattenMobileAppSelfService(ss, state, false)
	if !state.NotificationEnabled.ValueBool() {
		t.Errorf("notification_enabled not flattened")
	}
}

// TestFlattenMobileAppSelfService_HydratesIconAndCategoriesOnImport covers
// first-time import, where the caller allocates an empty self_service block:
// the zero-value struct carries a nil icon pointer and a nil category slice, so
// without the release neither ever populated.
func TestFlattenMobileAppSelfService_HydratesIconAndCategoriesOnImport(t *testing.T) {
	ss := &proclassic.MobileDeviceApplicationSelfService{
		SelfServiceIcon: &proclassic.MobileDeviceApplicationSelfServiceSelfServiceIcon{
			ID:  new(12),
			URI: new("https://example.invalid/icon.png"),
		},
		SelfServiceCategories: &proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategories{
			Category: &[]proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategoriesCategoryItem{
				{ID: new(4), Name: new("Productivity"), DisplayIn: new(true)},
			},
		},
	}

	state := &MobileAppSelfServiceModel{}
	flattenMobileAppSelfService(ss, state, true)

	if state.SelfServiceIcon == nil {
		t.Fatal("self_service_icon must hydrate on import")
	}
	if state.SelfServiceIcon.ID.ValueString() != "12" {
		t.Errorf("self_service_icon.id = %q, want 12", state.SelfServiceIcon.ID.ValueString())
	}
	if len(state.SelfServiceCategories) != 1 {
		t.Fatalf("self_service_categories expected 1 entry, got %d", len(state.SelfServiceCategories))
	}
	if state.SelfServiceCategories[0].Name.ValueString() != "Productivity" {
		t.Errorf("categories[0].name = %q, want Productivity", state.SelfServiceCategories[0].Name.ValueString())
	}
}

// TestFlattenMobileAppSelfService_ImportWithNoCategoriesStaysNil asserts the
// non-empty rule: storing an empty set where a create that never declared the
// attribute stores null would break ImportStateVerify.
func TestFlattenMobileAppSelfService_ImportWithNoCategoriesStaysNil(t *testing.T) {
	ss := &proclassic.MobileDeviceApplicationSelfService{
		SelfServiceCategories: &proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategories{
			Category: &[]proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategoriesCategoryItem{},
		},
	}
	state := &MobileAppSelfServiceModel{}
	flattenMobileAppSelfService(ss, state, true)
	if state.SelfServiceCategories != nil {
		t.Errorf("categories must stay nil when the server carries none, got %+v", state.SelfServiceCategories)
	}
	if state.SelfServiceIcon != nil {
		t.Errorf("icon must stay nil when the server omits it, got %+v", state.SelfServiceIcon)
	}
}

// TestFlattenMobileAppSelfService_RefreshDoesNotFabricate asserts an ordinary
// refresh still leaves an unauthored icon and category set alone.
func TestFlattenMobileAppSelfService_RefreshDoesNotFabricate(t *testing.T) {
	ss := &proclassic.MobileDeviceApplicationSelfService{
		SelfServiceIcon: &proclassic.MobileDeviceApplicationSelfServiceSelfServiceIcon{ID: new(12)},
		SelfServiceCategories: &proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategories{
			Category: &[]proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategoriesCategoryItem{
				{ID: new(4), Name: new("Productivity")},
			},
		},
	}
	state := &MobileAppSelfServiceModel{}
	flattenMobileAppSelfService(ss, state, false)
	if state.SelfServiceIcon != nil {
		t.Errorf("unauthored icon must stay nil on a refresh, got %+v", state.SelfServiceIcon)
	}
	if state.SelfServiceCategories != nil {
		t.Errorf("unauthored categories must stay nil on a refresh, got %+v", state.SelfServiceCategories)
	}
}
