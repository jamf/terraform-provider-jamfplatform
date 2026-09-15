// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func TestFlattenGeneral_LevelWireReadTranslated(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.OsXConfigurationProfileGeneral{
		ID:    new(42),
		Name:  new("ProfileX"),
		Level: new(levelWireReadCC),
	}, state)
	if state.ID.ValueString() != "42" {
		t.Fatalf("ID: got %v", state.ID)
	}
	if state.Level.ValueString() != levelUIComputer {
		t.Fatalf("Level: got %q want %q", state.Level.ValueString(), levelUIComputer)
	}
}

func TestFlattenGeneral_LevelUserSymmetric(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.OsXConfigurationProfileGeneral{Level: new(levelWireReadUC)}, state)
	if state.Level.ValueString() != levelUIUser {
		t.Fatalf("got %q want %q", state.Level.ValueString(), levelUIUser)
	}
}

func TestFlattenGeneral_CategoryAndSitePopulated(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.OsXConfigurationProfileGeneral{
		Category: &proclassic.CategoryObject{ID: new(64), Name: new("All Desktops")},
		Site:     &proclassic.SiteObject{ID: new(-1), Name: new("None")},
	}, state)
	if state.CategoryID.ValueString() != "64" {
		t.Fatalf("CategoryID: got %v", state.CategoryID)
	}
	if state.CategoryName.ValueString() != "All Desktops" {
		t.Fatalf("CategoryName: got %v", state.CategoryName)
	}
	if state.SiteID.ValueString() != "-1" {
		t.Fatalf("SiteID: got %v", state.SiteID)
	}
}

func TestFlattenGeneral_UUIDAndPayloadsExposed(t *testing.T) {
	t.Parallel()
	state := &GeneralModel{}
	flattenGeneral(&proclassic.OsXConfigurationProfileGeneral{
		UUID:     new("abc-uuid"),
		Payloads: new(proclassic.PayloadsXMLText("<plist/>")),
	}, state)
	if state.UUID.ValueString() != "abc-uuid" {
		t.Fatalf("UUID: got %v", state.UUID)
	}
	if got := state.Payloads.ValueString(); !strings.Contains(got, "<plist") {
		t.Fatalf("Payloads: got %v", got)
	}
}

