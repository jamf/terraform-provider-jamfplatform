// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func newList(t *testing.T, in []string) types.List {
	t.Helper()
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	l, diags := types.ListValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatalf("list construction: %v", diags)
	}
	return l
}

func TestStringOrSentinel(t *testing.T) {
	if got := stringOrSentinel(types.StringNull(), "-1"); got != "-1" {
		t.Errorf("null -> sentinel: got %q", got)
	}
	if got := stringOrSentinel(types.StringUnknown(), "-1"); got != "-1" {
		t.Errorf("unknown -> sentinel: got %q", got)
	}
	if got := stringOrSentinel(types.StringValue(""), "-1"); got != "-1" {
		t.Errorf("empty -> sentinel: got %q", got)
	}
	if got := stringOrSentinel(types.StringValue("42"), "-1"); got != "42" {
		t.Errorf("value pass-through: got %q", got)
	}
}

func TestBuildSkipSetupItemsMap(t *testing.T) {
	if buildSkipSetupItemsMap(nil) != nil {
		t.Errorf("nil input must return nil map (so SDK omits the field)")
	}

	plan := &SkipSetupItemsModel{
		Biometric:          types.BoolValue(true),
		FileVault:          types.BoolValue(true),
		ICloudDiagnostics:  types.BoolValue(true),
		EnableLockdownMode: types.BoolValue(false),
		AppleID:            types.BoolValue(true),
		TOS:                types.BoolValue(false),
	}
	out := buildSkipSetupItemsMap(plan)
	if out == nil {
		t.Fatalf("expected non-nil map")
	}
	wire := *out
	cases := map[string]bool{
		"Biometric":          true,
		"FileVault":          true,
		"iCloudDiagnostics":  true, // wire-side lowercase 'i'
		"EnableLockdownMode": false,
		"AppleID":            true,
		"TOS":                false,
	}
	for k, want := range cases {
		if got, ok := wire[k]; !ok || got != want {
			t.Errorf("wire key %q: want %v, got (ok=%v, val=%v)", k, want, ok, got)
		}
	}
	// All 25 wire keys must be present even when zero-valued in the model.
	if len(wire) != 25 {
		t.Errorf("map should always carry all 25 wire keys; got %d", len(wire))
	}
}

func TestBuildLocationInformation_FreshAndPopulated(t *testing.T) {
	fresh := buildLocationInformation(nil, "-1", 0)
	if fresh.ID != "-1" || fresh.VersionLock != 0 {
		t.Errorf("fresh: id/lock zero-value expected, got %+v", fresh)
	}
	if fresh.BuildingID != "-1" || fresh.DepartmentID != "-1" {
		t.Errorf("fresh: building/department sentinels expected, got %+v", fresh)
	}

	plan := &LocationInformationModel{
		Username:     types.StringValue("alice"),
		Realname:     types.StringValue("Alice Example"),
		BuildingID:   types.StringValue("3"),
		DepartmentID: types.StringValue("7"),
	}
	pop := buildLocationInformation(plan, "99", 4)
	if pop.ID != "99" || pop.VersionLock != 4 {
		t.Errorf("populated: id/lock not passed through")
	}
	if pop.Username != "alice" || pop.Realname != "Alice Example" {
		t.Errorf("populated: scalar fields not copied: %+v", pop)
	}
	if pop.BuildingID != "3" || pop.DepartmentID != "7" {
		t.Errorf("populated: building/department user values not preserved: %+v", pop)
	}
}

func TestBuildPurchasingInformation_DateDefaults(t *testing.T) {
	fresh := buildPurchasingInformation(nil, "-1", 0)
	if fresh.LeaseDate != "1970-01-01" || fresh.PoDate != "1970-01-01" || fresh.WarrantyDate != "1970-01-01" {
		t.Errorf("fresh: date sentinels missing, got %+v", fresh)
	}
	if !fresh.Purchased {
		t.Errorf("fresh: purchased should default true")
	}

	plan := &PurchasingInformationModel{
		Leased:         types.BoolValue(true),
		Purchased:      types.BoolValue(false),
		LeaseDate:      types.StringValue("2025-01-01"),
		LifeExpectancy: types.Int64Value(5),
	}
	pop := buildPurchasingInformation(plan, "101", 3)
	if pop.ID != "101" || pop.VersionLock != 3 {
		t.Errorf("populated: id/lock not passed through")
	}
	if !pop.Leased || pop.Purchased {
		t.Errorf("populated: leased/purchased not copied: %+v", pop)
	}
	if pop.LifeExpectancy != 5 || pop.LeaseDate != "2025-01-01" {
		t.Errorf("populated: scalar fields not copied: %+v", pop)
	}
}

