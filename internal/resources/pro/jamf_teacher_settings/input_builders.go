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

// buildTeacherSettingsInput converts the Terraform plan model into a Jamf
// Teacher settings full-replace PUT payload, preserving undeclared fields.
//
// timezoneId is Required and mandatory on every PUT (omission → HTTP 500), so
// it is always sent from the plan. For each Optional+Computed field, a known
// plan value (the user declared it, or UseStateForUnknown carried the prior
// state in on update) is sent; a null/unknown plan value (a field omitted on
// first create, where there is no prior state) falls back to the value read
// from the live settings (current). This is what makes "omit = preserve" hold
// on create too: the singleton always exists, so the first apply adopts the
// existing values rather than letting the full-replace write reset the
// undeclared ones (autoClear / maxRestrictionLengthSeconds → null,
// safelistedApps → [], isEnabled → false). On update, current is nil —
// UseStateForUnknown has already made every omitted field a known prior value,
// so the fallback is never consulted.
//
// An explicit "" restrictions_end_time is sent verbatim — the server persists
// it as null (the clear sentinel). The adopt fallbacks re-emit the response
// values as-is: a server-null autoClear echoes "" (re-nulled by the PUT) and a
// server-null maxRestrictionLengthSeconds decodes to 0 on the non-pointer
// response type (the wire cannot distinguish the two after the SDK decode, and
// the state builder reconciles either to the same user-facing view).
func buildTeacherSettingsInput(ctx context.Context, plan JamfTeacherSettingsResourceModel, current *pro.TeacherSettingsResponse) (*pro.TeacherSettingsRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	tz := plan.Timezone.ValueString()
	input := &pro.TeacherSettingsRequest{
		TimezoneID:                  &tz,
		IsEnabled:                   boolOrCurrent(plan.Enabled, current, func(c *pro.TeacherSettingsResponse) bool { return c.IsEnabled }),
		AutoClear:                   stringOrCurrent(plan.RestrictionsEndTime, current, func(c *pro.TeacherSettingsResponse) string { return c.AutoClear }),
		MaxRestrictionLengthSeconds: intOrCurrent(plan.MaximumRestrictionTimeSeconds, current, func(c *pro.TeacherSettingsResponse) int { return c.MaxRestrictionLengthSeconds }),
	}

	switch {
	case !plan.SafelistedApps.IsNull() && !plan.SafelistedApps.IsUnknown():
		// Known plan set (declared, or USFU-carried on update): always emit —
		// an empty plan set becomes a non-nil empty slice so `[]` clears the
		// collection server-side.
		var models []safelistedAppModel
		diags.Append(plan.SafelistedApps.ElementsAs(ctx, &models, false)...)
		if diags.HasError() {
			return nil, diags
		}
		apps := make([]pro.SafelistedApp, 0, len(models))
		for _, m := range models {
			name := m.Name.ValueString()
			bundleID := m.BundleID.ValueString()
			apps = append(apps, pro.SafelistedApp{Name: &name, BundleID: &bundleID})
		}
		input.SafelistedApps = &apps
	case current != nil:
		// Omitted on create: re-emit the live collection verbatim so the
		// full-replace write does not clear it to [].
		apps := make([]pro.SafelistedApp, len(current.SafelistedApps))
		copy(apps, current.SafelistedApps)
		input.SafelistedApps = &apps
	}

	return input, diags
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried),
// else falls back to the live value read from the server (preserve undeclared
// on create). Nil when there is no merge base (update path).
func boolOrCurrent(v types.Bool, current *pro.TeacherSettingsResponse, get func(*pro.TeacherSettingsResponse) bool) *bool {
	if p := helpers.OptionalBoolPointer(v); p != nil {
		return p
	}
	if current == nil {
		return nil
	}
	b := get(current)
	return &b
}

// stringOrCurrent mirrors boolOrCurrent for string fields.
func stringOrCurrent(v types.String, current *pro.TeacherSettingsResponse, get func(*pro.TeacherSettingsResponse) string) *string {
	if p := helpers.OptionalStringPointer(v); p != nil {
		return p
	}
	if current == nil {
		return nil
	}
	s := get(current)
	return &s
}

// intOrCurrent mirrors boolOrCurrent for int fields.
func intOrCurrent(v types.Int64, current *pro.TeacherSettingsResponse, get func(*pro.TeacherSettingsResponse) int) *int {
	if p := helpers.OptionalInt64Pointer(v); p != nil {
		return p
	}
	if current == nil {
		return nil
	}
	i := get(current)
	return &i
}
