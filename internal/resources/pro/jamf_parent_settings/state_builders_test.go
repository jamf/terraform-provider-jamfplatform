// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_parent_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// TestAssignJamfParentSettingsResourceModel_FullRoundTrip confirms a complete
// GET response lands in every state attribute, including the restricted-times
// map and the safelist set.
func TestAssignJamfParentSettingsResourceModel_FullRoundTrip(t *testing.T) {
	name, bundleID := "Example App", "com.example.app"
	wire := &pro.ParentApp{
		IsEnabled:                     true,
		TimezoneID:                    "Europe/London",
		DeviceGroupID:                 42,
		AllowClearPasscode:            new(true),
		DisassociateOnWipeAndReEnroll: new(false),
		AllowTemplates:                new(false),
		RestrictedTimes: map[string]pro.TimeFrame{
			"MONDAY": {BeginTime: new("08:30:00"), EndTime: new("15:30:00")},
			"FRIDAY": {BeginTime: new("09:00:00"), EndTime: new("14:00:00")},
		},
		SafelistedApps: &[]pro.SafelistedApp{{Name: &name, BundleID: &bundleID}},
	}

	var state JamfParentSettingsResourceModel
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.Enabled.ValueBool() != true {
		t.Errorf("enabled = %v, want true", state.Enabled.ValueBool())
	}
	if state.Timezone.ValueString() != "Europe/London" {
		t.Errorf("timezone = %q, want Europe/London", state.Timezone.ValueString())
	}
	if state.DeviceGroupID.ValueInt64() != 42 {
		t.Errorf("device_group_id = %d, want 42", state.DeviceGroupID.ValueInt64())
	}
	if state.AllowClearPasscode.ValueBool() != true {
		t.Errorf("allow_clear_passcode = %v, want true", state.AllowClearPasscode.ValueBool())
	}
	if state.RevokeOnWipeAndReEnroll.IsNull() || state.RevokeOnWipeAndReEnroll.ValueBool() != false {
		t.Errorf("revoke_on_wipe_and_re_enroll = %v, want known false", state.RevokeOnWipeAndReEnroll)
	}

	if state.RestrictedTimes.IsNull() || state.RestrictedTimes.IsUnknown() {
		t.Fatalf("restricted_times must be a known map")
	}
	var times map[string]restrictedTimeModel
	if diags := state.RestrictedTimes.ElementsAs(context.Background(), &times, false); diags.HasError() {
		t.Fatalf("decoding restricted_times: %v", diags)
	}
	if len(times) != 2 {
		t.Fatalf("restricted_times = %v, want 2 entries", times)
	}
	if times["MONDAY"].BeginTime.ValueString() != "08:30:00" || times["MONDAY"].EndTime.ValueString() != "15:30:00" {
		t.Errorf("restricted_times[MONDAY] = %v, want {08:30:00 15:30:00}", times["MONDAY"])
	}

	var apps []safelistedAppModel
	if diags := state.SafelistedApps.ElementsAs(context.Background(), &apps, false); diags.HasError() {
		t.Fatalf("decoding safelisted_apps: %v", diags)
	}
	if len(apps) != 1 || apps[0].Name.ValueString() != "Example App" || apps[0].BundleID.ValueString() != "com.example.app" {
		t.Errorf("safelisted_apps = %v, want the single wire entry", apps)
	}
}

// TestAssignJamfParentSettingsResourceModel_EmptyRestrictedTimes confirms an
// empty wire map flattens to a known empty map (not null) — the attribute is
// Required, so committed state can never be null, and {} is the valid "no
// restrictions" shape.
func TestAssignJamfParentSettingsResourceModel_EmptyRestrictedTimes(t *testing.T) {
	wire := &pro.ParentApp{TimezoneID: "UTC", RestrictedTimes: map[string]pro.TimeFrame{}}

	var state JamfParentSettingsResourceModel
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.RestrictedTimes.IsNull() || state.RestrictedTimes.IsUnknown() {
		t.Fatalf("restricted_times must be a known empty map, got %v", state.RestrictedTimes)
	}
	if n := len(state.RestrictedTimes.Elements()); n != 0 {
		t.Errorf("restricted_times has %d elements, want 0", n)
	}
}

