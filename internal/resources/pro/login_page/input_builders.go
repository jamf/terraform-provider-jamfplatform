// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package login_page

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildLoginPageSettingsInput converts the Terraform plan model into a login-customization
// full-replace PUT payload.
//
// Wire-probed 2026-06-09: the three text fields (heading, main, action) are required and
// non-empty on EVERY PUT, regardless of include_custom_disclaimer — omitting any of them,
// or sending an empty string, returns HTTP 400 "field is required" (probes 1/3a/3b in
// spike/LOGIN_CUSTOMIZATION_SPIKE.md). The PUT type's `omitempty` json tags are misleading.
// So every field is always emitted (full-replace, can't-omit → retain via always-emit).
//
// All four fields are Optional+Computed with UseStateForUnknown. For each, a known plan
// value (declared, or USFU-carried on update) is sent; an unknown/null plan value (a field
// omitted on first create, where there is no prior state) falls back to the value read from
// the live settings (current) — adopting the existing singleton rather than failing. The
// live GET always carries non-empty text (the server requires it), so the adopted fallback
// is always a valid non-empty value. On update current is nil — UseStateForUnknown has
// already made every omitted field a known prior value, so the fallback is never consulted.
func buildLoginPageSettingsInput(plan LoginPageSettingsResourceModel, current *pro.LoginContent) *pro.LoginContentPut {
	return &pro.LoginContentPut{
		IncludeCustomDisclaimer: boolOrCurrent(plan.IncludeCustomDisclaimer, currentBool(current, func(c *pro.LoginContent) bool { return c.IncludeCustomDisclaimer })),
		DisclaimerHeading:       stringOrCurrent(plan.DisclaimerHeading, currentString(current, func(c *pro.LoginContent) string { return c.DisclaimerHeading })),
		DisclaimerMainText:      stringOrCurrent(plan.DisclaimerMainText, currentString(current, func(c *pro.LoginContent) string { return c.DisclaimerMainText })),
		ActionText:              stringOrCurrent(plan.ActionText, currentString(current, func(c *pro.LoginContent) string { return c.ActionText })),
	}
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried on update), else
// falls back to the live value read from the server (adopt undeclared toggle on create).
// The wire field is a non-pointer bool, so the return is a plain bool.
func boolOrCurrent(v types.Bool, current bool) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	return current
}

// stringOrCurrent emits the plan value when known, else falls back to the live value read
// from the server. The PUT field is *string; this always returns a non-nil pointer because
// the server requires all three text fields on every write (always-emit, never omit).
func stringOrCurrent(v types.String, current string) *string {
	if !v.IsNull() && !v.IsUnknown() {
		s := v.ValueString()
		return &s
	}
	return &current
}

// currentBool safely extracts a bool field from a possibly-nil current read. A nil read
// (update path) yields false, but on update every plan value is known via UseStateForUnknown
// so this fallback is never consulted.
func currentBool(current *pro.LoginContent, get func(*pro.LoginContent) bool) bool {
	if current == nil {
		return false
	}
	return get(current)
}

// currentString safely extracts a string field from a possibly-nil current read.
func currentString(current *pro.LoginContent, get func(*pro.LoginContent) string) string {
	if current == nil {
		return ""
	}
	return get(current)
}
