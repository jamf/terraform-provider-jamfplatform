// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdm_profile_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignMDMProfileSettingsResourceModel populates a resource model from an SDK response.
//
// Every attribute is Optional+Computed, so committed state must always hold a concrete
// value (Optional+Computed fields cannot be null/unknown after apply). The SDK type uses
// pointers because the JSON wire format carries `omitempty`; in practice the Jamf Pro API
// echoes all six fields on every successful GET. The nil branches below are defensive
// fallbacks only — `false` for the bools (assumes the renewal behaviour is off) and `0`
// for the day limits.
func assignMDMProfileSettingsResourceModel(state *MDMProfileSettingsResourceModel, s *pro.DeviceCommunicationSettings) {
	state.AutoRenewComputerProfileWhenCaRenewed = boolFromPointer(s.AutoRenewComputerMDMProfileWhenCaRenewed)
	state.AutoRenewComputerProfileBeforeExpiry = boolFromPointer(s.AutoRenewComputerMDMProfileWhenDeviceIdentityCertExpiring)
	state.ComputerProfileExpirationLimitDays = int64FromPointer(s.MDMProfileComputerExpirationLimitInDays)
	state.AutoRenewMobileDeviceProfileWhenCaRenewed = boolFromPointer(s.AutoRenewMobileDeviceMDMProfileWhenCaRenewed)
	state.AutoRenewMobileDeviceProfileBeforeExpiry = boolFromPointer(s.AutoRenewMobileDeviceMDMProfileWhenDeviceIdentityCertExpiring)
	state.MobileDeviceProfileExpirationLimitDays = int64FromPointer(s.MDMProfileMobileDeviceExpirationLimitInDays)
}

// assignMDMProfileSettingsDataSourceModel populates a data source model from an
// SDK response. Same nil-fallback semantics as the resource assigner; see that
// function's comment for the rationale.
func assignMDMProfileSettingsDataSourceModel(state *MDMProfileSettingsDataSourceModel, s *pro.DeviceCommunicationSettings) {
	state.AutoRenewComputerProfileWhenCaRenewed = boolFromPointer(s.AutoRenewComputerMDMProfileWhenCaRenewed)
	state.AutoRenewComputerProfileBeforeExpiry = boolFromPointer(s.AutoRenewComputerMDMProfileWhenDeviceIdentityCertExpiring)
	state.ComputerProfileExpirationLimitDays = int64FromPointer(s.MDMProfileComputerExpirationLimitInDays)
	state.AutoRenewMobileDeviceProfileWhenCaRenewed = boolFromPointer(s.AutoRenewMobileDeviceMDMProfileWhenCaRenewed)
	state.AutoRenewMobileDeviceProfileBeforeExpiry = boolFromPointer(s.AutoRenewMobileDeviceMDMProfileWhenDeviceIdentityCertExpiring)
	state.MobileDeviceProfileExpirationLimitDays = int64FromPointer(s.MDMProfileMobileDeviceExpirationLimitInDays)
}

// boolFromPointer returns a concrete Bool value, defaulting a nil pointer to false.
func boolFromPointer(p *bool) types.Bool {
	if p != nil {
		return types.BoolValue(*p)
	}
	return types.BoolValue(false)
}

// int64FromPointer returns a concrete Int64 value, defaulting a nil pointer to 0.
func int64FromPointer(p *int) types.Int64 {
	if p != nil {
		return types.Int64Value(int64(*p))
	}
	return types.Int64Value(0)
}
