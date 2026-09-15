// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestFlattenSkipSetupItems_NilIn(t *testing.T) {
	if got := flattenSkipSetupItems(nil); got != nil {
		t.Errorf("nil wire map → nil model expected, got %+v", got)
	}
}

func TestFlattenSkipSetupItems_CompleteWireMap(t *testing.T) {
	wire := map[string]bool{
		"Biometric":                 true,
		"FileVault":                 true,
		"SoftwareUpdate":            false,
		"Diagnostics":               false,
		"Accessibility":             false,
		"Intelligence":              false,
		"ScreenTime":                false,
		"Siri":                      false,
		"Restore":                   false,
		"Privacy":                   false,
		"Registration":              false,
		"EnableLockdownMode":        false,
		"TermsOfAddress":            false,
		"iCloudDiagnostics":         true,
		"AppleID":                   true,
		"DisplayTone":               false,
		"Appearance":                false,
		"Payment":                   false,
		"TOS":                       false,
		"OSShowcase":                false,
		"Welcome":                   false,
		"Wallpaper":                 false,
		"iCloudStorage":             true,
		"AdditionalPrivacySettings": false,
		"Location":                  false,
	}
	m := flattenSkipSetupItems(wire)
	if m == nil {
		t.Fatalf("expected non-nil model")
	}
	if !m.Biometric.ValueBool() {
		t.Errorf("Biometric not copied")
	}
	if !m.ICloudDiagnostics.ValueBool() {
		t.Errorf("iCloudDiagnostics wire→ICloudDiagnostics not copied")
	}
	if !m.ICloudStorage.ValueBool() {
		t.Errorf("iCloudStorage wire→ICloudStorage not copied")
	}
	if !m.AppleID.ValueBool() {
		t.Errorf("AppleID not copied")
	}
}

func TestFlattenLocationInformation_NilIn(t *testing.T) {
	if got := flattenLocationInformation(nil); got != nil {
		t.Errorf("nil → nil expected")
	}
}

func TestFlattenLocationInformation_Populated(t *testing.T) {
	loc := &pro.LocationInformationV2{
		Username:     "alice",
		Realname:     "Alice E",
		BuildingID:   "3",
		DepartmentID: "-1",
	}
	m := flattenLocationInformation(loc)
	if m == nil || m.Username.ValueString() != "alice" || m.BuildingID.ValueString() != "3" {
		t.Errorf("flatten location: %+v", m)
	}
}

func TestFlattenAccountSettings_PreservesWoVersionFromPrior(t *testing.T) {
	got := &pro.AccountSettingsResponse{
		AdminUsername:     "ladmin",
		PrefillType:       "DEVICE_OWNER",
		UserAccountType:   "ADMINISTRATOR",
		PayloadConfigured: true,
	}
	prior := &AccountSettingsModel{
		AdminPasswordWoVersion: types.Int64Value(3),
	}
	m := flattenAccountSettings(got, prior)
	if m == nil {
		t.Fatalf("expected non-nil model")
	}
	if m.AdminPassword.ValueString() != "" || !m.AdminPassword.IsNull() {
		t.Errorf("admin_password must always be Null in state (WriteOnly)")
	}
	if m.AdminPasswordWoVersion.IsNull() || m.AdminPasswordWoVersion.ValueInt64() != 3 {
		t.Errorf("admin_password_wo_version must round-trip from prior state, got %v", m.AdminPasswordWoVersion)
	}
	if m.AdminUsername.ValueString() != "ladmin" {
		t.Errorf("admin_username not copied")
	}
}

// TestAssignGetToResource_RemovedBlocksStayNil reproduces the apply-time
// "was null, but now ObjectVal(...)" crash: an Update where the user removed
// the previously-managed nested blocks. The PLAN (target) has nil pointers
// while the PRIOR state still holds the populated blocks and the GET echoes
// them. The state-builder must honour the plan and leave each block nil, never
// repopulate from the prior state or the wire.
func TestAssignGetToResource_RemovedBlocksStayNil(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		ID: types.StringValue("384"),
		// All four nested blocks omitted from config → nil on the plan.
	}
	prior := ComputerPrestageEnrollmentResourceModel{
		ID:                    types.StringValue("384"),
		SkipSetupItems:        &SkipSetupItemsModel{},
		LocationInformation:   &LocationInformationModel{Username: types.StringValue("alice")},
		PurchasingInformation: &PurchasingInformationModel{Purchased: types.BoolValue(true)},
		AccountSettings:       &AccountSettingsModel{AdminUsername: types.StringValue("ladmin")},
	}
	got := &pro.GetComputerPrestageV3{
		ID:                    "384",
		SkipSetupItems:        map[string]bool{"Biometric": true},
		LocationInformation:   &pro.LocationInformationV2{Username: "alice", BuildingID: "-1", DepartmentID: "-1"},
		PurchasingInformation: &pro.PrestagePurchasingInformationV2{Purchased: true},
		AccountSettings:       &pro.AccountSettingsResponse{AdminUsername: "ladmin", PayloadConfigured: true},
	}

	if d := assignGetToResource(context.Background(), &plan, prior, got); d.HasError() {
		t.Fatalf("unexpected diags: %v", d)
	}

	if plan.SkipSetupItems != nil {
		t.Errorf("skip_setup_items must stay nil when removed from config, got %+v", plan.SkipSetupItems)
	}
	if plan.LocationInformation != nil {
		t.Errorf("location_information must stay nil when removed from config, got %+v", plan.LocationInformation)
	}
	if plan.PurchasingInformation != nil {
		t.Errorf("purchasing_information must stay nil when removed from config, got %+v", plan.PurchasingInformation)
	}
	if plan.AccountSettings != nil {
		t.Errorf("account_settings must stay nil when removed from config, got %+v", plan.AccountSettings)
	}
}

