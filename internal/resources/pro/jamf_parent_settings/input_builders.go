// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_parent_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildParentSettingsInput converts the Terraform plan model into a Jamf
// Parent settings full-replace PUT payload, preserving undeclared fields.
//
// The three Required attributes — timezone, device_group_id and
// restricted_times — are mandatory on every PUT (omission → HTTP 500 / 400,
// wire-probed 2026-06-10) and are always taken from the plan, never from
// current.
//
// For each owned Optional+Computed field, a known plan value (the user
// declared it, or UseStateForUnknown carried the prior state in on update) is
// sent; a null/unknown plan value (a field omitted on first create, where
// there is no prior state) falls back to the value read from the live settings
// (current). This is what makes "omit = preserve" hold on create too: the
// singleton always exists, so the first apply adopts the existing values
// rather than letting the full-replace write reset the undeclared ones
// (safelistedApps → [], the bools → false). On update, current's owned fields
// are effectively never consulted — UseStateForUnknown has already made every
// omitted field a known prior value.
//
// AllowTemplates is the exception: it is NOT in the schema (UI-absent —
// maintainer decision) and is ALWAYS carried from current, verbatim, on both
// Create and Update — the §768.3 round-trip of a non-owned field through a
// full-replace PUT. Without it, every Terraform write would reset the stored
// value to the server default (true).
func buildParentSettingsInput(ctx context.Context, plan JamfParentSettingsResourceModel, current *pro.ParentApp) (*pro.ParentApp, diag.Diagnostics) {
	var diags diag.Diagnostics

	restrictedTimes, rtDiags := expandRestrictedTimes(ctx, plan.RestrictedTimes)
	diags.Append(rtDiags...)
	if diags.HasError() {
		return nil, diags
	}

	input := &pro.ParentApp{
		TimezoneID:                    plan.Timezone.ValueString(),
		DeviceGroupID:                 int(plan.DeviceGroupID.ValueInt64()),
		RestrictedTimes:               restrictedTimes,
		IsEnabled:                     boolOrCurrent(plan.Enabled, current, func(c *pro.ParentApp) bool { return c.IsEnabled }),
		AllowClearPasscode:            boolPointerOrCurrent(plan.AllowClearPasscode, current, func(c *pro.ParentApp) *bool { return c.AllowClearPasscode }),
		DisassociateOnWipeAndReEnroll: boolPointerOrCurrent(plan.RevokeOnWipeAndReEnroll, current, func(c *pro.ParentApp) *bool { return c.DisassociateOnWipeAndReEnroll }),
	}

	// allowTemplates: always round-trip the live pointer unchanged (§768.3).
	// Nil only if the GET itself omitted the field — then omitempty drops it
	// and the server applies its default, which is the best available echo.
	if current != nil {
		input.AllowTemplates = current.AllowTemplates
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
	case current != nil && current.SafelistedApps != nil:
		// Omitted on create: re-emit the live collection verbatim so the
		// full-replace write does not clear it to [].
		apps := make([]pro.SafelistedApp, len(*current.SafelistedApps))
		copy(apps, *current.SafelistedApps)
		input.SafelistedApps = &apps
	}

	return input, diags
}

// expandRestrictedTimes converts the restricted_times plan map into the wire
// map. The attribute is Required, so the plan value is always known; an empty
// plan map yields a non-nil empty wire map — `restrictedTimes` carries no
// omitempty, and the server requires the field on every PUT (`{}` is the
// valid "no restrictions" shape, a JSON null is not). Both times are Required
// per entry, so the pointers are always populated.
func expandRestrictedTimes(ctx context.Context, planMap types.Map) (map[string]pro.TimeFrame, diag.Diagnostics) {
	var diags diag.Diagnostics

	var models map[string]restrictedTimeModel
	diags.Append(planMap.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil, diags
	}

	out := make(map[string]pro.TimeFrame, len(models))
	for day, m := range models {
		begin := m.BeginTime.ValueString()
		end := m.EndTime.ValueString()
		out[day] = pro.TimeFrame{BeginTime: &begin, EndTime: &end}
	}
	return out, diags
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried),
// else falls back to the live value read from the server (preserve undeclared
// on create). False when there is no merge base at all (defensive — the CRUD
// handlers always supply current).
func boolOrCurrent(v types.Bool, current *pro.ParentApp, get func(*pro.ParentApp) bool) bool {
	if p := helpers.OptionalBoolPointer(v); p != nil {
		return *p
	}
	if current == nil {
		return false
	}
	return get(current)
}

// boolPointerOrCurrent mirrors boolOrCurrent for the *bool request fields: a
// known plan value wins, else the live pointer is passed through verbatim
// (nil only if the GET omitted the field — then omitempty drops it).
func boolPointerOrCurrent(v types.Bool, current *pro.ParentApp, get func(*pro.ParentApp) *bool) *bool {
	if p := helpers.OptionalBoolPointer(v); p != nil {
		return p
	}
	if current == nil {
		return nil
	}
	return get(current)
}
