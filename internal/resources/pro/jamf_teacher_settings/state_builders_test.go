// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_teacher_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// TestAssignJamfTeacherSettingsResourceModel_FullRoundTrip confirms a complete
// GET response lands in every state attribute, including the safelist set.
func TestAssignJamfTeacherSettingsResourceModel_FullRoundTrip(t *testing.T) {
	name, bundleID := "Example App", "com.example.app"
	wire := &pro.TeacherSettingsResponse{
		IsEnabled:                   true,
		TimezoneID:                  "Europe/London",
		AutoClear:                   "17:30:00",
		MaxRestrictionLengthSeconds: 3600,
		SafelistedApps:              []pro.SafelistedApp{{Name: &name, BundleID: &bundleID}},
	}

	var state JamfTeacherSettingsResourceModel
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.Enabled.ValueBool() != true {
		t.Errorf("enabled = %v, want true", state.Enabled.ValueBool())
	}
	if state.Timezone.ValueString() != "Europe/London" {
		t.Errorf("timezone = %q, want Europe/London", state.Timezone.ValueString())
	}
	if state.RestrictionsEndTime.ValueString() != "17:30:00" {
		t.Errorf("restrictions_end_time = %q, want 17:30:00", state.RestrictionsEndTime.ValueString())
	}
	if state.MaximumRestrictionTimeSeconds.ValueInt64() != 3600 {
		t.Errorf("maximum_restriction_time_seconds = %d, want 3600", state.MaximumRestrictionTimeSeconds.ValueInt64())
	}
	if state.SafelistedApps.IsNull() || state.SafelistedApps.IsUnknown() {
		t.Fatalf("safelisted_apps must be a known set")
	}
	var apps []safelistedAppModel
	if diags := state.SafelistedApps.ElementsAs(context.Background(), &apps, false); diags.HasError() {
		t.Fatalf("decoding safelisted_apps: %v", diags)
	}
	if len(apps) != 1 || apps[0].Name.ValueString() != "Example App" || apps[0].BundleID.ValueString() != "com.example.app" {
		t.Errorf("safelisted_apps = %v, want the single wire entry", apps)
	}
}

// TestAssignJamfTeacherSettingsResourceModel_EmptyAutoClearReconciles confirms
// the "" clear-sentinel reconcile: a server-null autoClear echoes "" on the
// wire and must map to null state when the field was omitted, but stay "" when
// the user explicitly declared the clear sentinel.
func TestAssignJamfTeacherSettingsResourceModel_EmptyAutoClearReconciles(t *testing.T) {
	wire := &pro.TeacherSettingsResponse{TimezoneID: "UTC", AutoClear: ""}

	// Omitted (prior null): "" -> null.
	var omitted JamfTeacherSettingsResourceModel
	omitted.RestrictionsEndTime = types.StringNull()
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &omitted, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !omitted.RestrictionsEndTime.IsNull() {
		t.Errorf("restrictions_end_time = %v, want null for omitted field + empty echo", omitted.RestrictionsEndTime)
	}

	// Declared "" (the clear sentinel): "" is preserved so config == state.
	var cleared JamfTeacherSettingsResourceModel
	cleared.RestrictionsEndTime = types.StringValue("")
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &cleared, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if cleared.RestrictionsEndTime.IsNull() || cleared.RestrictionsEndTime.ValueString() != "" {
		t.Errorf("restrictions_end_time = %v, want explicit \"\" preserved", cleared.RestrictionsEndTime)
	}
}

// TestAssignJamfTeacherSettingsResourceModel_EmptySafelistKnown confirms an
// empty wire collection flattens to a known empty set (not null) so the
// Computed attribute resolves from Unknown at apply.
func TestAssignJamfTeacherSettingsResourceModel_EmptySafelistKnown(t *testing.T) {
	wire := &pro.TeacherSettingsResponse{TimezoneID: "UTC", SafelistedApps: []pro.SafelistedApp{}}

	var state JamfTeacherSettingsResourceModel
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.SafelistedApps.IsNull() || state.SafelistedApps.IsUnknown() {
		t.Fatalf("safelisted_apps must be a known empty set, got %v", state.SafelistedApps)
	}
	if n := len(state.SafelistedApps.Elements()); n != 0 {
		t.Errorf("safelisted_apps has %d elements, want 0", n)
	}
}

// TestAssignJamfTeacherSettingsResourceModel_NilElementFields confirms wire
// entries with missing name/bundleId (the server accepts them) map to null
// element fields rather than panicking.
func TestAssignJamfTeacherSettingsResourceModel_NilElementFields(t *testing.T) {
	wire := &pro.TeacherSettingsResponse{
		TimezoneID:     "UTC",
		SafelistedApps: []pro.SafelistedApp{{}},
	}

	var state JamfTeacherSettingsResourceModel
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var apps []safelistedAppModel
	if diags := state.SafelistedApps.ElementsAs(context.Background(), &apps, false); diags.HasError() {
		t.Fatalf("decoding safelisted_apps: %v", diags)
	}
	if len(apps) != 1 || !apps[0].Name.IsNull() || !apps[0].BundleID.IsNull() {
		t.Errorf("safelisted_apps = %v, want one element with null name/bundle_id", apps)
	}
}

// TestAssignJamfTeacherSettingsResourceModel_NilResponse is a no-op guard.
func TestAssignJamfTeacherSettingsResourceModel_NilResponse(t *testing.T) {
	var state JamfTeacherSettingsResourceModel
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &state, nil); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Enabled.IsNull() {
		t.Errorf("nil response must leave state untouched (null)")
	}
}

// TestAssignJamfTeacherSettingsResourceModel_DoesNotClobberID pins the
// singleton convention: the assigner never writes state.ID — the CRUD handler
// stamps it after the assign call.
func TestAssignJamfTeacherSettingsResourceModel_DoesNotClobberID(t *testing.T) {
	var state JamfTeacherSettingsResourceModel
	state.ID = types.StringValue("pre-existing")

	wire := &pro.TeacherSettingsResponse{TimezoneID: "UTC"}
	if diags := assignJamfTeacherSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "pre-existing" {
		t.Errorf("assigner must not touch state.ID, got %q", state.ID.ValueString())
	}
}

// TestSingletonIDConstant catches accidental drift in the shared constant the
// resource stores as its state ID.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID = %q, want \"singleton\"", helpers.SingletonID)
	}
}
