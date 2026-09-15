// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package local_admin_password_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildLocalAdminPasswordSettingsInput converts the Terraform plan model into a
// full-replace LAPS settings payload.
//
// The body is seeded from the live settings (current, always read first by the
// caller) so any control the user did not declare keeps its current value — this
// is what makes "omit = preserve" hold, including on the first apply, which
// adopts the existing settings rather than resetting the undeclared controls to a
// default. A declared (or UseStateForUnknown-carried) plan value then overrides
// the seed.
//
// Two server invariants drive the rotation handling (verified 2026-06-11): the
// stored durations behind both interval controls must always be greater than
// zero, even while automatic rotation is off. So when rotation_interval is
// "Never" the builder turns automatic rotation off but keeps the existing
// (non-zero) expiration duration as a dormant value rather than zeroing it.
func buildLocalAdminPasswordSettingsInput(plan LocalAdminPasswordSettingsResourceModel, current *pro.LapsSettingsResponseV2) *pro.LapsSettingsRequestV2 {
	out := &pro.LapsSettingsRequestV2{
		AutoDeployEnabled:        current.AutoDeployEnabled,
		PasswordRotationTime:     current.PasswordRotationTime,
		AutoRotateEnabled:        current.AutoRotateEnabled,
		AutoRotateExpirationTime: current.AutoRotateExpirationTime,
	}

	if b := plan.LapsForPrestageAccountsEnabled; !b.IsNull() && !b.IsUnknown() {
		out.AutoDeployEnabled = b.ValueBool()
	}

	if s := plan.RotationAfterViewingInterval; !s.IsNull() && !s.IsUnknown() {
		if dur, ok := rotationAfterViewingToDuration[s.ValueString()]; ok {
			out.PasswordRotationTime = dur
		}
	}

	if s := plan.RotationInterval; !s.IsNull() && !s.IsUnknown() {
		if s.ValueString() == rotationIntervalNever {
			out.AutoRotateEnabled = false
			// Keep the existing dormant expiration; the server rejects a zero
			// here even when rotation is off.
			out.AutoRotateExpirationTime = nonZeroOrDefault(current.AutoRotateExpirationTime)
		} else if dur, ok := rotationIntervalDurationToValue[s.ValueString()]; ok {
			out.AutoRotateEnabled = true
			out.AutoRotateExpirationTime = dur
		}
	}

	// Explicit zero guard. Adopt-from-current keeps both durations > 0, but the
	// server rejects either as <= 0, so pin a non-zero fallback so a later
	// refactor cannot silently reintroduce a zero (notably in the "Never" branch).
	if out.PasswordRotationTime <= 0 {
		out.PasswordRotationTime = defaultPasswordRotationDuration
	}
	if out.AutoRotateExpirationTime <= 0 {
		out.AutoRotateExpirationTime = defaultAutoRotateExpirationDuration
	}

	return out
}

// nonZeroOrDefault returns dur when it is greater than zero, else the non-zero
// default expiration duration.
func nonZeroOrDefault(dur int) int {
	if dur > 0 {
		return dur
	}
	return defaultAutoRotateExpirationDuration
}
