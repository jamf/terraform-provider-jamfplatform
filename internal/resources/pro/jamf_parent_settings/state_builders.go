// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_parent_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignJamfParentSettingsResourceModel populates resource state from a Jamf
// Parent settings GET response. Per STYLE_GUIDE §Singletons: GET-on-create to
// adopt, the per-type split applies:
//
//   - enabled (Optional+Computed bool, non-pointer on the wire): adopt the
//     server value directly — the Computed half of the attribute, so an
//     omitted field shows (and preserves) the live value.
//   - allow_clear_passcode / revoke_on_wipe_and_re_enroll (Optional+Computed
//     bools, *bool on the wire): adopt via BoolPointerValue — the GET always
//     echoes concrete values (wire-probed 2026-06-10); a nil would map to
//     null, which a Computed attribute may legitimately resolve to.
//   - restricted_times: always flattens to a known (possibly empty) map — the
//     attribute is Required, so committed state can never be null, and the
//     GET round-trips exactly the present day keys (no zero-fill).
//   - safelisted_apps: always flattens to a known (possibly empty) set so the
//     Computed attribute resolves from Unknown at apply.
//   - timezone / device_group_id are Required and always echoed; taken
//     directly.
//
// allowTemplates is intentionally not assigned — it is not modeled (see the
// crud.go annotation block).
//
// The assigner never writes state.ID — the CRUD handler stamps
// helpers.SingletonID after the assign call.
func assignJamfParentSettingsResourceModel(ctx context.Context, state *JamfParentSettingsResourceModel, s *pro.ParentApp) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	state.Enabled = types.BoolValue(s.IsEnabled)
	state.Timezone = types.StringValue(s.TimezoneID)
	state.DeviceGroupID = types.Int64Value(int64(s.DeviceGroupID))
	state.AllowClearPasscode = types.BoolPointerValue(s.AllowClearPasscode)
	state.RevokeOnWipeAndReEnroll = types.BoolPointerValue(s.DisassociateOnWipeAndReEnroll)

	rt, rtDiags := flattenRestrictedTimes(ctx, s.RestrictedTimes)
	diags.Append(rtDiags...)
	state.RestrictedTimes = rt

	apps, appDiags := flattenSafelistedApps(ctx, s.SafelistedApps)
	diags.Append(appDiags...)
	state.SafelistedApps = apps

	return diags
}

// flattenRestrictedTimes flattens the wire restrictedTimes map into the
// resource model's types.Map. Always returns a known (possibly empty) map —
// the attribute is Required, so null is never a valid committed state. The
// server enforces that both times are present on every stored entry
// ("Begin time and End time are required", wire-probed 2026-06-10), so the
// pointers are non-nil in practice; the nil-guard fallback to "" is defensive
// only, so an unexpected wire shape degrades to a visible empty string rather
// than a panic.
func flattenRestrictedTimes(ctx context.Context, wire map[string]pro.TimeFrame) (types.Map, diag.Diagnostics) {
	models := make(map[string]restrictedTimeModel, len(wire))
	for day, tf := range wire {
		models[day] = restrictedTimeModel{
			BeginTime: types.StringValue(derefOrEmpty(tf.BeginTime)),
			EndTime:   types.StringValue(derefOrEmpty(tf.EndTime)),
		}
	}
	return types.MapValueFrom(ctx, restrictedTimeObjectType, models)
}

// derefOrEmpty dereferences a wire string pointer, falling back to "" when
// nil (see flattenRestrictedTimes for why the guard exists).
func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// flattenSafelistedApps flattens the SDK safelisted-app slice into the
// resource model's types.Set. Always returns a known (possibly empty) set —
// a nil wire pointer flattens the same as an empty collection. Nil SDK
// pointers inside an element map to null element fields rather than panicking
// (the wire accepts entries with a missing name or bundle id).
func flattenSafelistedApps(ctx context.Context, apps *[]pro.SafelistedApp) (types.Set, diag.Diagnostics) {
	var wire []pro.SafelistedApp
	if apps != nil {
		wire = *apps
	}
	models := make([]safelistedAppModel, 0, len(wire))
	for _, a := range wire {
		models = append(models, safelistedAppModel{
			Name:     types.StringPointerValue(a.Name),
			BundleID: types.StringPointerValue(a.BundleID),
		})
	}
	return types.SetValueFrom(ctx, safelistedAppObjectType, models)
}
