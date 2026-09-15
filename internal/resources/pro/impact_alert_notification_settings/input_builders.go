// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact_alert_notification_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildImpactAlertNotificationSettingsInput converts the Terraform plan model into an SDK
// payload. The Impact Alert Notification API is a full-replace PUT and every field is a
// non-pointer bool, so all four toggles are emitted on every write.
//
// The four toggles are Optional+Computed with UseStateForUnknown: on update an omitted
// toggle is a known prior value (preserved). On first create there is no prior state, so
// a `current` merge base — the live settings read in Create — supplies the value for any
// toggle the user omitted, so the singleton is adopted rather than reset to `false`. On
// update current is nil (USFU already filled the plan).
func buildImpactAlertNotificationSettingsInput(plan ImpactAlertNotificationSettingsResourceModel, current *pro.ImpactAlertNotificationSettingsV1) *pro.ImpactAlertNotificationSettingsV1 {
	return &pro.ImpactAlertNotificationSettingsV1{
		DeployableObjectsAlertEnabled:            boolOrCurrent(plan.DeployableObjectsAlertEnabled, currentBool(current, func(c *pro.ImpactAlertNotificationSettingsV1) bool { return c.DeployableObjectsAlertEnabled })),
		DeployableObjectsConfirmationCodeEnabled: boolOrCurrent(plan.DeployableObjectsConfirmationCodeEnabled, currentBool(current, func(c *pro.ImpactAlertNotificationSettingsV1) bool { return c.DeployableObjectsConfirmationCodeEnabled })),
		ScopeableObjectsAlertEnabled:             boolOrCurrent(plan.ScopeableObjectsAlertEnabled, currentBool(current, func(c *pro.ImpactAlertNotificationSettingsV1) bool { return c.ScopeableObjectsAlertEnabled })),
		ScopeableObjectsConfirmationCodeEnabled:  boolOrCurrent(plan.ScopeableObjectsConfirmationCodeEnabled, currentBool(current, func(c *pro.ImpactAlertNotificationSettingsV1) bool { return c.ScopeableObjectsConfirmationCodeEnabled })),
	}
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried on update),
// else falls back to the live value read from the server (preserve undeclared toggles on
// first create). The wire field is a non-pointer bool, so the return is a plain bool.
func boolOrCurrent(v types.Bool, current bool) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	return current
}

// currentBool safely extracts a bool field from a possibly-nil current read. A nil read
// (update path, where no merge base is supplied) yields false, but on update every plan
// toggle is known via UseStateForUnknown so this fallback is never consulted.
func currentBool(current *pro.ImpactAlertNotificationSettingsV1, get func(*pro.ImpactAlertNotificationSettingsV1) bool) bool {
	if current == nil {
		return false
	}
	return get(current)
}
