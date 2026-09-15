// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func stringSet(t *testing.T, values ...string) types.Set {
	t.Helper()
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	s, d := types.SetValue(types.StringType, elems)
	if d.HasError() {
		t.Fatalf("SetValue: %v", d)
	}
	return s
}

func TestBuildGeneral_LevelTranslatedToWireWrite(t *testing.T) {
	t.Parallel()
	g, _, diags := buildGeneral(&GeneralModel{
		Name:  types.StringValue("tf-acc-test"),
		Level: types.StringValue(levelUIComputer),
	}, "")
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if g.Level == nil || *g.Level != levelWireWriteCC {
		t.Fatalf("expected wire-write level %q, got %v", levelWireWriteCC, g.Level)
	}
	if g.Name == nil || *g.Name != "tf-acc-test" {
		t.Fatalf("expected name copied through verbatim, got %v", g.Name)
	}
}

func TestBuildGeneral_DistributionMethodVerbatim(t *testing.T) {
	t.Parallel()
	g, _, _ := buildGeneral(&GeneralModel{
		Name:               types.StringValue("x"),
		DistributionMethod: types.StringValue(distributionMethodMakeAvailableInSS),
	}, "")
	if g.DistributionMethod == nil || *g.DistributionMethod != distributionMethodMakeAvailableInSS {
		t.Fatalf("distribution_method round-trip: got %v", g.DistributionMethod)
	}
}

func TestBuildGeneral_CategoryAndSiteFromStringID(t *testing.T) {
	t.Parallel()
	g, _, _ := buildGeneral(&GeneralModel{
		Name:       types.StringValue("x"),
		CategoryID: types.StringValue("42"),
		SiteID:     types.StringValue("7"),
	}, "")
	if g.Category == nil || g.Category.ID == nil || *g.Category.ID != 42 {
		t.Fatalf("category id: got %v", g.Category)
	}
	if g.Site == nil || g.Site.ID == nil || *g.Site.ID != 7 {
		t.Fatalf("site id: got %v", g.Site)
	}
}

func TestBuildGeneral_PayloadIdentifierInjectionOnUpdate(t *testing.T) {
	t.Parallel()
	const existingUUID = "dd063a98-1372-469c-a92f-5b74e6982631"
	const newPayload = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadIdentifier</key><string>NEW-identifier</string>
<key>PayloadUUID</key><string>NEW-UUID</string>
<key>PayloadVersion</key><integer>1</integer>
</dict></plist>`
	g, prepared, diags := buildGeneral(&GeneralModel{
		Name:     types.StringValue("x"),
		Payloads: types.StringValue(newPayload),
	}, existingUUID)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if g.Payloads == nil {
		t.Fatal("expected Payloads to be set")
	}
	if !strings.Contains(string(*g.Payloads), existingUUID) {
		t.Fatalf("expected server-canonical UUID substituted into both top-level fields; got %s", string(*g.Payloads))
	}
	if strings.Contains(string(*g.Payloads), "NEW-identifier") || strings.Contains(string(*g.Payloads), "NEW-UUID") {
		t.Fatalf("expected user-supplied identifiers replaced; got %s", string(*g.Payloads))
	}
	if len(prepared) == 0 {
		t.Fatal("expected prepared bytes returned")
	}
}

func TestBuildGeneral_PayloadIdentifierInjection_CreatePathNoOp(t *testing.T) {
	t.Parallel()
	const newPayload = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadIdentifier</key><string>CREATE-id</string>
<key>PayloadUUID</key><string>CREATE-uuid</string>
<key>PayloadVersion</key><integer>1</integer>
</dict></plist>`
	g, _, _ := buildGeneral(&GeneralModel{
		Name:     types.StringValue("x"),
		Payloads: types.StringValue(newPayload),
	}, "")
	if g.Payloads == nil || !strings.Contains(string(*g.Payloads), "CREATE-id") {
		t.Fatal("expected create-path payload untouched")
	}
}

