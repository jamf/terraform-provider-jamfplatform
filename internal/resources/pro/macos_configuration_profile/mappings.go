// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import "github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

// Level field wire/UI mappings. The Jamf Pro admin UI dropdown for the
// "Level" field offers `Computer Level` and `User Level`. The classic API
// wire is asymmetric: write `Computer` / `User`, read `System` / `User`.
// Translate at the input/output boundary per STYLE_GUIDE §"Asymmetric server
// normalisation on type-style discriminator fields".
//
// 2026-05-24 wire-probe summary:
//   - send `<level>Computer Level</level>` → server stores as User (the
//     long-form spelling is not accepted on write and defaults apply).
//   - send `<level>Computer</level>` → server reads back `<level>System</level>`.
//   - send `<level>User</level>` → server reads back `<level>User</level>`.
const (
	levelUIComputer  = "Computer Level"
	levelUIUser      = "User Level"
	levelWireWriteCC = "Computer" // accepted wire-write value for Computer Level
	levelWireWriteUC = "User"     // accepted wire-write value for User Level
	levelWireReadCC  = "System"   // wire-read form for Computer Level
	levelWireReadUC  = "User"     // wire-read form for User Level (symmetric)
)

// The four level values above stay literals deliberately, and not because the
// SDK lacks a vocabulary: proclassic.OsXConfigurationProfileGeneralLevel
// generates "computer" and "user". The wire probe recorded above found neither
// spelling — the write path accepts "Computer"/"User" and the read path returns
// "System"/"User" — so the generated set disagrees with the endpoint on both
// sides. Aliasing it would put a rejected value on the wire. Flagged upstream as
// a spec defect; asserted per value in TestEnumLiteralsComeFromTheSDK so this
// stops being an exemption the moment the spec is corrected.

// levelToWireWrite translates the TF/UI-facing value (`Computer Level` or
// `User Level`) into the value the classic API write path accepts.
func levelToWireWrite(uiValue string) string {
	switch uiValue {
	case levelUIComputer:
		return levelWireWriteCC
	case levelUIUser:
		return levelWireWriteUC
	}
	return uiValue
}

// levelFromWireRead translates the classic API read-side value into the
// TF/UI-canonical label. `System` ↔ `Computer Level`; `User` ↔ `User Level`.
// Any unrecognised input passes through unchanged so future server values
// surface as drift rather than silent loss.
func levelFromWireRead(wireValue string) string {
	switch wireValue {
	case levelWireReadCC:
		return levelUIComputer
	case levelWireReadUC:
		return levelUIUser
	}
	return wireValue
}

// Distribution method wire values are symmetric — the Jamf Pro admin UI
// labels match the wire spellings exactly. The set is constrained for
// schema validators.
const (
	distributionMethodInstallAutomatically = proclassic.OsXConfigurationProfileGeneralDistributionMethodInstallAutomatically
	distributionMethodMakeAvailableInSS    = proclassic.OsXConfigurationProfileGeneralDistributionMethodMakeAvailableInSelfService
)

// validDistributionMethods is the canonical set accepted by the classic API.
var validDistributionMethods = []string{
	distributionMethodInstallAutomatically,
	distributionMethodMakeAvailableInSS,
}

// validLevels is the canonical set accepted by the resource schema (UI form,
// before levelToWireWrite translation).
var validLevels = []string{
	levelUIComputer,
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

// Notification location is a fixed enum on the wire.
const (
	notificationLocationSelfService          = "Self Service"
	notificationLocationSelfServiceAndCenter = "Self Service and Notification Center"
)

var validNotificationLocations = []string{
	notificationLocationSelfService,
	notificationLocationSelfServiceAndCenter,
}

// Site / Category "no value" sentinel observed on the wire when the user
// has not assigned a site / category. Mirrors policy and other classic
// resources. Retained as documentation; the state builder currently surfaces
// the raw -1 from the wire so users see the same value the Jamf Pro UI
// renders for "no site" / "no category".
