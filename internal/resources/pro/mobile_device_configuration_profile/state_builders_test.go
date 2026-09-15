// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestFlattenGeneral_LevelWireReadTranslated(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.MobileDeviceConfigurationProfileGeneral{
		ID:    new(42),
		Name:  new("MobileX"),
		Level: new(levelWireReadDevice),
	}, state)
	if state.ID.ValueString() != "42" {
		t.Fatalf("ID: got %v", state.ID)
	}
	if state.Level.ValueString() != levelUIDevice {
		t.Fatalf("Level: got %q want %q", state.Level.ValueString(), levelUIDevice)
	}
}

func TestFlattenGeneral_LevelUserSymmetric(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.MobileDeviceConfigurationProfileGeneral{Level: new(levelWireReadUser)}, state)
	if state.Level.ValueString() != levelUIUser {
		t.Fatalf("got %q want %q", state.Level.ValueString(), levelUIUser)
	}
}

func TestFlattenGeneral_RedeployDaysBeforeCertificateExpires(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.MobileDeviceConfigurationProfileGeneral{
		RedeployDaysBeforeCertificateExpires: new(7),
	}, state)
	if state.RedeployDaysBeforeCertificateExpires.ValueInt64() != 7 {
		t.Fatalf("RedeployDays: got %v", state.RedeployDaysBeforeCertificateExpires)
	}
}

func TestFlattenGeneral_CategoryAndSitePopulated(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.MobileDeviceConfigurationProfileGeneral{
		Category: &proclassic.CategoryObject{ID: new(58), Name: new("Applications")},
		Site:     &proclassic.SiteObject{ID: new(-1), Name: new("None")},
	}, state)
	if state.CategoryID.ValueString() != "58" {
		t.Fatalf("CategoryID: got %v", state.CategoryID)
	}
	if state.CategoryName.ValueString() != "Applications" {
		t.Fatalf("CategoryName: got %v", state.CategoryName)
	}
	if state.SiteID.ValueString() != "-1" {
		t.Fatalf("SiteID: got %v", state.SiteID)
	}
}

func TestFlattenGeneral_DeploymentMethodMappedToDistributionMethod(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.MobileDeviceConfigurationProfileGeneral{
		DeploymentMethod: new(distributionMethodInstallAutomatically),
	}, state)
	if state.DistributionMethod.ValueString() != distributionMethodInstallAutomatically {
		t.Fatalf("DistributionMethod: got %v", state.DistributionMethod)
	}
}

func TestFlattenGeneral_UUIDAndPayloadsExposed(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.MobileDeviceConfigurationProfileGeneral{
		UUID:     new("test-uuid"),
		Payloads: new(proclassic.PayloadsXMLText("<plist/>")),
	}, state)
	if state.UUID.ValueString() != "test-uuid" {
		t.Fatalf("UUID: got %v", state.UUID)
	}
	if got := state.Payloads.ValueString(); !strings.Contains(got, "<plist") {
		t.Fatalf("Payloads: got %v", got)
	}
}

