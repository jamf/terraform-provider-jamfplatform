// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package re_enrollment_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildReenrollmentInput converts the Terraform plan model into a Re-enrollment
// settings full-replace PUT payload, preserving undeclared toggles.
//
// The five clear_* toggles are Optional+Computed. For each, a known plan value
// (the user declared it, or UseStateForUnknown carried the prior state in on
// update) is sent; an unknown/null plan value (a toggle omitted on first create,
// where there is no prior state) falls back to the value read from the live
// settings (current). This is what makes "omit = preserve" hold on create too:
// the singleton always exists, so the first apply adopts the existing toggles
// rather than resetting the undeclared ones to the server default. On update,
// current is nil — UseStateForUnknown has already made every omitted toggle a
// known prior value, so the fallback is never consulted.
//
// FlushMDMQueue (clear_management_history) is a Required enum — always known —
// and the server rejects an empty value, so it is sent verbatim.
func buildReenrollmentInput(plan ReEnrollmentSettingsResourceModel, current *pro.Reenrollment) *pro.Reenrollment {
	return &pro.Reenrollment{
		FlushMDMQueue:                            plan.ClearManagementHistory.ValueString(),
		IsFlushPolicyHistoryEnabled:              boolOrCurrent(plan.ClearPolicyLogs, currentBool(current, func(c *pro.Reenrollment) *bool { return c.IsFlushPolicyHistoryEnabled })),
		IsFlushLocationInformationEnabled:        boolOrCurrent(plan.ClearLocationInformation, currentBool(current, func(c *pro.Reenrollment) *bool { return c.IsFlushLocationInformationEnabled })),
		IsFlushLocationInformationHistoryEnabled: boolOrCurrent(plan.ClearLocationInformationHistory, currentBool(current, func(c *pro.Reenrollment) *bool { return c.IsFlushLocationInformationHistoryEnabled })),
		IsFlushExtensionAttributesEnabled:        boolOrCurrent(plan.ClearExtensionAttributes, currentBool(current, func(c *pro.Reenrollment) *bool { return c.IsFlushExtensionAttributesEnabled })),
		IsFlushSoftwareUpdatePlansEnabled:        boolOrCurrent(plan.ClearSoftwareUpdatePlans, currentBool(current, func(c *pro.Reenrollment) *bool { return c.IsFlushSoftwareUpdatePlansEnabled })),
	}
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried), else
// falls back to the live value read from the server (preserve undeclared on create).
func boolOrCurrent(v types.Bool, current *bool) *bool {
	if p := helpers.OptionalBoolPointer(v); p != nil {
		return p
	}
	return current
}

// currentBool safely extracts a *bool field from a possibly-nil current settings read.
func currentBool(current *pro.Reenrollment, get func(*pro.Reenrollment) *bool) *bool {
	if current == nil {
		return nil
	}
	return get(current)
}