// TestAssignJamfParentSettingsResourceModel_PartialRestrictedTimes confirms
// the GET's no-zero-fill contract is preserved: only the present day keys
// land in state.
func TestAssignJamfParentSettingsResourceModel_PartialRestrictedTimes(t *testing.T) {
	wire := &pro.ParentApp{
		TimezoneID: "UTC",
		RestrictedTimes: map[string]pro.TimeFrame{
			"SUNDAY": {BeginTime: new("00:00:00"), EndTime: new("12:05:00")},
		},
	}

	var state JamfParentSettingsResourceModel
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var times map[string]restrictedTimeModel
	if diags := state.RestrictedTimes.ElementsAs(context.Background(), &times, false); diags.HasError() {
		t.Fatalf("decoding restricted_times: %v", diags)
	}
	if len(times) != 1 {
		t.Fatalf("restricted_times = %v, want only the present SUNDAY key (no zero-fill)", times)
	}
	if times["SUNDAY"].BeginTime.ValueString() != "00:00:00" || times["SUNDAY"].EndTime.ValueString() != "12:05:00" {
		t.Errorf("restricted_times[SUNDAY] = %v, want {00:00:00 12:05:00}", times["SUNDAY"])
	}
}

// TestAssignJamfParentSettingsResourceModel_NilTimePointers confirms the
// defensive nil-guard: the server enforces both times on stored entries, but
// an unexpected nil degrades to "" rather than panicking.
func TestAssignJamfParentSettingsResourceModel_NilTimePointers(t *testing.T) {
	wire := &pro.ParentApp{
		TimezoneID: "UTC",
		RestrictedTimes: map[string]pro.TimeFrame{
			"MONDAY": {},
		},
	}

	var state JamfParentSettingsResourceModel
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var times map[string]restrictedTimeModel
	if diags := state.RestrictedTimes.ElementsAs(context.Background(), &times, false); diags.HasError() {
		t.Fatalf("decoding restricted_times: %v", diags)
	}
	if times["MONDAY"].BeginTime.ValueString() != "" || times["MONDAY"].EndTime.ValueString() != "" {
		t.Errorf("restricted_times[MONDAY] = %v, want empty-string fallbacks for nil wire pointers", times["MONDAY"])
	}
}

// TestAssignJamfParentSettingsResourceModel_NilSafelistKnownEmpty confirms a
// nil wire safelist pointer flattens to a known empty set (not null) so the
// Computed attribute resolves from Unknown at apply.
func TestAssignJamfParentSettingsResourceModel_NilSafelistKnownEmpty(t *testing.T) {
	wire := &pro.ParentApp{TimezoneID: "UTC", RestrictedTimes: map[string]pro.TimeFrame{}}

	var state JamfParentSettingsResourceModel
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.SafelistedApps.IsNull() || state.SafelistedApps.IsUnknown() {
		t.Fatalf("safelisted_apps must be a known empty set, got %v", state.SafelistedApps)
	}
	if n := len(state.SafelistedApps.Elements()); n != 0 {
		t.Errorf("safelisted_apps has %d elements, want 0", n)
	}
}

// TestAssignJamfParentSettingsResourceModel_NilResponse is a no-op guard.
func TestAssignJamfParentSettingsResourceModel_NilResponse(t *testing.T) {
	var state JamfParentSettingsResourceModel
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, nil); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Enabled.IsNull() {
		t.Errorf("nil response must leave state untouched (null)")
	}
}

// TestAssignJamfParentSettingsResourceModel_DoesNotClobberID pins the
// singleton convention: the assigner never writes state.ID — the CRUD handler
// stamps it after the assign call.
func TestAssignJamfParentSettingsResourceModel_DoesNotClobberID(t *testing.T) {
	var state JamfParentSettingsResourceModel
	state.ID = types.StringValue("pre-existing")

	wire := &pro.ParentApp{TimezoneID: "UTC", RestrictedTimes: map[string]pro.TimeFrame{}}
	if diags := assignJamfParentSettingsResourceModel(context.Background(), &state, wire); diags.HasError() {
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
