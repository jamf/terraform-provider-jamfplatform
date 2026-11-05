// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import "github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

// The skipSetupItems attribute ⇔ wire-key mapping is the skipItems table in skip_items.go,
// which drives the schema, buildSkipSetupItemsMap, flattenSkipSetupItems and
// diffSkipSetupItems, so a pane is added or renamed in one row. The wire keys are the
// mixed-case identifiers Jamf Pro echoes back in the `skipSetupItems` map, taken from live
// GETs on 2026-11-05; TestSkipItems_CoverTheLiveWireKeys pins them.

// Enum value sets (per OpenAPI spec + live wire-probe, §F14). Kept in one
// place so schema validators and tests share the source of truth.
var (
	// prestageMinimumOsTargetVersionValues is shared by the iOS and iPadOS
	// min-OS enforcement enums, which the API declares identically. Keyed on
	// the iOS vocabulary; TestMinimumOsVocabulariesAgree fails if a future SDK
	// release stops the two agreeing, since one shared var could then only be
	// right for one of them.
	prestageMinimumOsTargetVersionValues = pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIosValues()

	// assignNamesUsingValues are the UI-label strings that ARE the wire
	// values for `names.assignNamesUsing` (§4.2). The spec models the field as a
	// plain string and generates no enum, so these stay literals — asserted per
	// value in TestEnumLiteralsComeFromTheSDK.
	assignNamesUsingValues = []string{
		"Default Names",
		"List of Names",
		"Serial Numbers",
		"Single Name",
	}
)

// Sentinel constants for nested block IDs / "none" values.
const (
	// sentinelNestedIDForCreate is the literal "-1" Jamf Pro requires on
	// POST nested blocks (locationInformation.id, purchasingInformation.id)
	// to signal "create a new server-side record" (§F1).
	sentinelNestedIDForCreate = "-1"

	// sentinelNoneIDDash1 is Jamf Pro's "none" sentinel for several optional
	// ID fields (enrollmentSiteId, rtsConfigProfileId, location.buildingId,
	// location.departmentId when unset).
	sentinelNoneIDDash1 = "-1"

	// sentinelDateUnset is the wire-format Jamf returns for an unset
	// purchasing date field.
	sentinelDateUnset = "1970-01-01"

	// sentinelNameIDForCreate is sent for every prestage_device_names
	// element id on a fresh create / for a not-yet-server-assigned name —
	// omitting `id` silently rolls back the whole `names` mutation (§F4b).
	sentinelNameIDForCreate = "-1"

	// minStorageQuotaMegabytes is the server-enforced POST floor for
	// storageQuotaSizeMegabytes (§F3) — Jamf Pro rejects a smaller value on
	// create even when useStorageQuotaSize is false.
	minStorageQuotaMegabytes = 1024

	// defaultAssignNamesUsing is the safe default emitted in a synthesized
	// `names` object when the user omits the block — an empty names:{} on
	// POST/PUT triggers a server 500 (§F2).
	defaultAssignNamesUsing = "Serial Numbers"
)
