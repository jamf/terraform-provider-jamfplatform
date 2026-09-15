// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdm_profile_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildMDMProfileSettingsInput converts the Terraform plan model into an SDK payload.
//
// The SDK ints are *int but the Terraform model uses Int64; the explicit int() casts
// below bridge the two. Every field is sent as a pointer to the plan value — the Jamf
// Pro PUT echoes the body and a subsequent GET captures authoritative state, so a
// partial payload is harmless.
func buildMDMProfileSettingsInput(plan MDMProfileSettingsResourceModel) *pro.DeviceCommunicationSettings {
	autoRenewComputerWhenCaRenewed := plan.AutoRenewComputerProfileWhenCaRenewed.ValueBool()
	autoRenewComputerBeforeExpiry := plan.AutoRenewComputerProfileBeforeExpiry.ValueBool()
	computerExpirationLimit := int(plan.ComputerProfileExpirationLimitDays.ValueInt64())
	autoRenewMobileWhenCaRenewed := plan.AutoRenewMobileDeviceProfileWhenCaRenewed.ValueBool()
	autoRenewMobileBeforeExpiry := plan.AutoRenewMobileDeviceProfileBeforeExpiry.ValueBool()
	mobileExpirationLimit := int(plan.MobileDeviceProfileExpirationLimitDays.ValueInt64())

	return &pro.DeviceCommunicationSettings{
		AutoRenewComputerMDMProfileWhenCaRenewed:                      &autoRenewComputerWhenCaRenewed,
		AutoRenewComputerMDMProfileWhenDeviceIdentityCertExpiring:     &autoRenewComputerBeforeExpiry,
		MDMProfileComputerExpirationLimitInDays:                       &computerExpirationLimit,
		AutoRenewMobileDeviceMDMProfileWhenCaRenewed:                  &autoRenewMobileWhenCaRenewed,
		AutoRenewMobileDeviceMDMProfileWhenDeviceIdentityCertExpiring: &autoRenewMobileBeforeExpiry,
		MDMProfileMobileDeviceExpirationLimitInDays:                   &mobileExpirationLimit,
	}
}