func TestBuildScope_NullCollapsesToNil(t *testing.T) {
	t.Parallel()
	s, diags := buildScope(context.Background(), &scope.ComputerScopeModel{})
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if s != nil {
		t.Fatalf("expected nil scope when no children set, got %+v", s)
	}
}

func TestBuildScope_AllComputersOnly(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.ComputerScopeModel{
		Targets: &scope.ComputerScopeTargetsModel{
			AllComputers: types.BoolValue(true),
		},
	})
	if s == nil || s.AllComputers == nil || !*s.AllComputers {
		t.Fatalf("expected all_computers=true, got %+v", s)
	}
	if s.Computers != nil || s.ComputerGroups != nil {
		t.Fatal("expected target sub-blocks omitted")
	}
}

func TestBuildScope_ComputerIDsPopulated(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.ComputerScopeModel{
		Targets: &scope.ComputerScopeTargetsModel{
			ComputerIDs: stringSet(t, "11", "22"),
		},
	})
	if s == nil || s.Computers == nil || s.Computers.Computer == nil || len(*s.Computers.Computer) != 2 {
		t.Fatalf("expected 2 computers, got %+v", s)
	}
}

func TestBuildScope_LimitationsPopulated(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.ComputerScopeModel{
		Limitations: &scope.ComputerScopeLimitationsModel{
			NetworkSegmentIDs:                stringSet(t, "5"),
			DirectoryServiceOrLocalUserNames: stringSet(t, "alice", "bob"),
		},
	})
	if s == nil || s.Limitations == nil {
		t.Fatalf("expected limitations populated, got %+v", s)
	}
	if s.Limitations.NetworkSegments == nil || s.Limitations.NetworkSegments.NetworkSegment == nil || len(*s.Limitations.NetworkSegments.NetworkSegment) != 1 {
		t.Fatalf("expected 1 network segment limitation, got %+v", s.Limitations.NetworkSegments)
	}
	if s.Limitations.Users == nil || s.Limitations.Users.User == nil || len(*s.Limitations.Users.User) != 2 {
		t.Fatalf("expected 2 DS user names, got %+v", s.Limitations.Users)
	}
}

func TestBuildScope_ExclusionsPopulated(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.ComputerScopeModel{
		Targets: &scope.ComputerScopeTargetsModel{
			AllComputers: types.BoolValue(true),
		},
		Exclusions: &scope.ComputerScopeExclusionsModel{
			ComputerIDs:                    stringSet(t, "99"),
			DirectoryServiceUserGroupNames: stringSet(t, "DS-Group"),
		},
	})
	if s == nil || s.Exclusions == nil {
		t.Fatalf("expected exclusions populated")
	}
	if s.Exclusions.Computers == nil || len(*s.Exclusions.Computers.Computer) != 1 {
		t.Fatalf("expected 1 excluded computer, got %+v", s.Exclusions.Computers)
	}
	if s.Exclusions.UserGroups == nil || s.Exclusions.UserGroups.UserGroup == nil || len(*s.Exclusions.UserGroups.UserGroup) != 1 {
		t.Fatalf("expected 1 DS user group exclusion, got %+v", s.Exclusions.UserGroups)
	}
}

// TestBuildSelfService_NotificationSplit pins that the two Terraform attributes
// still split into the one dual-valued wire element for a pairing Jamf Pro can
// store.
//
// It used to assert the pairing with notificationLocationSelfServiceAndCenter,
// which Jamf Pro cannot store at all: the flag and the location share one
// element and setting either resets the other, so that request stores
// "Self Service" and reports success. The unwritable pairing is refused at plan
// time now and omitted from the payload — see buildSelfServiceNotification and
// TestBuildSelfServiceNotification.
func TestBuildSelfService_NotificationSplit(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		DisplayNotifications: types.BoolValue(true),
		NotificationLocation: types.StringValue(notificationLocationSelfService),
		NotificationSubject:  types.StringValue("Subj"),
		NotificationMessage:  types.StringValue("Body"),
	})
	if ss == nil || ss.Notification == nil {
		t.Fatalf("expected NotificationValue populated")
	}
	if ss.Notification.Enabled == nil || !*ss.Notification.Enabled {
		t.Fatal("expected Enabled=true")
	}
	if ss.Notification.Method == nil || *ss.Notification.Method != notificationLocationSelfService {
		t.Fatalf("expected Method = %q, got %v", notificationLocationSelfService, ss.Notification.Method)
	}
	if ss.NotificationSubject == nil || *ss.NotificationSubject != "Subj" {
		t.Fatalf("expected NotificationSubject Subj, got %v", ss.NotificationSubject)
	}
}

