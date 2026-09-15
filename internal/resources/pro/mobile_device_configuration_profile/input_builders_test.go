// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

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
		Name:  types.StringValue("tf-test"),
		Level: types.StringValue(levelUIDevice),
	}, "")
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if g.Level == nil || *g.Level != levelWireWriteDevice {
		t.Fatalf("expected wire-write level %q, got %v", levelWireWriteDevice, g.Level)
	}
	if g.Name == nil || *g.Name != "tf-test" {
		t.Fatalf("expected name copied verbatim, got %v", g.Name)
	}
}

func TestBuildGeneral_LevelUserTranslated(t *testing.T) {
	t.Parallel()
	g, _, _ := buildGeneral(&GeneralModel{
		Name:  types.StringValue("x"),
		Level: types.StringValue(levelUIUser),
	}, "")
	if g.Level == nil || *g.Level != levelWireWriteUser {
		t.Fatalf("expected wire-write level %q, got %v", levelWireWriteUser, g.Level)
	}
}

func TestBuildGeneral_DistributionMethodMapsToDeploymentMethod(t *testing.T) {
	t.Parallel()
	g, _, _ := buildGeneral(&GeneralModel{
		Name:               types.StringValue("x"),
		DistributionMethod: types.StringValue(distributionMethodMakeAvailableInSS),
	}, "")
	if g.DeploymentMethod == nil || *g.DeploymentMethod != distributionMethodMakeAvailableInSS {
		t.Fatalf("DeploymentMethod round-trip: got %v", g.DeploymentMethod)
	}
}

func TestBuildGeneral_CategoryAndSiteFromStringID(t *testing.T) {
	t.Parallel()
	g, _, _ := buildGeneral(&GeneralModel{
		Name:       types.StringValue("x"),
		CategoryID: types.StringValue("58"),
		SiteID:     types.StringValue("7"),
	}, "")
	if g.Category == nil || g.Category.ID == nil || *g.Category.ID != 58 {
		t.Fatalf("category id: got %v", g.Category)
	}
	if g.Site == nil || g.Site.ID == nil || *g.Site.ID != 7 {
		t.Fatalf("site id: got %v", g.Site)
	}
}

func TestBuildGeneral_RedeployDaysBeforeCertificateExpires(t *testing.T) {
	t.Parallel()
	g, _, _ := buildGeneral(&GeneralModel{
		Name:                                 types.StringValue("x"),
		RedeployDaysBeforeCertificateExpires: types.Int64Value(14),
	}, "")
	if g.RedeployDaysBeforeCertificateExpires == nil || *g.RedeployDaysBeforeCertificateExpires != 14 {
		t.Fatalf("RedeployDays: got %v", g.RedeployDaysBeforeCertificateExpires)
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
		t.Fatalf("expected server-canonical UUID substituted; got %s", string(*g.Payloads))
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
	s, diags := buildScope(context.Background(), &scope.MobileScopeModel{})
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if s != nil {
		t.Fatalf("expected nil scope when no children set, got %+v", s)
	}
}

func TestBuildScope_AllMobileDevicesOnly(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.MobileScopeModel{
		Targets: &scope.MobileScopeTargetsModel{
			AllMobileDevices: types.BoolValue(true),
		},
	})
	if s == nil || s.AllMobileDevices == nil || !*s.AllMobileDevices {
		t.Fatalf("expected all_mobile_devices=true, got %+v", s)
	}
	if s.MobileDevices != nil || s.MobileDeviceGroups != nil {
		t.Fatal("expected target sub-blocks omitted when only all_mobile_devices set")
	}
}

func TestBuildScope_MobileDeviceIDsPopulated(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.MobileScopeModel{
		Targets: &scope.MobileScopeTargetsModel{
			MobileDeviceIDs: stringSet(t, "11", "22"),
		},
	})
	if s == nil || s.MobileDevices == nil || len(*s.MobileDevices.MobileDevice) != 2 {
		t.Fatalf("expected 2 mobile devices, got %+v", s)
	}
}

func TestBuildScope_LimitationsPopulated(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.MobileScopeModel{
		Limitations: &scope.MobileScopeLimitationsModel{
			NetworkSegmentIDs:                stringSet(t, "5"),
			DirectoryServiceOrLocalUserNames: stringSet(t, "alice", "bob"),
		},
	})
	if s == nil || s.Limitations == nil {
		t.Fatalf("expected limitations populated, got %+v", s)
	}
	if s.Limitations.NetworkSegments == nil || len(*s.Limitations.NetworkSegments.NetworkSegment) != 1 {
		t.Fatalf("expected 1 network segment, got %+v", s.Limitations.NetworkSegments)
	}
	if s.Limitations.Users == nil || len(*s.Limitations.Users.User) != 2 {
		t.Fatalf("expected 2 DS users, got %+v", s.Limitations.Users)
	}
}