func TestBuildAccountSettingsRequest_RotateGate(t *testing.T) {
	plan := &AccountSettingsModel{
		PayloadConfigured:        types.BoolValue(true),
		LocalAdminAccountEnabled: types.BoolValue(true),
		AdminUsername:            types.StringValue("ladmin"),
	}
	cfgWithPwd := &AccountSettingsModel{
		AdminPassword: types.StringValue("Sup3r"),
	}

	// rotate=false: password must NOT be sent even when cfg has it.
	out := buildAccountSettingsRequest(plan, nil, cfgWithPwd, false)
	if out == nil {
		t.Fatalf("expected non-nil request")
	}
	if out.AdminPassword != nil {
		t.Errorf("rotate=false: password must be nil")
	}
	if out.AdminUsername == nil || *out.AdminUsername != "ladmin" {
		t.Errorf("admin_username must be copied")
	}

	// rotate=true: password IS sent.
	out = buildAccountSettingsRequest(plan, nil, cfgWithPwd, true)
	if out.AdminPassword == nil || *out.AdminPassword != "Sup3r" {
		t.Errorf("rotate=true: password must be sent, got %v", out.AdminPassword)
	}

	// nil plan + nil GET ⇒ nil out.
	if buildAccountSettingsRequest(nil, nil, nil, false) != nil {
		t.Errorf("nil/nil should produce nil")
	}
}

func TestPlaintextRecoveryPassword(t *testing.T) {
	if plaintextRecoveryPassword(ComputerPrestageEnrollmentResourceModel{}) != nil {
		t.Errorf("null pwd → nil")
	}
	cfg := ComputerPrestageEnrollmentResourceModel{
		RecoveryLockPassword: types.StringValue(""),
	}
	if plaintextRecoveryPassword(cfg) != nil {
		t.Errorf("empty pwd → nil")
	}
	cfg.RecoveryLockPassword = types.StringValue("RecP@ss")
	if got := plaintextRecoveryPassword(cfg); got == nil || *got != "RecP@ss" {
		t.Errorf("populated pwd should round-trip: %v", got)
	}
}

func TestStringSetToSlice_NullUnknown(t *testing.T) {
	if got, d := stringSetToSlice(context.Background(), types.SetNull(types.StringType)); got != nil || d.HasError() {
		t.Errorf("null set should return nil slice, got %v / %v", got, d)
	}
	if got, d := stringSetToSlice(context.Background(), types.SetUnknown(types.StringType)); got != nil || d.HasError() {
		t.Errorf("unknown set should return nil slice, got %v / %v", got, d)
	}
}

func TestStringListToSlice_NullUnknown(t *testing.T) {
	if got, d := stringListToSlice(context.Background(), types.ListNull(types.StringType)); got != nil || d.HasError() {
		t.Errorf("null list should return nil slice")
	}
	if got, d := stringListToSlice(context.Background(), newList(t, []string{"a", "b"})); len(got) != 2 || d.HasError() {
		t.Errorf("populated list extraction failed: %v / %v", got, d)
	}
}

func TestBuildPostInput_Smoke(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		DisplayName:                       types.StringValue("smoke"),
		DeviceEnrollmentProgramInstanceID: types.StringValue("1"),
		EnrollmentSiteID:                  types.StringValue("-1"),
		CustomPackageDistributionPointID:  types.StringValue("-1"),
	}
	post, d := buildPostInput(context.Background(), plan, plan)
	if d.HasError() {
		t.Fatalf("post build diags: %v", d)
	}
	if post.DisplayName != "smoke" {
		t.Errorf("display_name not copied")
	}
	if post.LocationInformation.ID != sentinelNestedIDForCreate {
		t.Errorf("nested location id must be the create sentinel %q on POST, got %q", sentinelNestedIDForCreate, post.LocationInformation.ID)
	}
	if post.PurchasingInformation.ID != sentinelNestedIDForCreate {
		t.Errorf("nested purchasing id must be the create sentinel on POST, got %q", post.PurchasingInformation.ID)
	}
}

func TestBuildPutInput_AccountSettingsAlwaysIncluded(t *testing.T) {
	plan := ComputerPrestageEnrollmentResourceModel{
		DisplayName:                       types.StringValue("smoke"),
		DeviceEnrollmentProgramInstanceID: types.StringValue("1"),
		AccountSettings:                   nil, // user omitted the block
	}
	got := &pro.GetComputerPrestageV3{
		AccountSettings: &pro.AccountSettingsResponse{ID: "87", VersionLock: 0},
	}

	put, d := buildPutInput(context.Background(), plan, plan, got, false, false)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	// Per spike F6: PUT body MUST always carry accountSettings. The
	// builder defaults to an empty struct when the user-side plan is nil.
	if put.AccountSettings == nil {
		t.Errorf("accountSettings must be non-nil on PUT even when plan omits it")
	}
}
