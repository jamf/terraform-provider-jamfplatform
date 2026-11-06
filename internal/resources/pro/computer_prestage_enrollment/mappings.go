// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import "github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

// The skipSetupItems attribute ⇔ wire-key mapping is the skipItems table in skip_items.go,
// which drives the schema, buildSkipSetupItemsMap, flattenSkipSetupItems and
// diffSkipSetupItems, so a pane is added or renamed in one row. The wire keys are the
// mixed-case identifiers Jamf Pro echoes back in the `skipSetupItems` map, taken from live
// GETs on 2026-11-05; TestSkipItems_CoverTheLiveWireKeys pins them.

// Enum value sets, taken from the SDK's generated helpers where they exist so
// the schema validators cannot drift from the API. Kept in one place so the
// validators and the tests share the source of truth.
//
// prefillTypeValues is the exception and stays a literal pair: the spec
// documents the vocabulary in prose only — pro/types.go says "Values accepted
// are only CUSTOM and DEVICE_OWNER" above the PrefillType field — and generates
// no enum for it, so there is nothing to alias. Asserted per value in
// TestEnumLiteralsComeFromTheSDK.
var (
	recoveryLockPasswordTypeValues       = pro.ComputerPrestageV3RecoveryLockPasswordTypeValues()
	prestageMinimumOsTargetVersionValues = pro.ComputerPrestageV3PrestageMinimumOsTargetVersionTypeValues()
	userAccountTypeValues                = pro.AccountSettingsRequestUserAccountTypeValues()
	prefillTypeValues                    = []string{"CUSTOM", "DEVICE_OWNER"}
)

// Sentinel constants for nested block IDs / "none" values.
const (
	// sentinelNestedIDForCreate is the literal "-1" Jamf Pro requires on
	// POST nested blocks (locationInformation.id, purchasingInformation.id)
	// to signal "create a new server-side record". Verified by live wire
	// probe — POST with "0" returns 400 INVALID_ID.
	sentinelNestedIDForCreate = "-1"

	// sentinelNoneIDDash1 is Jamf Pro's "none" sentinel for several
	// optional ID fields (siteId, enrollmentSiteId, pssoConfigProfileId,
	// location.buildingId, location.departmentId,
	// customPackageDistributionPointId when unset).
	sentinelNoneIDDash1 = "-1"

	// sentinelDateUnset is the wire-format Jamf returns for an unset
	// purchasing date field.
	sentinelDateUnset = "1970-01-01"
)
