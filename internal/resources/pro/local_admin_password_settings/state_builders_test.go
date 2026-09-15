// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package local_admin_password_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignModel_MapsPresets proves stored durations map back to dropdown labels
// and the toggle is taken directly.
func TestAssignModel_MapsPresets(t *testing.T) {
	var state LocalAdminPasswordSettingsResourceModel
	var diags diag.Diagnostics
	assignLocalAdminPasswordSettingsResourceModel(&state, &pro.LapsSettingsResponseV2{
		AutoDeployEnabled:        true,
		PasswordRotationTime:     604800, // 7 days
		AutoRotateEnabled:        true,
		AutoRotateExpirationTime: 5184000, // 60 days
	}, &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.LapsForPrestageAccountsEnabled.ValueBool() {
		t.Errorf("toggle = false, want true")
	}
	if state.RotationAfterViewingInterval.ValueString() != "7 days" {
		t.Errorf("rotation_after_viewing = %q, want \"7 days\"", state.RotationAfterViewingInterval.ValueString())
	}
	if state.RotationInterval.ValueString() != "60 days" {
		t.Errorf("rotation_interval = %q, want \"60 days\"", state.RotationInterval.ValueString())
	}
}

// TestAssignModel_DisabledIsNever proves automatic rotation being off maps to
// "Never" regardless of the (dormant) stored expiration — no error even though
// the dormant value is not a preset.
func TestAssignModel_DisabledIsNever(t *testing.T) {
	var state LocalAdminPasswordSettingsResourceModel
	var diags diag.Diagnostics
	assignLocalAdminPasswordSettingsResourceModel(&state, &pro.LapsSettingsResponseV2{
		AutoDeployEnabled:        false,
		PasswordRotationTime:     3600, // 1 hour
		AutoRotateEnabled:        false,
		AutoRotateExpirationTime: 7776000, // 90 days — not a preset, but dormant
	}, &diags)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.RotationInterval.ValueString() != rotationIntervalNever {
		t.Errorf("rotation_interval = %q, want %q", state.RotationInterval.ValueString(), rotationIntervalNever)
	}
}

// TestAssignModel_NonPresetAfterViewingErrors proves an out-of-band custom
// rotation-after-viewing duration surfaces a diagnostic rather than snapping to a
// preset.
func TestAssignModel_NonPresetAfterViewingErrors(t *testing.T) {
	var state LocalAdminPasswordSettingsResourceModel
	var diags diag.Diagnostics
	assignLocalAdminPasswordSettingsResourceModel(&state, &pro.LapsSettingsResponseV2{
		PasswordRotationTime:     12345, // custom, unsupported
		AutoRotateEnabled:        false,
		AutoRotateExpirationTime: 604800,
	}, &diags)

	if !diags.HasError() {
		t.Fatalf("expected a diagnostic for an unsupported rotation-after-viewing value")
	}
}

// TestAssignModel_NonPresetIntervalWhileEnabledErrors proves an out-of-band custom
// rotation interval while rotation is ON surfaces a diagnostic.
func TestAssignModel_NonPresetIntervalWhileEnabledErrors(t *testing.T) {
	var state LocalAdminPasswordSettingsResourceModel
	var diags diag.Diagnostics
	assignLocalAdminPasswordSettingsResourceModel(&state, &pro.LapsSettingsResponseV2{
		PasswordRotationTime:     604800,
		AutoRotateEnabled:        true,
		AutoRotateExpirationTime: 99999, // custom, unsupported, while enabled
	}, &diags)

	if !diags.HasError() {
		t.Fatalf("expected a diagnostic for an unsupported rotation interval while enabled")
	}
}
