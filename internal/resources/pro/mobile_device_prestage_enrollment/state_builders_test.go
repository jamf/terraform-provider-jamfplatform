// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func newStringList(t *testing.T, in []string) types.List {
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

func TestFlattenSkipSetupItems_NilIn(t *testing.T) {
	if got := flattenSkipSetupItems(nil); got != nil {
		t.Errorf("nil wire map → nil model expected, got %+v", got)
	}
}

func TestFlattenSkipSetupItems_RoundTripAndMissingKey(t *testing.T) {
	// Set a handful true, omit one entirely (should flatten to false).
	wire := map[string]bool{
		"Biometric":           true,
		"AppleID":             true,
		"iMessageAndFaceTime": true,
		"EnableLockdownMode":  false,
		// "Zoom" deliberately omitted → must flatten to false.
	}
	m := flattenSkipSetupItems(wire)
	if m == nil {
		t.Fatalf("expected non-nil model")
	}
	if !m.Biometric.ValueBool() {
		t.Errorf("Biometric not copied")
	}
	if !m.AppleID.ValueBool() {
		t.Errorf("AppleID not copied")
	}
	if !m.IMessageAndFaceTime.ValueBool() {
		t.Errorf("iMessageAndFaceTime wire→IMessageAndFaceTime not copied")
	}
	if m.EnableLockdownMode.ValueBool() {
		t.Errorf("EnableLockdownMode should be false")
	}
	if m.Zoom.ValueBool() {
		t.Errorf("missing wire key Zoom must flatten to false, got true")
	}

	// All 45 model attrs must be concrete (non-null) BoolValues.
	if m.ActionButton.IsNull() || m.WatchMigration.IsNull() || m.TVRoom.IsNull() {
		t.Errorf("flattened model attrs must be concrete bools, not null")
	}
}

func TestFlattenNames_NilIn(t *testing.T) {
	if got := flattenNames(nil); got != nil {
		t.Errorf("nil → nil expected")
	}
}

func TestFlattenNames_PrestageDeviceNamesNilWhenServerEmpty(t *testing.T) {
	n := &pro.MobileDevicePrestageNamesV3{
		AssignNamesUsing: new("Serial Numbers"),
		ManageNames:      new(false),
		// PrestageDeviceNames nil — the server returned no entries.
	}
	m := flattenNames(n)
	if m == nil {
		t.Fatalf("expected non-nil model")
	}
	if m.AssignNamesUsing.ValueString() != "Serial Numbers" {
		t.Errorf("assign_names_using not copied: %v", m.AssignNamesUsing)
	}
	if m.PrestageDeviceNames != nil {
		t.Errorf("server-empty prestage_device_names must stay nil, got %v", m.PrestageDeviceNames)
	}
}

func TestFlattenNames_PrestageDeviceNamesPopulated(t *testing.T) {
	n := &pro.MobileDevicePrestageNamesV3{
		AssignNamesUsing: new("List of Names"),
		PrestageDeviceNames: &[]pro.MobileDevicePrestageNameV3{
			{DeviceName: new("iPad-1"), ID: new("42"), Used: new(true)},
		},
	}
	m := flattenNames(n)
	if m == nil || len(m.PrestageDeviceNames) != 1 {
		t.Fatalf("expected 1 prestage device name, got %+v", m)
	}
	el := m.PrestageDeviceNames[0]
	if el.DeviceName.ValueString() != "iPad-1" || el.ID.ValueString() != "42" || !el.Used.ValueBool() {
		t.Errorf("prestage device name not round-tripped: %+v", el)
	}
}