func TestBuildSelfService_SecurityRemovalDisallowed(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		RemovalDisallowed: types.StringValue(removalDisallowedWithAuthorization),
	})
	if ss == nil || ss.Security == nil || ss.Security.RemovalDisallowed == nil {
		t.Fatalf("expected Security.RemovalDisallowed populated")
	}
	if *ss.Security.RemovalDisallowed != removalDisallowedWithAuthorization {
		t.Fatalf("unexpected RemovalDisallowed: %q", *ss.Security.RemovalDisallowed)
	}
}

func TestBuildSelfService_CategoriesAsList(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		Categories: []SelfServiceCategoryItem{
			{ID: types.StringValue("64"), DisplayIn: types.BoolValue(true), FeatureIn: types.BoolValue(false)},
			{ID: types.StringValue("46"), DisplayIn: types.BoolValue(true), FeatureIn: types.BoolValue(true)},
		},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil {
		t.Fatal("expected SelfServiceCategories populated")
	}
	cats := *ss.SelfServiceCategories.Category
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
	if cats[0].ID == nil || *cats[0].ID != 64 {
		t.Fatalf("category[0].ID: got %v", cats[0].ID)
	}
	if cats[1].FeatureIn == nil || !*cats[1].FeatureIn {
		t.Fatalf("category[1].FeatureIn: got %v", cats[1].FeatureIn)
	}
}

func TestBuildInput_EndToEndCreatePath(t *testing.T) {
	t.Parallel()
	plan := ResourceModel{
		General: &GeneralModel{
			Name:               types.StringValue("E2E"),
			Description:        types.StringValue("desc"),
			Level:              types.StringValue(levelUIComputer),
			DistributionMethod: types.StringValue(distributionMethodInstallAutomatically),
			UserRemovable:      types.BoolValue(false),
		},
		Scope: &scope.ComputerScopeModel{
			Targets: &scope.ComputerScopeTargetsModel{
				AllComputers: types.BoolValue(true),
			},
		},
	}
	out, diags := buildInput(context.Background(), plan, "")
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if out.General == nil || out.Scope == nil {
		t.Fatalf("expected General + Scope on output, got %+v", out)
	}
	if out.General.Level == nil || *out.General.Level != levelWireWriteCC {
		t.Fatalf("Level wire-write translation: got %v want %q", out.General.Level, levelWireWriteCC)
	}
}

// Sanity check that nil sub-blocks on the resource model produce nil
// sub-blocks on the wire — proves the omission semantics survive the
// resource-to-wire boundary.
func TestBuildInput_NilSubBlocksProduceNilWire(t *testing.T) {
	t.Parallel()
	plan := ResourceModel{
		General: &GeneralModel{Name: types.StringValue("x")},
	}
	out, _ := buildInput(context.Background(), plan, "")
	if out.Scope != nil {
		t.Fatalf("expected nil Scope, got %+v", out.Scope)
	}
	if out.SelfService != nil {
		t.Fatalf("expected nil SelfService, got %+v", out.SelfService)
	}
}

// Cross-check: when scope.exclusions has only ID fields, the buildScopeExclusions
// helper assembles the right slice container.
func TestBuildScopeExclusions_NetworkSegmentsCarryDedicatedItemType(t *testing.T) {
	t.Parallel()
	e, _ := buildScopeExclusions(context.Background(), &scope.ComputerScopeExclusionsModel{
		NetworkSegmentIDs: stringSet(t, "5"),
	})
	if e == nil || e.NetworkSegments == nil || e.NetworkSegments.NetworkSegment == nil {
		t.Fatalf("expected network_segments populated, got %+v", e)
	}
	items := *e.NetworkSegments.NetworkSegment
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	// Type assert to the resource's network segment item type to confirm the
	// builder uses the dedicated item struct (not bare IDName).
	_ = proclassic.OsXConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem(items[0])
}

