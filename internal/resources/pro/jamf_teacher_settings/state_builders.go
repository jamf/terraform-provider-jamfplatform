// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_teacher_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignJamfTeacherSettingsResourceModel populates resource state from a Jamf
// Teacher settings GET response. Per STYLE_GUIDE §Singletons: GET-on-create to
// adopt, the per-type split applies:
//
//   - enabled / maximum_restriction_time_seconds (Optional+Computed bool/int):
//     adopt the server value directly — the Computed half of the attribute, so
//     an omitted field shows (and preserves) the live value. NOT the
//     Reconcile* helpers, which return null when prior state is unset and
//     would blank an omitted toggle instead of adopting it. The response
//     decodes a server-null maxRestrictionLengthSeconds to 0 (non-pointer SDK
//     field), so null and 0 present identically — they are indistinguishable
//     after the SDK decode.
//   - restrictions_end_time (Optional+Computed string with a "" clear
//     sentinel): ReconcileOptionalString, so a user-declared "" survives the
//     server echoing the cleared value as "" (state "" == config ""), while an
//     omitted field maps a server-null/"" echo to null.
//   - safelisted_apps: always flattens to a known (possibly empty) set so the
//     Computed attribute resolves from Unknown at apply — the response always
//     carries the collection ([] when empty).
//   - timezone is Required and always echoed; taken directly.
//
// The assigner never writes state.ID — the CRUD handler stamps
// helpers.SingletonID after the assign call.
func assignJamfTeacherSettingsResourceModel(ctx context.Context, state *JamfTeacherSettingsResourceModel, s *pro.TeacherSettingsResponse) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	state.Enabled = types.BoolValue(s.IsEnabled)
	state.Timezone = types.StringValue(s.TimezoneID)
	state.RestrictionsEndTime = helpers.ReconcileOptionalString(s.AutoClear, state.RestrictionsEndTime)
	state.MaximumRestrictionTimeSeconds = types.Int64Value(int64(s.MaxRestrictionLengthSeconds))

	apps, appDiags := flattenSafelistedApps(ctx, s.SafelistedApps)
	diags.Append(appDiags...)
	state.SafelistedApps = apps

	return diags
}

// flattenSafelistedApps flattens the SDK safelisted-app slice into the resource
// model's types.Set. Always returns a known (possibly empty) set. Nil SDK
// pointers inside an element map to null element fields rather than panicking
// (the wire accepts entries with a missing name or bundle id).
func flattenSafelistedApps(ctx context.Context, apps []pro.SafelistedApp) (types.Set, diag.Diagnostics) {
	models := make([]safelistedAppModel, 0, len(apps))
	for _, a := range apps {
		models = append(models, safelistedAppModel{
			Name:     types.StringPointerValue(a.Name),
			BundleID: types.StringPointerValue(a.BundleID),
		})
	}
	return types.SetValueFrom(ctx, safelistedAppObjectType, models)
}
