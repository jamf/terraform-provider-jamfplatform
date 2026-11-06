// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// minEnforcedTemporarySessionTimeout is the Jamf Pro admin-UI minimum for a
// temporary session timeout when enforcement is enabled. Below this, Jamf Pro
// silently nulls the value (§F11); the validator surfaces it at plan time.
const minEnforcedTemporarySessionTimeout = 30

// The IANA timezone validator formerly defined here was promoted to the shared
// package as validators.IANATimeZone() (internal/common/validators/timezone.go)
// when the Jamf Teacher settings resource became its second consumer; the probe
// evidence travelled with it.

// singleNameRequiresSingleDeviceName fires when names.assign_names_using is
// "Single Name" but names.single_device_name is empty. Provider-side courtesy
// validator (the server is permissive — §F11); mirrors the prefillType
// value-specific validator shape from the computer sibling.
func singleNameRequiresSingleDeviceName() validator.String {
	return singleNameRequiresSingleDeviceNameValidator{}
}

type singleNameRequiresSingleDeviceNameValidator struct{}

func (singleNameRequiresSingleDeviceNameValidator) Description(_ context.Context) string {
	return `names.assign_names_using = "Single Name" requires names.single_device_name to be set.`
}

func (v singleNameRequiresSingleDeviceNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (singleNameRequiresSingleDeviceNameValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != "Single Name" {
		return
	}

	sibling := req.Path.ParentPath().AtName("single_device_name")
	var single types.String
	if d := req.Config.GetAttribute(ctx, sibling, &single); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if single.IsUnknown() {
		return
	}
	if single.IsNull() || single.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			sibling,
			`single_device_name is required when assign_names_using = "Single Name"`,
			`Supply a non-empty single_device_name or choose a different assign_names_using mode.`,
		)
	}
}

// storageQuotaConflictsWithTemporarySession fires when both
// use_storage_quota_size and temporary_session_only are set true. They are the
// two mutually-exclusive Shared-iPad storage modes — Jamf Pro silently forces
// use_storage_quota_size to false when temporary_session_only is true (§F9), so
// this is a provider-side courtesy guard surfacing the conflict at plan time.
// Attached to use_storage_quota_size so it fires once. Mirrors the computer
// sibling's recoveryLockPasswordTypeRandomConflictsWithPassword shape.
func storageQuotaConflictsWithTemporarySession() validator.Bool {
	return storageQuotaConflictsWithTemporarySessionValidator{}
}

type storageQuotaConflictsWithTemporarySessionValidator struct{}

func (storageQuotaConflictsWithTemporarySessionValidator) Description(_ context.Context) string {
	return "use_storage_quota_size cannot be true when temporary_session_only is true (mutually-exclusive Shared iPad storage modes)."
}

func (v storageQuotaConflictsWithTemporarySessionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (storageQuotaConflictsWithTemporarySessionValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || !req.ConfigValue.ValueBool() {
		return
	}

	companion := path.Root("temporary_session_only")
	var temp types.Bool
	if d := req.Config.GetAttribute(ctx, companion, &temp); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if temp.IsNull() || temp.IsUnknown() || !temp.ValueBool() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"use_storage_quota_size conflicts with temporary_session_only",
		"use_storage_quota_size and temporary_session_only are mutually-exclusive Shared iPad storage modes; set at most one to true.",
	)
}

// attributeName is the attribute a validator request is attached to.
func attributeName(p path.Path) string {
	steps := p.Steps()
	if len(steps) == 0 {
		return p.String()
	}
	return steps[len(steps)-1].String()
}

// readBoolPair reads the replacement and the deprecated spelling of one boolean from the
// configuration and folds them into one value: null when neither is set, unknown when either
// is unknown, and otherwise the replacement or the deprecated attribute (negated for an inverted
// pair). Cross-field validators use it so a configuration that still uses the old name is held
// to the same rule.
func readBoolPair(ctx context.Context, cfg tfsdk.Config, newName, oldName string, invert bool) (types.Bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	var replacement, deprecated types.Bool
	diags.Append(cfg.GetAttribute(ctx, path.Root(newName), &replacement)...)
	diags.Append(cfg.GetAttribute(ctx, path.Root(oldName), &deprecated)...)
	if diags.HasError() {
		return types.BoolNull(), diags
	}
	if replacement.IsUnknown() || deprecated.IsUnknown() {
		return types.BoolUnknown(), diags
	}
	return pickBool(replacement, deprecated, invert), diags
}

// sharedIPadRequiresPreventActivationLock fires when Enable Shared iPad is true but
// prevent_activation_lock is not also true. Jamf Pro enforces this as a hard prerequisite: the
// server rejects the write with `400 PREREQUISITE_NOT_MET: "Prevent activation lock needs to be
// enabled in order to enable Shared iPad"`, so this surfaces it at plan time. The rule is
// value-specific (it fires only when Shared iPad is true), so a custom validator is required per
// STYLE_GUIDE §Cross-field validation; off-the-shelf `boolvalidator.AlsoRequires` would fire when
// it is false too. It attaches to enable_shared_ipad and its deprecated alias multi_user, and the
// error points at prevent_activation_lock, the field the user must set.
func sharedIPadRequiresPreventActivationLock() validator.Bool {
	return sharedIPadRequiresPreventActivationLockValidator{}
}

type sharedIPadRequiresPreventActivationLockValidator struct{}

