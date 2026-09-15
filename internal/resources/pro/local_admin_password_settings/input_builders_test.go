// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package local_admin_password_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// current is a representative live-settings read used as the merge base.
func current() *pro.LapsSettingsResponseV2 {
	return &pro.LapsSettingsResponseV2{
		AutoDeployEnabled:        true,
		PasswordRotationTime:     86400, // 1 day
		AutoRotateEnabled:        true,
		AutoRotateExpirationTime: 2592000, // 30 days
	}
}

// TestBuildInput_AdoptsCurrentWhenUnset proves undeclared (null/unknown) controls
// keep their live value — the omit=preserve / adopt-on-create contract.
func TestBuildInput_AdoptsCurrentWhenUnset(t *testing.T) {
	plan := LocalAdminPasswordSettingsResourceModel{
		LapsForPrestageAccountsEnabled: types.BoolNull(),
		RotationInterval:               types.StringUnknown(),
		RotationAfterViewingInterval:   types.StringNull(),
	}
	got := buildLocalAdminPasswordSettingsInput(plan, current())

	if !got.AutoDeployEnabled {
		t.Errorf("AutoDeployEnabled = false, want adopted true")
	}
	if got.PasswordRotationTime != 86400 {
		t.Errorf("PasswordRotationTime = %d, want adopted 86400", got.PasswordRotationTime)
	}
	if !got.AutoRotateEnabled || got.AutoRotateExpirationTime != 2592000 {
		t.Errorf("rotation = (%v,%d), want adopted (true,2592000)", got.AutoRotateEnabled, got.AutoRotateExpirationTime)
	}
}

// TestBuildInput_DeclaredOverrides proves declared values win over the live base
// and that the enum labels translate to the right durations.
func TestBuildInput_DeclaredOverrides(t *testing.T) {
	plan := LocalAdminPasswordSettingsResourceModel{
		LapsForPrestageAccountsEnabled: types.BoolValue(false),
		RotationInterval:               types.StringValue("180 days"),
		RotationAfterViewingInterval:   types.StringValue("3 hours"),
	}
	got := buildLocalAdminPasswordSettingsInput(plan, current())

	if got.AutoDeployEnabled {
		t.Errorf("AutoDeployEnabled = true, want declared false")
	}
	if got.PasswordRotationTime != 10800 {
		t.Errorf("PasswordRotationTime = %d, want 10800 (3 hours)", got.PasswordRotationTime)
	}
	if !got.AutoRotateEnabled || got.AutoRotateExpirationTime != 15552000 {
		t.Errorf("rotation = (%v,%d), want (true,15552000) for 180 days", got.AutoRotateEnabled, got.AutoRotateExpirationTime)
	}
}

// TestBuildInput_NeverKeepsDormantExpiration proves rotation_interval=Never turns
// rotation off but keeps the existing (non-zero) expiration — the server rejects
// a zero here even while rotation is off.
func TestBuildInput_NeverKeepsDormantExpiration(t *testing.T) {
	plan := LocalAdminPasswordSettingsResourceModel{
		RotationInterval: types.StringValue(rotationIntervalNever),
	}
	got := buildLocalAdminPasswordSettingsInput(plan, current())

	if got.AutoRotateEnabled {
		t.Errorf("AutoRotateEnabled = true, want false for Never")
	}
	if got.AutoRotateExpirationTime != 2592000 {
		t.Errorf("AutoRotateExpirationTime = %d, want dormant 2592000 preserved", got.AutoRotateExpirationTime)
	}
}

// TestBuildInput_NeverDefaultsWhenCurrentZero proves the zero guard: if the live
// expiration were somehow 0, Never still sends a non-zero default.
func TestBuildInput_NeverDefaultsWhenCurrentZero(t *testing.T) {
	base := current()
	base.AutoRotateExpirationTime = 0
	plan := LocalAdminPasswordSettingsResourceModel{
		RotationInterval: types.StringValue(rotationIntervalNever),
	}
	got := buildLocalAdminPasswordSettingsInput(plan, base)

	if got.AutoRotateExpirationTime != defaultAutoRotateExpirationDuration {
		t.Errorf("AutoRotateExpirationTime = %d, want default %d", got.AutoRotateExpirationTime, defaultAutoRotateExpirationDuration)
	}
}

// TestBuildInput_NeverZeroGuardsBothDurations proves neither duration is ever 0
// (both server-rejected) even if the merge base carries zeros.
func TestBuildInput_NeverZeroGuardsBothDurations(t *testing.T) {
	base := &pro.LapsSettingsResponseV2{} // all zero
	plan := LocalAdminPasswordSettingsResourceModel{
		RotationInterval: types.StringValue(rotationIntervalNever),
	}
	got := buildLocalAdminPasswordSettingsInput(plan, base)

	if got.PasswordRotationTime <= 0 {
		t.Errorf("PasswordRotationTime = %d, want > 0", got.PasswordRotationTime)
	}
	if got.AutoRotateExpirationTime <= 0 {
		t.Errorf("AutoRotateExpirationTime = %d, want > 0", got.AutoRotateExpirationTime)
	}
}