// TestAssignGetToResource_ManagedBlocksRefreshed is the companion: a block the
// plan still manages (non-nil pointer) must be (re)populated from the wire.
func TestAssignGetToResource_ManagedBlocksRefreshed(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		ID:                  types.StringValue("384"),
		LocationInformation: &LocationInformationModel{}, // managed (even if empty)
	}
	prior := ComputerPrestageEnrollmentResourceModel{ID: types.StringValue("384")}
	got := &pro.GetComputerPrestageV3{
		ID:                  "384",
		LocationInformation: &pro.LocationInformationV2{Username: "bob", BuildingID: "7", DepartmentID: "-1"},
	}

	if d := assignGetToResource(context.Background(), &plan, prior, got); d.HasError() {
		t.Fatalf("unexpected diags: %v", d)
	}
	if plan.LocationInformation == nil {
		t.Fatalf("location_information managed by plan must be populated from the wire")
	}
	if plan.LocationInformation.Username.ValueString() != "bob" || plan.LocationInformation.BuildingID.ValueString() != "7" {
		t.Errorf("managed block not refreshed from wire: %+v", plan.LocationInformation)
	}
	// Blocks the plan did not manage must remain nil.
	if plan.PurchasingInformation != nil || plan.AccountSettings != nil || plan.SkipSetupItems != nil {
		t.Errorf("unmanaged blocks must stay nil")
	}
}

func TestScopeSerialsToSet(t *testing.T) {
	// nil resp → empty set
	got := scopeSerialsToSet(nil)
	if got.IsNull() || got.IsUnknown() {
		t.Errorf("nil resp should yield concrete (empty) set, got %v", got)
	}
	if got.Elements() != nil && len(got.Elements()) != 0 {
		t.Errorf("nil resp set must be empty, got %v", got.Elements())
	}

	// populated assignments
	resp := &pro.PrestageScopeResponseV2{
		Assignments: []pro.PrestageScopeAssignmentV2{
			{SerialNumber: "AAA"},
			{SerialNumber: "BBB"},
		},
	}
	got = scopeSerialsToSet(resp)
	out, _ := stringSetToSlice(context.Background(), got)
	sort.Strings(out)
	if len(out) != 2 || out[0] != "AAA" || out[1] != "BBB" {
		t.Errorf("scope serials round-trip failed, got %v", out)
	}
}

func TestDiffPlanAgainstGet_Match(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		DisplayName: types.StringValue("x"),
		Mandatory:   types.BoolValue(true),
	}
	got := &pro.GetComputerPrestageV3{
		DisplayName: "x",
		Mandatory:   true,
	}
	if diff := diffPlanAgainstGet(context.Background(), plan, got); len(diff) != 0 {
		t.Errorf("matching plan/get should return no diffs, got %v", diff)
	}
}

func TestDiffPlanAgainstGet_RootMismatch(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		DisplayName:           types.StringValue("after"),
		Mandatory:             types.BoolValue(true),
		RequireAuthentication: types.BoolValue(true),
	}
	got := &pro.GetComputerPrestageV3{
		DisplayName:           "before", // mismatch
		Mandatory:             true,
		RequireAuthentication: false, // mismatch
	}
	diff := diffPlanAgainstGet(context.Background(), plan, got)
	if len(diff) != 2 {
		t.Fatalf("want 2 mismatches, got %d (%v)", len(diff), diff)
	}
	want := map[string]bool{"display_name": true, "require_authentication": true}
	for _, f := range diff {
		if !want[f] {
			t.Errorf("unexpected mismatch field %q", f)
		}
	}
}

func TestDiffPlanAgainstGet_NestedAccountSettingsMismatch(t *testing.T) {
	// Critical: the F4b workaround MUST detect nested-field rollbacks too.
	plan := ComputerPrestageEnrollmentResourceModel{
		DisplayName: types.StringValue("x"),
		AccountSettings: &AccountSettingsModel{
			AdminUsername: types.StringValue("rotated"),
		},
	}
	got := &pro.GetComputerPrestageV3{
		DisplayName: "x",
		AccountSettings: &pro.AccountSettingsResponse{
			AdminUsername: "original",
		},
	}
	diff := diffPlanAgainstGet(context.Background(), plan, got)
	if len(diff) == 0 {
		t.Fatalf("expected nested mismatch, got none")
	}
	found := false
	for _, f := range diff {
		if f == "account_settings.admin_username" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `account_settings.admin_username` in diff, got %v", diff)
	}
}

func TestDiffPlanAgainstGet_NestedSkipSetupItemsMismatch(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		DisplayName: types.StringValue("x"),
		SkipSetupItems: &SkipSetupItemsModel{
			ICloudDiagnostics: types.BoolValue(true),
		},
	}
	got := &pro.GetComputerPrestageV3{
		DisplayName: "x",
		SkipSetupItems: map[string]bool{
			"iCloudDiagnostics": false, // rolled back
		},
	}
	diff := diffPlanAgainstGet(context.Background(), plan, got)
	found := false
	for _, f := range diff {
		if f == "skip_setup_items.icloud_diagnostics" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `skip_setup_items.icloud_diagnostics` in diff, got %v", diff)
	}
}

func TestEqualStringSetsUnordered(t *testing.T) {
	if !equalStringSetsUnordered([]string{"a", "b"}, []string{"b", "a"}) {
		t.Errorf("unordered equal failed")
	}
	if equalStringSetsUnordered([]string{"a"}, []string{"b"}) {
		t.Errorf("different elements should differ")
	}
	if equalStringSetsUnordered([]string{"a"}, []string{"a", "b"}) {
		t.Errorf("different lengths should differ")
	}
}
