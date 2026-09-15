// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// platformSsoBundleConfigured reports whether platform_sso_app_bundle_id holds a
// real (unattended-mode) value. Empty string is the wire "none" form, so it
// counts as unset.
func platformSsoBundleConfigured(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

// pssoConfigProfileConfigured reports whether psso_config_profile_id holds a
// real (attended-mode) value. Both the empty string and the "-1" sentinel are
// "none", so either counts as unset.
func pssoConfigProfileConfigured(v types.String) bool {
	if v.IsNull() || v.IsUnknown() {
		return false
	}
	s := v.ValueString()
	return s != "" && s != sentinelNoneIDDash1
}

// enrollmentCustomizationEnabled reports whether enrollment_customization_id
// selects a real customization. Its "none" sentinel is "0" (not "-1"), and ""
// is also none, so either counts as disabled.
func enrollmentCustomizationEnabled(v types.String) bool {
	if v.IsNull() || v.IsUnknown() {
		return false
	}
	s := v.ValueString()
	return s != "" && s != "0"
}

// pssoConfigProfileConflictsWithBundle is a value-specific cross-field validator
// attached to psso_config_profile_id (attended Platform SSO). It rejects a
// config that ALSO sets platform_sso_app_bundle_id (unattended Platform SSO);
// Jamf Pro treats the two as a single either/or mode. Off-the-shelf
// stringvalidator.ConflictsWith is wrong here because each field's "none" form
// is a value (psso_config_profile_id's literal "-1" sentinel, the bundle's "")
// rather than null, so ConflictsWith would over-fire when one side is "none".
func pssoConfigProfileConflictsWithBundle() validator.String {
	return pssoConfigProfileConflictsWithBundleValidator{}
}

type pssoConfigProfileConflictsWithBundleValidator struct{}

func (pssoConfigProfileConflictsWithBundleValidator) Description(_ context.Context) string {
	return "psso_config_profile_id (attended Platform SSO) and platform_sso_app_bundle_id (unattended Platform SSO) are mutually exclusive."
}

func (v pssoConfigProfileConflictsWithBundleValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (pssoConfigProfileConflictsWithBundleValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	// Only the attended-mode "real value" case can conflict.
	if !pssoConfigProfileConfigured(req.ConfigValue) {
		return
	}
	companion := path.Root("platform_sso_app_bundle_id")
	var bundle types.String
	if d := req.Config.GetAttribute(ctx, companion, &bundle); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	// Defer on unknown sibling (§config-time validators must defer on unknown);
	// platformSsoBundleConfigured already returns false for unknown.
	if platformSsoBundleConfigured(bundle) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Conflicting Platform SSO mode inputs",
			"psso_config_profile_id selects attended Platform SSO and platform_sso_app_bundle_id selects unattended Platform SSO; the two are mutually exclusive. Set at most one.",
		)
	}
}

// recoveryLockPasswordTypeRandomConflictsWithPassword fires only when the
// declared type is "RANDOM" but the user also supplied a plaintext
// recovery_lock_password. Off-the-shelf ConflictsWith would over-fire when
// the type is "MANUAL".
func recoveryLockPasswordTypeRandomConflictsWithPassword() validator.String {
	return recoveryLockPasswordTypeRandomConflictsWithPasswordValidator{}
}

type recoveryLockPasswordTypeRandomConflictsWithPasswordValidator struct{}

func (recoveryLockPasswordTypeRandomConflictsWithPasswordValidator) Description(_ context.Context) string {
	return `When recovery_lock_password_type is "RANDOM", recovery_lock_password must not be set.`
}