func TestBuildScope_ExclusionsPopulated(t *testing.T) {
	t.Parallel()
	s, _ := buildScope(context.Background(), &scope.MobileScopeModel{
		Targets: &scope.MobileScopeTargetsModel{
			AllMobileDevices: types.BoolValue(true),
		},
		Exclusions: &scope.MobileScopeExclusionsModel{
			MobileDeviceIDs:                stringSet(t, "99"),
			DirectoryServiceUserGroupNames: stringSet(t, "DS-Group"),
		},
	})
	if s == nil || s.Exclusions == nil {
		t.Fatal("expected exclusions populated")
	}
	if s.Exclusions.MobileDevices == nil || len(*s.Exclusions.MobileDevices.MobileDevice) != 1 {
		t.Fatalf("expected 1 excluded mobile device, got %+v", s.Exclusions.MobileDevices)
	}
	if s.Exclusions.UserGroups == nil || len(*s.Exclusions.UserGroups.UserGroup) != 1 {
		t.Fatalf("expected 1 DS user group exclusion, got %+v", s.Exclusions.UserGroups)
	}
}

func TestBuildSelfService_SecurityBlock(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		RemovalDisallowed:     types.StringValue(removalDisallowedWithAuthorization),
		AuthorizationPassword: types.StringValue("pass123"),
	})
	if ss == nil || ss.Security == nil {
		t.Fatal("expected Security populated")
	}
	if ss.Security.RemovalDisallowed == nil || *ss.Security.RemovalDisallowed != removalDisallowedWithAuthorization {
		t.Fatalf("RemovalDisallowed: got %v", ss.Security.RemovalDisallowed)
	}
	if ss.Security.Password == nil || *ss.Security.Password != "pass123" {
		t.Fatalf("Password: got %v", ss.Security.Password)
	}
}

func TestBuildSelfService_SecurityBlock_PasswordOnly(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		AuthorizationPassword: types.StringValue("pass123"),
	})
	if ss.Security == nil || ss.Security.Password == nil {
		t.Fatal("expected Security.Password when only password set")
	}
	if ss.Security.RemovalDisallowed != nil {
		t.Fatal("expected RemovalDisallowed absent when not set")
	}
}

func TestBuildSelfService_SecurityBlock_NeitherSet_Nil(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{})
	if ss.Security != nil {
		t.Fatalf("expected nil Security when neither removal_disallowed nor authorization_password set, got %+v", ss.Security)
	}
}

func TestBuildSelfService_CategoriesAsList(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		Categories: []SelfServiceCategoryItem{
			{ID: types.StringValue("58")},
			{ID: types.StringValue("44")},
		},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil {
		t.Fatal("expected SelfServiceCategories populated")
	}
	cats := *ss.SelfServiceCategories.Category
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
	if cats[0].ID == nil || *cats[0].ID != 58 {
		t.Fatalf("category[0].ID: got %v", cats[0].ID)
	}
}

// TestBuildSelfService_CategoryIDOnlySendsDisplayIn guards the config shape
// that silently stored nothing. This resource surfaces no display_in attribute,
// so the write must supply it unconditionally. See
// helpers.SelfServiceCategoryDisplayIn.
func TestBuildSelfService_CategoryIDOnlySendsDisplayIn(t *testing.T) {
	t.Parallel()
	ss, _ := buildSelfService(&SelfServiceModel{
		Categories: []SelfServiceCategoryItem{{ID: types.StringValue("64")}},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil {
		t.Fatal("expected a category on the wire")
	}
	cats := *ss.SelfServiceCategories.Category
	if len(cats) != 1 || cats[0].DisplayIn == nil || !*cats[0].DisplayIn {
		t.Fatalf("display_in must be sent as true for an id-only category, got %+v", cats)
	}
}

func TestBuildInput_EndToEndCreatePath(t *testing.T) {
	t.Parallel()
	plan := ResourceModel{
		General: &GeneralModel{
			Name:               types.StringValue("E2E"),
			Description:        types.StringValue("desc"),
			Level:              types.StringValue(levelUIDevice),
			DistributionMethod: types.StringValue(distributionMethodInstallAutomatically),
		},
		Scope: &scope.MobileScopeModel{
			Targets: &scope.MobileScopeTargetsModel{
				AllMobileDevices: types.BoolValue(true),
			},
		},
	}
	out, diags := buildInput(context.Background(), plan, "")
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if out.General == nil || out.Scope == nil {
		t.Fatalf("expected General + Scope, got %+v", out)
	}
	if out.General.Level == nil || *out.General.Level != levelWireWriteDevice {
		t.Fatalf("Level wire-write: got %v want %q", out.General.Level, levelWireWriteDevice)
	}
}

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

func TestBuildScopeExclusions_NetworkSegmentsCarryDedicatedItemType(t *testing.T) {
	t.Parallel()
	e, _ := buildScopeExclusions(context.Background(), &scope.MobileScopeExclusionsModel{
		NetworkSegmentIDs: stringSet(t, "5"),
	})
	if e == nil || e.NetworkSegments == nil || len(*e.NetworkSegments.NetworkSegment) != 1 {
		t.Fatalf("expected network_segments populated, got %+v", e)
	}
	_ = proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem((*e.NetworkSegments.NetworkSegment)[0])
}