func TestFlattenScope_NilSubBlocksProduceEmptySets(t *testing.T) {
	t.Parallel()
	// Managed (declared) categories flatten an absent SDK sub-block to an
	// empty set, never null: a null would trip the post-apply consistency
	// check on a declared `[]`. Undeclared categories are covered by
	// TestFlattenScope_ManagedRefreshUnmanagedStaysNull.
	state := &scope.ComputerScopeModel{Targets: &scope.ComputerScopeTargetsModel{
		AllComputers:     types.BoolValue(false),
		ComputerIDs:      scope.EmptyStringSet(),
		ComputerGroupIDs: scope.EmptyStringSet(),
		BuildingIDs:      scope.EmptyStringSet(),
		DepartmentIDs:    scope.EmptyStringSet(),
		UserIDs:          scope.EmptyStringSet(),
		UserGroupIDs:     scope.EmptyStringSet(),
	}}
	diags := flattenScope(context.Background(), &proclassic.OsXConfigurationProfileScope{
		AllComputers: new(true),
	}, state, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.Targets.AllComputers.ValueBool() {
		t.Fatal("all_computers not propagated")
	}
	for label, s := range map[string]types.Set{
		"ComputerIDs":      state.Targets.ComputerIDs,
		"ComputerGroupIDs": state.Targets.ComputerGroupIDs,
		"BuildingIDs":      state.Targets.BuildingIDs,
		"DepartmentIDs":    state.Targets.DepartmentIDs,
		"UserIDs":          state.Targets.UserIDs,
		"UserGroupIDs":     state.Targets.UserGroupIDs,
	} {
		if s.IsNull() || len(s.Elements()) != 0 {
			t.Fatalf("%s expected empty set when SDK sub-block absent, got %v", label, s)
		}
	}
}

// TestFlattenScope_ReconcileLeavesNullWhenStateUnconfigured pins the granular
// ownership contract: a user who never declared `all_computers` stays at null
// in state even when the server reports a value — the toggle is owned outside
// Terraform.
func TestFlattenScope_ReconcileLeavesNullWhenStateUnconfigured(t *testing.T) {
	t.Parallel()
	state := &scope.ComputerScopeModel{Targets: &scope.ComputerScopeTargetsModel{}}
	flattenScope(context.Background(), &proclassic.OsXConfigurationProfileScope{
		AllComputers: new(true),
	}, state, false)
	if !state.Targets.AllComputers.IsNull() {
		t.Fatalf("expected AllComputers to stay null for unconfigured state, got %v", state.Targets.AllComputers)
	}
}

func TestFlattenScope_ComputerIDsPopulated(t *testing.T) {
	t.Parallel()
	// computer_ids declared (managed) so the wire members refresh into state.
	state := &scope.ComputerScopeModel{Targets: &scope.ComputerScopeTargetsModel{
		ComputerIDs: scope.EmptyStringSet(),
	}}
	src := &proclassic.OsXConfigurationProfileScope{
		Computers: &proclassic.OsXConfigurationProfileScopeComputers{
			Computer: &[]proclassic.OsXConfigurationProfileScopeComputersComputerItem{
				{ID: new(28), Name: new("hostA")},
				{ID: new(31), Name: new("hostB")},
			},
		},
	}
	diags := flattenScope(context.Background(), src, state, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.Targets.ComputerIDs.IsNull() {
		t.Fatal("expected ComputerIDs populated")
	}
	var ids []string
	dd := state.Targets.ComputerIDs.ElementsAs(context.Background(), &ids, false)
	if dd.HasError() {
		t.Fatalf("ElementsAs: %v", dd)
	}
	if !contains(ids, "28") || !contains(ids, "31") {
		t.Fatalf("expected 28+31 in computer_ids; got %v", ids)
	}
}

func TestFlattenScopeLimitations_NetworkSegmentWithUID(t *testing.T) {
	t.Parallel()
	state := &scope.ComputerScopeLimitationsModel{
		NetworkSegmentIDs: scope.EmptyStringSet(), // managed
	}
	src := &proclassic.OsXConfigurationProfileScopeLimitations{
		NetworkSegments: &proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegments{
			NetworkSegment: &[]proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegmentsNetworkSegmentItem{
				{ID: new(5), Name: new("Lab"), Uid: new("43_5")},
			},
		},
	}
	flattenScopeLimitations(context.Background(), src, state, false)
	var ids []string
	if dd := state.NetworkSegmentIDs.ElementsAs(context.Background(), &ids, false); dd.HasError() {
		t.Fatalf("ElementsAs: %v", dd)
	}
	if len(ids) != 1 || ids[0] != "5" {
		t.Fatalf("expected [\"5\"], got %v", ids)
	}
}

func TestFlattenScopeLimitations_DSUserNames(t *testing.T) {
	t.Parallel()
	state := &scope.ComputerScopeLimitationsModel{
		DirectoryServiceOrLocalUserNames: scope.EmptyStringSet(), // managed
	}
	flattenScopeLimitations(context.Background(), &proclassic.OsXConfigurationProfileScopeLimitations{
		Users: &proclassic.OsXConfigurationProfileScopeLimitationsUsers{
			User: &[]proclassic.IDName{{Name: new("alice")}, {Name: new("bob")}},
		},
	}, state, false)
	var names []string
	if dd := state.DirectoryServiceOrLocalUserNames.ElementsAs(context.Background(), &names, false); dd.HasError() {
		t.Fatalf("ElementsAs: %v", dd)
	}
	if len(names) != 2 || !contains(names, "alice") || !contains(names, "bob") {
		t.Fatalf("expected alice+bob, got %v", names)
	}
}

func TestFlattenScopeExclusions_ComputersAndUserGroupsByName(t *testing.T) {
	t.Parallel()
	state := &scope.ComputerScopeExclusionsModel{
		ComputerIDs:                    scope.EmptyStringSet(), // managed
		DirectoryServiceUserGroupNames: scope.EmptyStringSet(), // managed
	}
	src := &proclassic.OsXConfigurationProfileScopeExclusions{
		Computers: &proclassic.OsXConfigurationProfileScopeExclusionsComputers{
			Computer: &[]proclassic.OsXConfigurationProfileScopeExclusionsComputersComputerItem{
				{ID: new(28)},
			},
		},
		UserGroups: &proclassic.OsXConfigurationProfileScopeExclusionsUserGroups{
			UserGroup: &[]proclassic.IDName{{Name: new("DS-Group")}},
		},
	}
	flattenScopeExclusions(context.Background(), src, state, false)
	var ids, names []string
	state.ComputerIDs.ElementsAs(context.Background(), &ids, false)
	state.DirectoryServiceUserGroupNames.ElementsAs(context.Background(), &names, false)
	if len(ids) != 1 || ids[0] != "28" {
		t.Fatalf("expected computer 28, got %v", ids)
	}
	if len(names) != 1 || names[0] != "DS-Group" {
		t.Fatalf("expected DS-Group, got %v", names)
	}
}

func TestFlattenScope_ManagedRefreshUnmanagedStaysNull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Managed categories carry a non-null current value (declared in config);
	// everything else is null = unmanaged and must stay null.
	state := &scope.ComputerScopeModel{
		Targets: &scope.ComputerScopeTargetsModel{
			ComputerIDs: scope.EmptyStringSet(), // managed, refreshes from wire
		},
		Limitations: &scope.ComputerScopeLimitationsModel{
			NetworkSegmentIDs: stringSet(t, "9"), // managed, drift-refreshes
		},
		Exclusions: &scope.ComputerScopeExclusionsModel{
			DirectoryServiceOrLocalUserNames: scope.EmptyStringSet(), // managed
		},
	}
	src := &proclassic.OsXConfigurationProfileScope{
		AllComputers: new(false),
		Computers: &proclassic.OsXConfigurationProfileScopeComputers{
			Computer: &[]proclassic.OsXConfigurationProfileScopeComputersComputerItem{{ID: new(11)}, {ID: new(12)}},
		},
		ComputerGroups: &proclassic.OsXConfigurationProfileScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Limitations: &proclassic.OsXConfigurationProfileScopeLimitations{
			NetworkSegments: &proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegments{
				NetworkSegment: &[]proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegmentsNetworkSegmentItem{{ID: new(2)}},
			},
		},
		Exclusions: &proclassic.OsXConfigurationProfileScopeExclusions{
			Users: &proclassic.OsXConfigurationProfileScopeExclusionsUsers{
				User: &[]proclassic.OsXConfigurationProfileScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}
	flattenScope(ctx, src, state, false)

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
	if !state.Limitations.IbeaconIDs.IsNull() {
		t.Errorf("unmanaged limitations ibeacon_ids must stay null, got %v", state.Limitations.IbeaconIDs)
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

func TestFlattenScope_HydrateAllForMergeBase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// includeUnmanaged=true hydrates every wire-present category into a zero
	// model — the shape Update uses to build the read-merge-write base.
	state := &scope.ComputerScopeModel{}
	src := &proclassic.OsXConfigurationProfileScope{
		AllComputers: new(false),
		ComputerGroups: &proclassic.OsXConfigurationProfileScopeComputerGroups{
			ComputerGroup: &[]proclassic.IDName{{ID: new(5)}},
		},
		Exclusions: &proclassic.OsXConfigurationProfileScopeExclusions{
			Users: &proclassic.OsXConfigurationProfileScopeExclusionsUsers{
				User: &[]proclassic.OsXConfigurationProfileScopeExclusionsUsersUserItem{{Name: new("alice")}},
			},
		},
	}
	flattenScope(ctx, src, state, true)

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
	if state.Limitations != nil {
		t.Fatalf("expected limitations nil when wire-absent; got %+v", state.Limitations)
	}
}

func TestFlattenSelfService_NotificationRecombined(t *testing.T) {
	t.Parallel()
	// Pre-configure each field so ReconcileOptional* helpers substitute the
	// server response. The wire is reliable post-SDK fix (Method-first marshal
	// order) — flatten uses the standard reconcile pattern for every field.
	state := &SelfServiceModel{
		DisplayNotifications: types.BoolValue(false),
		NotificationLocation: types.StringValue(""),
		NotificationSubject:  types.StringValue(""),
	}
	flattenSelfService(&proclassic.OsXConfigurationProfileSelfService{
		Notification: &proclassic.NotificationValue{
			Enabled: new(true),
			Method:  new(notificationLocationSelfServiceAndCenter),
		},
		NotificationSubject: new("Subj"),
		NotificationMessage: new("Body"),
	}, state, false)
	if !state.DisplayNotifications.ValueBool() {
		t.Fatal("DisplayNotifications: expected true (initial assignment when state was null)")
	}
	if state.NotificationLocation.ValueString() != notificationLocationSelfServiceAndCenter {
		t.Fatalf("NotificationLocation: got %q", state.NotificationLocation.ValueString())
	}
	if state.NotificationSubject.ValueString() != "Subj" {
		t.Fatalf("NotificationSubject: got %v", state.NotificationSubject)
	}
}

// TestFlattenSelfService_DisplayNotificationsReconciledFromWire — the SDK
// now emits Method-before-Enabled so the Classic API preserves the bool.
// State reconciles from the server value when the user has it configured.
func TestFlattenSelfService_DisplayNotificationsReconciledFromWire(t *testing.T) {
	t.Parallel()
	state := &SelfServiceModel{
		DisplayNotifications: types.BoolValue(false), // pre-configured to allow reconcile substitution
	}
	flattenSelfService(&proclassic.OsXConfigurationProfileSelfService{
		Notification: &proclassic.NotificationValue{
			Enabled: new(true),
		},
	}, state, false)
	if !state.DisplayNotifications.ValueBool() {
		t.Fatalf("expected DisplayNotifications to reconcile to true from server, got %v", state.DisplayNotifications)
	}
}

func TestFlattenSelfService_SecurityRemovalDisallowed(t *testing.T) {
	t.Parallel()
	state := &SelfServiceModel{RemovalDisallowed: types.StringValue("")}
	flattenSelfService(&proclassic.OsXConfigurationProfileSelfService{
		Security: &proclassic.OsXConfigurationProfileSelfServiceSecurity{
			RemovalDisallowed: new(removalDisallowedNever),
		},
	}, state, false)
	if state.RemovalDisallowed.ValueString() != removalDisallowedNever {
		t.Fatalf("RemovalDisallowed: got %q", state.RemovalDisallowed.ValueString())
	}
}

// TestFlattenSelfService_Categories pins the ownership gate on
// self_service.categories, which is what issue #392 was about: the hydration
// used to test only what the wire carried, so dropping categories from a
// self_service block you otherwise keep failed the apply — the plan said null
// and Read handed back a populated list.
//
// Optional-only, so a nil slice means unmanaged and an empty non-nil one means
// the configuration declares `[]`.
func TestFlattenSelfService_Categories(t *testing.T) {
	t.Parallel()

	wire := func() *proclassic.OsXConfigurationProfileSelfService {
		return &proclassic.OsXConfigurationProfileSelfService{
			SelfServiceCategories: &proclassic.OsXConfigurationProfileSelfServiceSelfServiceCategories{
				Category: &[]proclassic.OsXConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem{
					{ID: new(64), Name: new("All Desktops"), DisplayIn: new(true), FeatureIn: new(false)},
					{ID: new(46), Name: new("All Laptops"), DisplayIn: new(true), FeatureIn: new(true)},
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
		if state.Categories[0].ID.ValueString() != "64" || state.Categories[1].ID.ValueString() != "46" {
			t.Fatalf("category IDs: got %v / %v", state.Categories[0].ID, state.Categories[1].ID)
		}
		if !state.Categories[1].FeatureIn.ValueBool() {
			t.Fatal("category[1].FeatureIn expected true")
		}
	})

	t.Run("managed and the wire is empty: converges on the declared empty list", func(t *testing.T) {
		t.Parallel()
		state := &SelfServiceModel{Categories: []SelfServiceCategoryItem{}}
		flattenSelfService(&proclassic.OsXConfigurationProfileSelfService{}, state, false)
		if state.Categories == nil {
			t.Fatal("a declared empty list must stay non-nil, or `categories = []` never converges")
		}
		if len(state.Categories) != 0 {
			t.Fatalf("expected 0 categories, got %d", len(state.Categories))
		}
	})

	t.Run("unmanaged on an ordinary refresh: left alone", func(t *testing.T) {
		t.Parallel()
		// The #392 regression. Hydrating here is what made dropping the
		// attribute fail "inconsistent result after apply".
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
		flattenSelfService(&proclassic.OsXConfigurationProfileSelfService{}, state, true)
		if state.Categories != nil {
			t.Fatal("import must not fabricate an empty list the practitioner never wrote")
		}
	})
}

func TestAssignResourceModel_TopLevelIDFromGeneral(t *testing.T) {
	t.Parallel()
	state := &ResourceModel{}
	diags := assignResourceModel(context.Background(), state, &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{ID: new(99), Name: new("X")},
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
	diags := assignResourceModel(context.Background(), state, &proclassic.OsXConfigurationProfile{
		ID:      new(1),
		General: &proclassic.OsXConfigurationProfileGeneral{Name: new("X")},
		Scope: &proclassic.OsXConfigurationProfileScope{
			AllComputers: new(true),
		},
		SelfService: &proclassic.OsXConfigurationProfileSelfService{
			InstallButtonText: new("Install"),
		},
	}, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.Scope != nil {
		t.Fatalf("expected Scope unchanged (state did not declare it); got %+v", state.Scope)
	}
	if state.SelfService != nil {
		t.Fatalf("expected SelfService unchanged; got %+v", state.SelfService)
	}
}

func TestAssignResourceModel_PopulatedSubBlocksRefreshed(t *testing.T) {
	t.Parallel()
	// Pre-populate with configured values so the reconcile helpers
	// substitute server-returned values.
	state := &ResourceModel{
		Scope:       &scope.ComputerScopeModel{Targets: &scope.ComputerScopeTargetsModel{AllComputers: types.BoolValue(false)}},
		SelfService: &SelfServiceModel{InstallButtonText: types.StringValue("")},
	}
	diags := assignResourceModel(context.Background(), state, &proclassic.OsXConfigurationProfile{
		ID:      new(2),
		General: &proclassic.OsXConfigurationProfileGeneral{Name: new("X")},
		Scope:   &proclassic.OsXConfigurationProfileScope{AllComputers: new(true)},
		SelfService: &proclassic.OsXConfigurationProfileSelfService{
			InstallButtonText: new("Install"),
		},
	}, false)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.Scope.Targets.AllComputers.ValueBool() {
		t.Fatal("Scope.AllComputers not refreshed")
	}
	if state.SelfService.InstallButtonText.ValueString() != "Install" {
		t.Fatalf("SelfService.InstallButtonText: got %q", state.SelfService.InstallButtonText.ValueString())
	}
}

// TestAssignResourceModel_IncludeUnmanagedHydratesFromScratch pins the
// config-generation contract: with includeUnmanaged set and an empty starting
// model, every wire-present optional section is allocated and hydrated.
func TestAssignResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	t.Parallel()
	state := &ResourceModel{}
	diags := assignResourceModel(context.Background(), state, &proclassic.OsXConfigurationProfile{
		ID:      new(7),
		General: &proclassic.OsXConfigurationProfileGeneral{Name: new("X")},
		Scope: &proclassic.OsXConfigurationProfileScope{
			AllComputers: new(true),
			Computers: &proclassic.OsXConfigurationProfileScopeComputers{
				Computer: &[]proclassic.OsXConfigurationProfileScopeComputersComputerItem{{ID: new(5)}},
			},
			Exclusions: &proclassic.OsXConfigurationProfileScopeExclusions{},
		},
		SelfService: &proclassic.OsXConfigurationProfileSelfService{
			InstallButtonText: new("Install"),
		},
	}, true)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.Scope == nil || state.Scope.Targets == nil {
		t.Fatalf("expected Scope.Targets hydrated from scratch; got %+v", state.Scope)
	}
	if !state.Scope.Targets.AllComputers.ValueBool() {
		t.Fatal("expected all_computers hydrated true")
	}
	if state.Scope.Exclusions == nil {
		t.Fatal("expected Exclusions allocated when wire-present")
	}
	if state.Scope.Limitations != nil {
		t.Fatalf("expected Limitations to stay nil when wire-absent; got %+v", state.Scope.Limitations)
	}
	if state.SelfService == nil || state.SelfService.InstallButtonText.ValueString() != "Install" {
		t.Fatalf("expected SelfService hydrated; got %+v", state.SelfService)
	}
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}