// TestFlattenScope_ManagedRefreshUnmanagedStaysNull pins the per-category
// ownership gate: a managed category (non-null current value) refreshes from
// the wire — including to `[]` when the wire element is absent — while an
// unmanaged (null) category stays null so members maintained in the admin UI
// never enter state.
func TestFlattenScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := &scope.MobileScopeModel{
		Targets: &scope.MobileScopeTargetsModel{
			AllMobileDevices: types.BoolValue(false), // managed, refreshes from wire
			MobileDeviceIDs:  scope.EmptyStringSet(), // managed, refreshes from wire
		},
		Limitations: &scope.MobileScopeLimitationsModel{
			NetworkSegmentIDs: stringSet(t, "9"), // managed, drift-refreshes
		},
		Exclusions: &scope.MobileScopeExclusionsModel{
			DirectoryServiceOrLocalUserNames: scope.EmptyStringSet(), // managed
		},
	}
	diags := flattenScope(ctx, &proclassic.MobileDeviceConfigurationProfileScope{
		AllMobileDevices: new(true),
		MobileDevices: &proclassic.MobileDeviceConfigurationProfileScopeMobileDevices{
			MobileDevice: &[]proclassic.MobileDeviceConfigurationProfileScopeMobileDevicesMobileDeviceItem{{ID: new(11)}, {ID: new(12)}},
		},
		MobileDeviceGroups: &proclassic.MobileDeviceConfigurationProfileScopeMobileDeviceGroups{
			MobileDeviceGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Limitations: &proclassic.MobileDeviceConfigurationProfileScopeLimitations{
			NetworkSegments: &proclassic.MobileDeviceConfigurationProfileScopeLimitationsNetworkSegments{
				NetworkSegment: &[]proclassic.IDName{{ID: new(2)}},
			},
		},
		Exclusions: &proclassic.MobileDeviceConfigurationProfileScopeExclusions{
			Users: &proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsers{
				User: &[]proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}, state, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if !state.Targets.AllMobileDevices.ValueBool() {
		t.Fatal("managed all_mobile_devices should refresh from wire")
	}
	var mdIDs []string
	state.Targets.MobileDeviceIDs.ElementsAs(ctx, &mdIDs, false)
	if len(mdIDs) != 2 {
		t.Errorf("managed mobile_device_ids should refresh from wire: got %v", mdIDs)
	}
	if !state.Targets.MobileDeviceGroupIDs.IsNull() {
		t.Errorf("unmanaged mobile_device_group_ids must stay null, got %v", state.Targets.MobileDeviceGroupIDs)
	}
	if !state.Targets.AllJssUsers.IsNull() {
		t.Errorf("unmanaged all_jss_users must stay null, got %v", state.Targets.AllJssUsers)
	}
	var segIDs []string
	state.Limitations.NetworkSegmentIDs.ElementsAs(ctx, &segIDs, false)
	if len(segIDs) != 1 || segIDs[0] != "2" {
		t.Errorf("managed limitations network_segment_ids should drift-refresh: got %v", segIDs)
	}
	if !state.Limitations.IbeaconIDs.IsNull() {
		t.Errorf("unmanaged limitations ibeacon_ids must stay null, got %v", state.Limitations.IbeaconIDs)
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

// TestFlattenScope_HydrateAllForMergeBase: includeUnmanaged=true hydrates every
// wire-present category into a zero model — the shape Update uses to build the
// read-merge-write base (and import / config generation use for full state).
func TestFlattenScope_HydrateAllForMergeBase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := &scope.MobileScopeModel{}
	diags := flattenScope(ctx, &proclassic.MobileDeviceConfigurationProfileScope{
		AllMobileDevices: new(false),
		MobileDeviceGroups: &proclassic.MobileDeviceConfigurationProfileScopeMobileDeviceGroups{
			MobileDeviceGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Exclusions: &proclassic.MobileDeviceConfigurationProfileScopeExclusions{
			Users: &proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsers{
				User: &[]proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}, state, true)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

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

func TestFlattenScope_ReconcileLeavesNullWhenStateUnconfigured(t *testing.T) {
	t.Parallel()
	state := &scope.MobileScopeModel{}
	flattenScope(context.Background(), &proclassic.MobileDeviceConfigurationProfileScope{
		AllMobileDevices: new(true),
	}, state, false)
	if state.Targets != nil {
		t.Fatalf("expected Targets to stay nil for unconfigured state, got %v", state.Targets)
	}
}

func TestFlattenScope_MobileDeviceIDsPopulated(t *testing.T) {
	t.Parallel()
	// Declared (managed) category: a non-null current value refreshes from wire.
	state := &scope.MobileScopeModel{Targets: &scope.MobileScopeTargetsModel{MobileDeviceIDs: scope.EmptyStringSet()}}
	diags := flattenScope(context.Background(), &proclassic.MobileDeviceConfigurationProfileScope{
		MobileDevices: &proclassic.MobileDeviceConfigurationProfileScopeMobileDevices{
			MobileDevice: &[]proclassic.MobileDeviceConfigurationProfileScopeMobileDevicesMobileDeviceItem{
				{ID: new(12)},
				{ID: new(31)},
			},
		},
	}, state, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.Targets.MobileDeviceIDs.IsNull() {
		t.Fatal("expected MobileDeviceIDs populated")
	}
	var ids []string
	if dd := state.Targets.MobileDeviceIDs.ElementsAs(context.Background(), &ids, false); dd.HasError() {
		t.Fatalf("ElementsAs: %v", dd)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d: %v", len(ids), ids)
	}
}

func TestFlattenScopeLimitations_DSUserNamesAndNetworkSegments(t *testing.T) {
	t.Parallel()
	// Declared (managed) categories carry a non-null current value and refresh
	// from wire; ibeacon_ids / directory_service_user_group_names stay null
	// (unmanaged).
	state := &scope.MobileScopeLimitationsModel{
		DirectoryServiceOrLocalUserNames: scope.EmptyStringSet(),
		NetworkSegmentIDs:                scope.EmptyStringSet(),
	}
	diags := flattenScopeLimitations(context.Background(), &proclassic.MobileDeviceConfigurationProfileScopeLimitations{
		Users: &proclassic.MobileDeviceConfigurationProfileScopeLimitationsUsers{
			User: &[]proclassic.IDName{{Name: new("alice")}, {Name: new("bob")}},
		},
		NetworkSegments: &proclassic.MobileDeviceConfigurationProfileScopeLimitationsNetworkSegments{
			NetworkSegment: &[]proclassic.IDName{{ID: new(5)}},
		},
	}, state, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	var names []string
	if dd := state.DirectoryServiceOrLocalUserNames.ElementsAs(context.Background(), &names, false); dd.HasError() {
		t.Fatalf("ElementsAs users: %v", dd)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 user names, got %v", names)
	}
	var segIDs []string
	if dd := state.NetworkSegmentIDs.ElementsAs(context.Background(), &segIDs, false); dd.HasError() {
		t.Fatalf("ElementsAs segs: %v", dd)
	}
	if len(segIDs) != 1 || segIDs[0] != "5" {
		t.Fatalf("expected [\"5\"], got %v", segIDs)
	}
	if !state.IbeaconIDs.IsNull() {
		t.Fatalf("unmanaged ibeacon_ids must stay null, got %v", state.IbeaconIDs)
	}
}

func TestFlattenScopeExclusions_MobileDevicesAndUserGroups(t *testing.T) {
	t.Parallel()
	// Declared (managed) categories refresh from wire; the rest stay null.
	state := &scope.MobileScopeExclusionsModel{
		MobileDeviceIDs:                scope.EmptyStringSet(),
		DirectoryServiceUserGroupNames: scope.EmptyStringSet(),
	}
	diags := flattenScopeExclusions(context.Background(), &proclassic.MobileDeviceConfigurationProfileScopeExclusions{
		MobileDevices: &proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevices{
			MobileDevice: &[]proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevicesMobileDeviceItem{
				{ID: new(12)},
			},
		},
		UserGroups: &proclassic.MobileDeviceConfigurationProfileScopeExclusionsUserGroups{
			UserGroup: &[]proclassic.IDName{{Name: new("DS-Group")}},
		},
	}, state, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	var ids []string
	state.MobileDeviceIDs.ElementsAs(context.Background(), &ids, false)
	if len(ids) != 1 || ids[0] != "12" {
		t.Fatalf("expected mobile device 12, got %v", ids)
	}
	var names []string
	state.DirectoryServiceUserGroupNames.ElementsAs(context.Background(), &names, false)
	if len(names) != 1 || names[0] != "DS-Group" {
		t.Fatalf("expected DS-Group, got %v", names)
	}
	if !state.BuildingIDs.IsNull() {
		t.Fatalf("unmanaged exclusion building_ids must stay null, got %v", state.BuildingIDs)
	}
}

func TestFlattenSelfService_SecurityRemovalDisallowed(t *testing.T) {
	t.Parallel()
	state := &SelfServiceModel{RemovalDisallowed: types.StringValue("")}
	flattenSelfService(&proclassic.MobileDeviceConfigurationProfileSelfService{
		Security: &proclassic.MobileDeviceConfigurationProfileSelfServiceSecurity{
			RemovalDisallowed: new(removalDisallowedNever),
		},
	}, state, false)
	if state.RemovalDisallowed.ValueString() != removalDisallowedNever {
		t.Fatalf("RemovalDisallowed: got %q", state.RemovalDisallowed.ValueString())
	}
}

func TestFlattenSelfService_AuthorizationPasswordReconciled(t *testing.T) {
	t.Parallel()
	state := &SelfServiceModel{AuthorizationPassword: types.StringValue("")}
	flattenSelfService(&proclassic.MobileDeviceConfigurationProfileSelfService{
		Security: &proclassic.MobileDeviceConfigurationProfileSelfServiceSecurity{
			RemovalDisallowed: new(removalDisallowedWithAuthorization),
			Password:          new("s3cr3t"),
		},
	}, state, false)
	if state.AuthorizationPassword.ValueString() != "s3cr3t" {
		t.Fatalf("AuthorizationPassword: got %q", state.AuthorizationPassword.ValueString())
	}
}

func TestFlattenSelfService_Categories(t *testing.T) {
	t.Parallel()

	wire := func() *proclassic.MobileDeviceConfigurationProfileSelfService {
		return &proclassic.MobileDeviceConfigurationProfileSelfService{
			SelfServiceCategories: &proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories{
				Category: &[]proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem{
					{ID: new(58), Name: new("Applications")},
					{ID: new(44), Name: new("Auto-Update")},
				},
			},
		}
	}

	t.Run("managed: the wire is authoritative", func(t *testing.T) {
		t.Parallel()
		state := &SelfServiceModel{Categories: []SelfServiceCategoryItem{{ID: types.StringValue("999")}}}
		flattenSelfService(wire(), state, false)
		if len(state.Categories) != 2 {
			t.Fatalf("expected 2 categories, got %d", len(state.Categories))
		}
		if state.Categories[0].ID.ValueString() != "58" || state.Categories[1].ID.ValueString() != "44" {
			t.Fatalf("category IDs: got %v / %v", state.Categories[0].ID, state.Categories[1].ID)
		}
		if state.Categories[0].Name.ValueString() != "Applications" {
			t.Fatalf("category[0].Name: got %q", state.Categories[0].Name.ValueString())
		}
	})

	t.Run("managed and the wire is empty: converges on the declared empty list", func(t *testing.T) {
		t.Parallel()
		state := &SelfServiceModel{Categories: []SelfServiceCategoryItem{}}
		flattenSelfService(&proclassic.MobileDeviceConfigurationProfileSelfService{}, state, false)
		if state.Categories == nil {
			t.Fatal("a declared empty list must stay non-nil, or `categories = []` never converges")
		}
		if len(state.Categories) != 0 {
			t.Fatalf("expected 0 categories, got %d", len(state.Categories))
		}
	})

	t.Run("unmanaged on an ordinary refresh: left alone", func(t *testing.T) {
		t.Parallel()
		// The #392 regression, which names this resource alongside the macOS
		// profile. Hydrating here is what made dropping the attribute fail
		// "inconsistent result after apply".
		state := &SelfServiceModel{}
		flattenSelfService(wire(), state, false)
		if state.Categories != nil {
			t.Fatalf("an unmanaged attribute must stay nil, got %d categories", len(state.Categories))
		}
	})

	t.Run("unmanaged on first hydration: populated", func(t *testing.T) {
		t.Parallel()
		// import / config generation / the Update merge base.
		state := &SelfServiceModel{}
		flattenSelfService(wire(), state, true)
		if len(state.Categories) != 2 {
			t.Fatalf("import must hydrate, got %d categories", len(state.Categories))
		}
	})

	t.Run("unmanaged first hydration with nothing on the server: still nil", func(t *testing.T) {
		t.Parallel()
		state := &SelfServiceModel{}
		flattenSelfService(&proclassic.MobileDeviceConfigurationProfileSelfService{}, state, true)
		if state.Categories != nil {
			t.Fatal("import must not fabricate an empty list the practitioner never wrote")
		}
	})
}

func TestAssignResourceModel_TopLevelIDFromGeneral(t *testing.T) {
	t.Parallel()
	state := &ResourceModel{}
	diags := assignResourceModel(context.Background(), state, &proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{ID: new(99), Name: new("X")},
	}, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "99" {
		t.Fatalf("ID: got %v", state.ID)
	}
}

func TestAssignResourceModel_OptionalSubBlocksSkippedWhenStateNil(t *testing.T) {
	t.Parallel()
	state := &ResourceModel{}
	diags := assignResourceModel(context.Background(), state, &proclassic.MobileDeviceConfigurationProfile{
		ID:      new(1),
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{Name: new("X")},
		Scope: &proclassic.MobileDeviceConfigurationProfileScope{
			AllMobileDevices: new(true),
		},
		SelfService: &proclassic.MobileDeviceConfigurationProfileSelfService{
			SelfServiceDescription: new("desc"),
		},
	}, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.Scope != nil {
		t.Fatalf("expected Scope unchanged; got %+v", state.Scope)
	}
	if state.SelfService != nil {
		t.Fatalf("expected SelfService unchanged; got %+v", state.SelfService)
	}
}

func TestAssignResourceModel_PopulatedSubBlocksRefreshed(t *testing.T) {
	t.Parallel()
	state := &ResourceModel{
		Scope:       &scope.MobileScopeModel{Targets: &scope.MobileScopeTargetsModel{AllMobileDevices: types.BoolValue(false)}},
		SelfService: &SelfServiceModel{RemovalDisallowed: types.StringValue("")},
	}
	diags := assignResourceModel(context.Background(), state, &proclassic.MobileDeviceConfigurationProfile{
		ID:      new(2),
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{Name: new("X")},
		Scope:   &proclassic.MobileDeviceConfigurationProfileScope{AllMobileDevices: new(true)},
		SelfService: &proclassic.MobileDeviceConfigurationProfileSelfService{
			Security: &proclassic.MobileDeviceConfigurationProfileSelfServiceSecurity{
				RemovalDisallowed: new(removalDisallowedNever),
			},
		},
	}, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.Scope.Targets.AllMobileDevices.ValueBool() {
		t.Fatal("Scope.Targets.AllMobileDevices not refreshed")
	}
	if state.SelfService.RemovalDisallowed.ValueString() != removalDisallowedNever {
		t.Fatalf("SelfService.RemovalDisallowed: got %q", state.SelfService.RemovalDisallowed.ValueString())
	}
}

// TestAssignResourceModel_IncludeUnmanagedHydratesFromScratch pins the
// config-generation contract: with includeUnmanaged set and an empty starting
// model, every wire-present optional section is allocated and hydrated.
func TestAssignResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	t.Parallel()
	state := &ResourceModel{}
	diags := assignResourceModel(context.Background(), state, &proclassic.MobileDeviceConfigurationProfile{
		ID:      new(7),
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{Name: new("X")},
		Scope: &proclassic.MobileDeviceConfigurationProfileScope{
			AllMobileDevices: new(true),
			Exclusions:       &proclassic.MobileDeviceConfigurationProfileScopeExclusions{},
		},
		SelfService: &proclassic.MobileDeviceConfigurationProfileSelfService{
			SelfServiceDescription: new("desc"),
		},
	}, true)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected Scope.Targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllMobileDevices.ValueBool() {
		t.Fatal("expected all_mobile_devices hydrated true")
	}
	if state.Scope.Exclusions == nil {
		t.Fatal("expected Exclusions allocated when wire-present")
	}
	if state.Scope.Limitations != nil {
		t.Fatalf("expected Limitations to stay nil when wire-absent; got %+v", state.Scope.Limitations)
	}
	if state.SelfService == nil {
		t.Fatalf("expected SelfService hydrated; got nil")
	}
}

// TestBuildSelfService_NeverSendsASelfServiceDescription pins that the input
// builder emits no <self_service_description>, and says why, because the
// obvious "the schema is missing a field the API has" reading is wrong.
//
// Wire-probed 2026-09-07 across eighteen create and update combinations: the
// element is a write-alias for <description>. Whatever it carried replaced
// general.description, an empty value wiped it, and the read returned an empty
// element every time. Sending it can only overwrite a sibling the practitioner
// set, so the attribute was removed (issue #393). The admin UI does show a
// separate Self Service description and the macOS profile endpoint keeps the two
// independent, so this is a Jamf Pro defect; if it is fixed, restore the
// attribute rather than reviving the element here on its own.
func TestBuildSelfService_NeverSendsASelfServiceDescription(t *testing.T) {
	ss, diags := buildSelfService(&SelfServiceModel{
		FeatureOnMainPage: types.BoolValue(true),
		RemovalDisallowed: types.StringValue("Never"),
	})
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if ss == nil {
		t.Fatal("self_service payload must be built")
	}
	if ss.SelfServiceDescription != nil {
		t.Errorf("self_service_description must never be sent; got %q", *ss.SelfServiceDescription)
	}
}