func TestFlattenLocationInformation_NilAndPopulated(t *testing.T) {
	if got := flattenLocationInformation(nil); got != nil {
		t.Errorf("nil → nil expected")
	}
	loc := &pro.LocationInformationV3{
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

func TestFlattenPurchasingInformation_NilAndPopulated(t *testing.T) {
	if got := flattenPurchasingInformation(nil); got != nil {
		t.Errorf("nil → nil expected")
	}
	pur := &pro.PrestagePurchasingInformationV3{
		Leased:         true,
		Purchased:      false,
		AppleCareID:    "AC-1",
		LifeExpectancy: 5,
		LeaseDate:      "2025-01-01",
	}
	m := flattenPurchasingInformation(pur)
	if m == nil {
		t.Fatalf("expected non-nil model")
	}
	if !m.Leased.ValueBool() || m.Purchased.ValueBool() {
		t.Errorf("leased/purchased not copied: %+v", m)
	}
	if m.AppleCareID.ValueString() != "AC-1" || m.LifeExpectancy.ValueInt64() != 5 || m.LeaseDate.ValueString() != "2025-01-01" {
		t.Errorf("scalar fields not copied: %+v", m)
	}
}

func TestScopeSerialsToSet(t *testing.T) {
	// nil resp → empty (concrete) set.
	got := scopeSerialsToSet(nil)
	if got.IsNull() || got.IsUnknown() {
		t.Errorf("nil resp should yield concrete (empty) set, got %v", got)
	}
	if len(got.Elements()) != 0 {
		t.Errorf("nil resp set must be empty, got %v", got.Elements())
	}

	// populated assignments → set of serials.
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

// TestDiffPlanAgainstGet_ExclusionSet is the single most important unit test in
// the package. The §9.1 server-authoritative exclusion set
// (storage_quota_size_megabytes, use_storage_quota_size, temporary_session_only,
// temporary_session_timeout) MUST NOT appear in the rollback-detection diff even
// when every one of them differs between plan and GET — their post-PUT drift is
// server-intentional, not a silent rollback.
//
// default_prestage is NOT in the set: it was excluded on the belief Jamf Pro
// silently keeps it false, but a refused claim is a hard 400 ALREADY_DEFAULT, so
// a plan/GET mismatch there is a real rollback and must be reported.
//
// Only those four fields are set on the plan; everything else is left null
// (zero-value types.*), which the per-field early-return guards skip.
func TestDiffPlanAgainstGet_ExclusionSet(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		StorageQuotaSizeMegabytes: types.Int64Value(8192),
		UseStorageQuotaSize:       types.BoolValue(true),
		TemporarySessionOnly:      types.BoolValue(true),
		TemporarySessionTimeout:   types.Int64Value(120),
	}
	got := &pro.GetMobileDevicePrestageV3{
		StorageQuotaSizeMegabytes: 1024,  // differs
		UseStorageQuotaSize:       false, // differs
		TemporarySessionOnly:      false, // differs
		TemporarySessionTimeout:   0,     // differs
	}
	diff := diffPlanAgainstGet(context.Background(), plan, got)
	if len(diff) != 0 {
		t.Fatalf("excluded fields must NEVER appear in the rollback diff; got %v", diff)
	}
}

// TestDiffPlanAgainstGet_DefaultPrestageIsChecked pins the counterpart: a
// planned default_prestage that the GET contradicts is a genuine rollback.
func TestDiffPlanAgainstGet_DefaultPrestageIsChecked(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{DefaultPrestage: types.BoolValue(true)}
	got := &pro.GetMobileDevicePrestageV3{DefaultPrestage: false}
	diff := diffPlanAgainstGet(context.Background(), plan, got)
	if len(diff) != 1 || diff[0] != "default_prestage" {
		t.Fatalf("default_prestage mismatch must be reported, got %v", diff)
	}
}

// TestDiffPlanAgainstGet_RealRollbackDetected proves the inverse: a genuine
// rollback of an IN-CHECK field (anchor_certificates) and a names field IS
// flagged. anchor_certificates only enters the check when the plan list is
// non-nil; diffNames only runs when BOTH plan.Names and got.Names are non-nil.
func TestDiffPlanAgainstGet_RealRollbackDetected(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		AnchorCertificates: newStringList(t, []string{"cert-A"}),
		Names: &NamesModel{
			AssignNamesUsing: types.StringValue("List of Names"),
		},
	}
	got := &pro.GetMobileDevicePrestageV3{
		AnchorCertificates: []string{"cert-B"}, // rolled back
		Names: &pro.MobileDevicePrestageNamesV3{
			AssignNamesUsing: new("Serial Numbers"), // rolled back
		},
	}
	diff := diffPlanAgainstGet(context.Background(), plan, got)

	wantAnchors, wantNames := false, false
	for _, f := range diff {
		switch f {
		case "anchor_certificates":
			wantAnchors = true
		case "names.assign_names_using":
			wantNames = true
		}
	}
	if !wantAnchors {
		t.Errorf("expected anchor_certificates mismatch in diff, got %v", diff)
	}
	if !wantNames {
		t.Errorf("expected names.assign_names_using mismatch in diff, got %v", diff)
	}
}

func TestEqualStringSlices(t *testing.T) {
	if !equalStringSlices([]string{"a", "b"}, []string{"a", "b"}) {
		t.Errorf("equal slices reported unequal")
	}
	if equalStringSlices([]string{"a", "b"}, []string{"b", "a"}) {
		t.Errorf("order matters: should be unequal")
	}
	if equalStringSlices([]string{"a"}, []string{"a", "b"}) {
		t.Errorf("different lengths should differ")
	}
}