// TestBuildSelfServiceNotification pins the projection of the two Terraform
// attributes onto the single <notification> wire element, including the pairing
// Jamf Pro cannot store. The wire evidence is on buildSelfServiceNotification.
func TestBuildSelfServiceNotification(t *testing.T) {
	t.Parallel()

	const center = "Self Service and Notification Center"

	for _, tc := range []struct {
		name       string
		display    types.Bool
		location   types.String
		wantNil    bool
		wantBool   *bool
		wantMethod *string
	}{
		{
			name: "neither set omits the element", display: types.BoolNull(), location: types.StringNull(),
			wantNil: true,
		},
		{
			name: "flag alone", display: types.BoolValue(true), location: types.StringNull(),
			wantBool: new(true),
		},
		{
			name: "flag false alone", display: types.BoolValue(false), location: types.StringNull(),
			wantBool: new(false),
		},
		{
			name: "location alone", display: types.BoolNull(), location: types.StringValue(center),
			wantMethod: new(center),
		},
		{
			name: "the reachable pairing is sent whole", display: types.BoolValue(true), location: types.StringValue("Self Service"),
			wantBool: new(true), wantMethod: new("Self Service"),
		},
		{
			// The whole point: emitting this would silently downgrade an
			// admin-UI-set profile to "Self Service" while reporting success,
			// so the element is withheld and the server keeps what it has.
			name:    "the unwritable pairing omits the element entirely",
			display: types.BoolValue(true), location: types.StringValue(center),
			wantNil: true,
		},
		{
			name:    "flag false with Notification Center is reachable, so it is sent",
			display: types.BoolValue(false), location: types.StringValue(center),
			wantBool: new(false), wantMethod: new(center),
		},
		{
			name: "unknown values are treated as unset", display: types.BoolUnknown(), location: types.StringUnknown(),
			wantNil: true,
		},
		{
			name: "an empty location string is treated as unset", display: types.BoolNull(), location: types.StringValue(""),
			wantNil: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildSelfServiceNotification(&SelfServiceModel{
				DisplayNotifications: tc.display,
				NotificationLocation: tc.location,
			})
			if tc.wantNil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("want a value, got nil")
			}
			switch {
			case tc.wantBool == nil && got.Enabled != nil:
				t.Errorf("Enabled: want nil, got %v", *got.Enabled)
			case tc.wantBool != nil && got.Enabled == nil:
				t.Errorf("Enabled: want %v, got nil", *tc.wantBool)
			case tc.wantBool != nil && *got.Enabled != *tc.wantBool:
				t.Errorf("Enabled: want %v, got %v", *tc.wantBool, *got.Enabled)
			}
			switch {
			case tc.wantMethod == nil && got.Method != nil:
				t.Errorf("Method: want nil, got %q", *got.Method)
			case tc.wantMethod != nil && got.Method == nil:
				t.Errorf("Method: want %q, got nil", *tc.wantMethod)
			case tc.wantMethod != nil && *got.Method != *tc.wantMethod:
				t.Errorf("Method: want %q, got %q", *tc.wantMethod, *got.Method)
			}
		})
	}
}

// TestBuildSelfService_CategoryIDOnlyStillSendsDisplayIn is the regression
// guard for the config shape that silently stored nothing.
func TestBuildSelfService_CategoryIDOnlyStillSendsDisplayIn(t *testing.T) {
	t.Parallel()

	ss, _ := buildSelfService(&SelfServiceModel{
		Categories: []SelfServiceCategoryItem{{ID: types.StringValue("64")}},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil {
		t.Fatal("expected a category on the wire")
	}
	cats := *ss.SelfServiceCategories.Category
	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if cats[0].DisplayIn == nil || !*cats[0].DisplayIn {
		t.Fatalf("display_in must be sent as true for an id-only category, got %v", cats[0].DisplayIn)
	}
}
