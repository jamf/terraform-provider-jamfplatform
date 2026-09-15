// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import "github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

// Level field wire/UI mappings. The Jamf Pro admin UI dropdown for the
// "Level" field offers `Device Level` and `User Level`. The classic API
// wire is asymmetric: write `Device` / `User`, read `System` / `User`.
// Translate at the input/output boundary per STYLE_GUIDE §"Asymmetric server
// normalisation on type-style discriminator fields".
//
// 2026-05-25 wire-probe summary (platform-nmartin):
//   - send `<level>Device</level>` → server reads back `<level>System</level>`.
//   - send `<level>User</level>` → server reads back `<level>User</level>`.
//   - send `<level>Device Level</level>` (long form) → reads `System` (accepted but use short form).
//   - send `<level>User Level</level>` (long form) → reads `System` (WRONG — defaults!).
const (
	levelUIDevice = "Device Level"
	levelUIUser   = "User Level"
	// proclassic.MobileDeviceConfigurationProfileGeneralLevel generates the
	// read-side pair (System / User). "Device" is the write-side spelling for
	// Device Level and has no constant; the User Level spelling is symmetric
	// across write and read, so both of its uses alias the same constant.
	levelWireWriteDevice = "Device"                                                    // accepted wire-write value for Device Level
	levelWireWriteUser   = proclassic.MobileDeviceConfigurationProfileGeneralLevelUser // accepted wire-write value for User Level

	levelWireReadDevice = proclassic.MobileDeviceConfigurationProfileGeneralLevelSystem // wire-read form for Device Level
	levelWireReadUser   = proclassic.MobileDeviceConfigurationProfileGeneralLevelUser   // wire-read form for User Level (symmetric)
)

// levelToWireWrite translates the TF/UI-facing value into the Classic API write value.
func levelToWireWrite(uiValue string) string {
	switch uiValue {
	case levelUIDevice:
		return levelWireWriteDevice
	case levelUIUser:
		return levelWireWriteUser
	}
	return uiValue
}

// levelFromWireRead translates the Classic API read-side value into the
// TF/UI-canonical label. Unrecognised inputs pass through so future server
// values surface as drift rather than silent loss.
func levelFromWireRead(wireValue string) string {
	switch wireValue {
	case levelWireReadDevice:
		return levelUIDevice
	case levelWireReadUser:
		return levelUIUser
	}
	return wireValue
}

// Distribution method wire values are symmetric — the admin UI labels match
// the wire spellings exactly. The spec generates this vocabulary under the
// field name deploymentMethod rather than distributionMethod, so the constants
// carry the SDK's DeploymentMethod prefix.
const (
	distributionMethodInstallAutomatically = proclassic.MobileDeviceConfigurationProfileGeneralDeploymentMethodInstallAutomatically
	distributionMethodMakeAvailableInSS    = proclassic.MobileDeviceConfigurationProfileGeneralDeploymentMethodMakeAvailableInSelfService
)

var validDistributionMethods = []string{
	distributionMethodInstallAutomatically,
	distributionMethodMakeAvailableInSS,
}

var validLevels = []string{
	levelUIDevice,
	levelUIUser,
}

// Security removal_disallowed is a fixed enum on the wire.
const (
	removalDisallowedNever             = "Never"
	removalDisallowedAlways            = "Always"
	removalDisallowedWithAuthorization = "With Authorization"
)

var validRemovalDisallowedValues = []string{
	removalDisallowedNever,
	removalDisallowedAlways,
	removalDisallowedWithAuthorization,
}
