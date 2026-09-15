// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_plus_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildSelfServicePlusSettingsInput converts the Terraform plan model into an SDK payload.
func buildSelfServicePlusSettingsInput(plan SelfServicePlusSettingsResourceModel) *pro.SelfServicePlusSettings {
	enabled := plan.Enabled.ValueBool()
	return &pro.SelfServicePlusSettings{
		Enabled: &enabled,
	}
}