func (sharedIPadRequiresPreventActivationLockValidator) Description(_ context.Context) string {
	return "enable_shared_ipad requires prevent_activation_lock to be true."
}

func (v sharedIPadRequiresPreventActivationLockValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (sharedIPadRequiresPreventActivationLockValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || !req.ConfigValue.ValueBool() {
		return
	}

	companion := path.Root("prevent_activation_lock")
	var pal types.Bool
	if d := req.Config.GetAttribute(ctx, companion, &pal); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if pal.IsUnknown() {
		return
	}
	if !pal.IsNull() && pal.ValueBool() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		companion,
		"prevent_activation_lock required for Shared iPad",
		attributeName(req.Path)+" = true (Enable Shared iPad) requires prevent_activation_lock = true; Jamf Pro rejects the configuration otherwise.",
	)
}

// sharedIPadRequiresSupervision fires when Enable Shared iPad is true but the supervision
// setting is not also true. Unlike prevent_activation_lock, which Jamf Pro rejects with a hard
// 400, the server silently coerces Shared iPad back to false on a device that is not supervised,
// which Terraform reports as "provider produced inconsistent result after apply". This turns that
// into a plan-time error. Apple requires Shared iPad devices to be supervised, so no valid
// configuration is rejected. It reads supervise_devices_with_ios_12_or_earlier and its deprecated
// alias supervised, and the error points at the replacement.
func sharedIPadRequiresSupervision() validator.Bool {
	return sharedIPadRequiresSupervisionValidator{}
}

type sharedIPadRequiresSupervisionValidator struct{}

func (sharedIPadRequiresSupervisionValidator) Description(_ context.Context) string {
	return "enable_shared_ipad requires supervise_devices_with_ios_12_or_earlier to be true."
}

func (v sharedIPadRequiresSupervisionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (sharedIPadRequiresSupervisionValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || !req.ConfigValue.ValueBool() {
		return
	}

	sup, d := readBoolPair(ctx, req.Config, "supervise_devices_with_ios_12_or_earlier", "supervised", false)
	if d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if sup.IsUnknown() {
		return
	}
	if !sup.IsNull() && sup.ValueBool() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("supervise_devices_with_ios_12_or_earlier"),
		"supervision required for Shared iPad",
		attributeName(req.Path)+" = true (Enable Shared iPad) requires supervise_devices_with_ios_12_or_earlier = true; otherwise Jamf Pro turns Shared iPad off without an error, because Apple requires Shared iPad devices to be supervised.",
	)
}

// preserveManagedAppsRequiresReturnToService fires when preserve_managed_apps is true while
// Return to Service is explicitly disabled. The admin UI shows the control only under "Allow use
// of the Return to Service option during device reset", so the setting means nothing without it.
// A configuration that leaves Return to Service unset is accepted, because the value Jamf Pro
// holds for it is not visible at validation time. It reads allow_return_to_service and its
// deprecated alias rts_enabled.
func preserveManagedAppsRequiresReturnToService() validator.Bool {
	return preserveManagedAppsRequiresReturnToServiceValidator{}
}

type preserveManagedAppsRequiresReturnToServiceValidator struct{}

func (preserveManagedAppsRequiresReturnToServiceValidator) Description(_ context.Context) string {
	return "preserve_managed_apps cannot be true while allow_return_to_service is false."
}

func (v preserveManagedAppsRequiresReturnToServiceValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (preserveManagedAppsRequiresReturnToServiceValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || !req.ConfigValue.ValueBool() {
		return
	}

	rts, d := readBoolPair(ctx, req.Config, "allow_return_to_service", "rts_enabled", false)
	if d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if rts.IsNull() || rts.IsUnknown() || rts.ValueBool() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("allow_return_to_service"),
		"Return to Service required for preserve_managed_apps",
		"preserve_managed_apps = true applies only while Return to Service is allowed. Set allow_return_to_service = true or drop preserve_managed_apps.",
	)
}

// temporarySessionTimeoutMinimum fires when enforce_temporary_session_timeout
// is true and temporary_session_timeout is set below the Jamf Pro minimum of
// 30. Below the minimum Jamf Pro silently nulls the value (§F11); the validator
// surfaces it at plan time. Attached to temporary_session_timeout.
func temporarySessionTimeoutMinimum() validator.Int64 {
	return temporarySessionTimeoutMinimumValidator{}
}

type temporarySessionTimeoutMinimumValidator struct{}

func (temporarySessionTimeoutMinimumValidator) Description(_ context.Context) string {
	return "temporary_session_timeout must be at least 30 when enforce_temporary_session_timeout is true."
}

func (v temporarySessionTimeoutMinimumValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (temporarySessionTimeoutMinimumValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueInt64() >= minEnforcedTemporarySessionTimeout {
		return
	}

	companion := path.Root("enforce_temporary_session_timeout")
	var enforce types.Bool
	if d := req.Config.GetAttribute(ctx, companion, &enforce); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	if enforce.IsNull() || enforce.IsUnknown() || !enforce.ValueBool() {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"temporary_session_timeout below minimum",
		// Wrap-safe: no whitespace at the ~80-col wrap point (memory:
		// ExpectError regex line-wrap). "at-least-30" is a single token.
		"temporary_session_timeout must be at-least-30 when enforce_temporary_session_timeout is true; Jamf Pro discards smaller values.",
	)
}
