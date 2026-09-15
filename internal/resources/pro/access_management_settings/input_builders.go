// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package access_management_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildAccessManagementSettingsInput converts the Terraform plan model into an SDK
// payload for the full-replace POST (/v4/enrollment/access-management).
//
// The single field is Optional+Computed with UseStateForUnknown. On update an omitted
// field is a known prior value (preserved). On first create there is no prior state, so
// a `current` merge base — the live setting read in Create — supplies the value when the
// user omitted it, so the singleton is adopted rather than cleared. On update current is
// nil (USFU already filled the plan).
//
// The field is ALWAYS emitted as a concrete string (never omitted from the body): the
// POST is a "configure" call, so we never send an empty `{}` body of unknown semantics.
// An empty string ("") is the wire representation of "no ADE server configured" (a fresh
// tenant echoes `{"automatedDeviceEnrollmentServerUuid": ""}`), so emitting "" both
// clears the setting and round-trips cleanly.
func buildAccessManagementSettingsInput(plan AccessManagementSettingsResourceModel, current *pro.AccessManagementSetting) *pro.AccessManagementSetting {
	uuid := stringOrCurrent(plan.AutomatedDeviceEnrollmentServerUUID, current)
	return &pro.AccessManagementSetting{
		AutomatedDeviceEnrollmentServerUUID: &uuid,
	}
}

// stringOrCurrent returns the plan value when known (declared, or USFU-carried on
// update), else the live value read from the server (adopt undeclared value on first
// create), else "" (always emit a concrete string). A pointer to "" serialises as
// `""` — `omitempty` only drops nil pointers, not pointers to empty strings — so the
// clear/empty case is sent verbatim.
func stringOrCurrent(v types.String, current *pro.AccessManagementSetting) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	if current != nil && current.AutomatedDeviceEnrollmentServerUUID != nil {
		return *current.AutomatedDeviceEnrollmentServerUUID
	}
	return ""
}