func (v recoveryLockPasswordTypeRandomConflictsWithPasswordValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (recoveryLockPasswordTypeRandomConflictsWithPasswordValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != pro.ComputerPrestageV3RecoveryLockPasswordTypeRandom {
		return
	}
	companion := path.Root("recovery_lock_password")
	var pwd types.String
	if d := req.Config.GetAttribute(ctx, companion, &pwd); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if pwd.IsNull() || pwd.IsUnknown() {
		return
	}
	if pwd.ValueString() == "" {
		return
	}
	resp.Diagnostics.AddAttributeError(
		companion,
		"recovery_lock_password conflicts with recovery_lock_password_type = RANDOM",
		`Jamf Pro generates random Recovery Lock passwords when recovery_lock_password_type = "RANDOM"; remove recovery_lock_password from the configuration.`,
	)
}

// recoveryLockPasswordRequiresManualAndEnabled fires when the user supplies a
// recovery_lock_password without also setting enable_recovery_lock=true AND
// recovery_lock_password_type="MANUAL".
func recoveryLockPasswordRequiresManualAndEnabled() validator.String {
	return recoveryLockPasswordRequiresManualAndEnabledValidator{}
}

type recoveryLockPasswordRequiresManualAndEnabledValidator struct{}

func (recoveryLockPasswordRequiresManualAndEnabledValidator) Description(_ context.Context) string {
	return `recovery_lock_password requires enable_recovery_lock = true and recovery_lock_password_type = "MANUAL".`
}

func (v recoveryLockPasswordRequiresManualAndEnabledValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (recoveryLockPasswordRequiresManualAndEnabledValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == "" {
		return
	}

	var enable types.Bool
	if d := req.Config.GetAttribute(ctx, path.Root("enable_recovery_lock"), &enable); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if enable.IsUnknown() {
		return
	}
	if enable.IsNull() || !enable.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("recovery_lock_password"),
			"recovery_lock_password requires enable_recovery_lock = true",
			"Set enable_recovery_lock = true when supplying a manual recovery lock password.",
		)
		return
	}

	var pwdType types.String
	if d := req.Config.GetAttribute(ctx, path.Root("recovery_lock_password_type"), &pwdType); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if pwdType.IsNull() || pwdType.IsUnknown() {
		return
	}
	if pwdType.ValueString() != pro.ComputerPrestageV3RecoveryLockPasswordTypeManual {
		resp.Diagnostics.AddAttributeError(
			path.Root("recovery_lock_password"),
			`recovery_lock_password requires recovery_lock_password_type = "MANUAL"`,
			fmt.Sprintf(`recovery_lock_password_type is %q; only "MANUAL" accepts a user-supplied password.`, pwdType.ValueString()),
		)
	}
}

// pssoAttendedConflictsWithEnrollmentCustomization is attached to
// psso_config_profile_id. Jamf Pro forbids an enrollment customization when
// attended Platform SSO is in use, so when this field holds a real (attended)
// value and enrollment_customization_id is enabled, the config is rejected.
func pssoAttendedConflictsWithEnrollmentCustomization() validator.String {
	return pssoAttendedConflictsWithEnrollmentCustomizationValidator{}
}

type pssoAttendedConflictsWithEnrollmentCustomizationValidator struct{}

func (pssoAttendedConflictsWithEnrollmentCustomizationValidator) Description(_ context.Context) string {
	return "Enrollment customization cannot be enabled when attended Platform SSO (psso_config_profile_id) is used."
}

func (v pssoAttendedConflictsWithEnrollmentCustomizationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (pssoAttendedConflictsWithEnrollmentCustomizationValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	// Only attended Platform SSO triggers the constraint.
	if !pssoConfigProfileConfigured(req.ConfigValue) {
		return
	}
	companion := path.Root("enrollment_customization_id")
	var customization types.String
	if d := req.Config.GetAttribute(ctx, companion, &customization); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	// Defer on unknown companion; enrollmentCustomizationEnabled is false for unknown.
	if enrollmentCustomizationEnabled(customization) {
		resp.Diagnostics.AddAttributeError(
			companion,
			"Enrollment customization conflicts with attended Platform SSO",
			`psso_config_profile_id selects attended Platform SSO, which is incompatible with an enrollment customization. Set enrollment_customization_id to "0" (none) or switch to unattended Platform SSO (platform_sso_app_bundle_id).`,
		)
	}
}

// prefillTypeCustomRequiresFullAndUserNames fires when account_settings.prefill_type
// is "CUSTOM" but prefill_account_full_name or prefill_account_user_name is empty.
func prefillTypeCustomRequiresFullAndUserNames() validator.String {
	return prefillTypeCustomRequiresFullAndUserNamesValidator{}
}

type prefillTypeCustomRequiresFullAndUserNamesValidator struct{}

func (prefillTypeCustomRequiresFullAndUserNamesValidator) Description(_ context.Context) string {
	return `account_settings.prefill_type = "CUSTOM" requires prefill_account_full_name and prefill_account_user_name to be set.`
}

func (v prefillTypeCustomRequiresFullAndUserNamesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (prefillTypeCustomRequiresFullAndUserNamesValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != "CUSTOM" {
		return
	}

	checkSibling := func(name string) {
		sibling := req.Path.ParentPath().AtName(name)
		var v types.String
		if d := req.Config.GetAttribute(ctx, sibling, &v); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
		if v.IsUnknown() {
			return
		}
		if v.IsNull() || v.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				sibling,
				fmt.Sprintf(`%s is required when prefill_type = "CUSTOM"`, name),
				`Supply a non-empty value or change prefill_type to "DEVICE_OWNER".`,
			)
		}
	}
	checkSibling("prefill_account_full_name")
	checkSibling("prefill_account_user_name")
}
