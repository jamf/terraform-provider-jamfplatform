// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_check_in_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildComputerCheckInSettingsInput converts the Terraform plan model into an SDK
// payload. The Client Check-In API is a full-replace PUT, so every field is emitted
// on every write.
//
// The eight startup/login toggles are Optional+Computed with UseStateForUnknown:
// on update an omitted toggle is a known prior value (preserved). On first create
// there is no prior state, so a `current` merge base — the live settings read in
// Create — supplies the value for any toggle the user omitted, so the singleton is
// adopted rather than reset to `false`. On update current is nil (USFU already
// filled the plan). check_in_frequency is Required, so it is always sent from plan.
func buildComputerCheckInSettingsInput(plan ComputerCheckInSettingsResourceModel, current *pro.ClientCheckInV3) *pro.ClientCheckInV3 {
	checkInFrequency := int(plan.CheckInFrequency.ValueInt64())

	return &pro.ClientCheckInV3{
		CheckInFrequency:                 &checkInFrequency,
		CreateStartupScript:              boolOrCurrent(plan.CreateStartupScript, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.CreateStartupScript })),
		StartupLog:                       boolOrCurrent(plan.StartupLog, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.StartupLog })),
		StartupPolicies:                  boolOrCurrent(plan.StartupPolicies, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.StartupPolicies })),
		StartupSsh:                       boolOrCurrent(plan.StartupSsh, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.StartupSsh })),
		CreateHooks:                      boolOrCurrent(plan.CreateLoginHook, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.CreateHooks })),
		HookLog:                          boolOrCurrent(plan.LoginHookLog, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.HookLog })),
		HookPolicies:                     boolOrCurrent(plan.LoginHookPolicies, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.HookPolicies })),
		EnableLocalConfigurationProfiles: boolOrCurrent(plan.AllowNetworkStateChangeTriggers, currentBool(current, func(c *pro.ClientCheckInV3) *bool { return c.EnableLocalConfigurationProfiles })),
	}
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried on
// update), else falls back to the live value read from the server (preserve
// undeclared toggles on first create).
func boolOrCurrent(v types.Bool, current *bool) *bool {
	if !v.IsNull() && !v.IsUnknown() {
		b := v.ValueBool()
		return &b
	}
	return current
}

// currentBool safely extracts a *bool field from a possibly-nil current read.
func currentBool(current *pro.ClientCheckInV3, get func(*pro.ClientCheckInV3) *bool) *bool {
	if current == nil {
		return nil
	}
	return get(current)
}
