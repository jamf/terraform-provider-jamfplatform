// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// computerExtensionAttributeTimeoutAttributeTypes defines the timeout attribute
// types for the resource operations.
var computerExtensionAttributeTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// Enum value sets, taken from the SDK's generated helpers so the OneOf
// validators cannot drift from the API. A live-tenant probe (2026-06-02)
// agreed with the spec on all four sets, and Jamf rejects an out-of-set value
// with 400.
//
// The plural ComputerExtensionAttributes* vocabularies are the right ones
// here: the singular ComputerExtensionAttribute* types belong to the computer
// *inventory* read and differ — DATE_TIME rather than DATE, LDAP rather than
// DIRECTORY_SERVICE_ATTRIBUTE_MAPPING — so keying on those would put values on
// the wire that this endpoint refuses.
var (
	// validDataTypes is the "Data type" dropdown: String / Integer / Date.
	validDataTypes = pro.ComputerExtensionAttributesDataTypeValues()

	// validInputTypes is the "Input type" set. The modern admin UI offers only
	// TEXT / POPUP / SCRIPT for new computer EAs, but the API accepts and
	// round-trips DIRECTORY_SERVICE_ATTRIBUTE_MAPPING and existing LDAP-mapped
	// EAs use it, so the full generated set is kept so such EAs can be imported
	// and managed.
	validInputTypes = pro.ComputerExtensionAttributesInputTypeValues()

	// validManageExistingData is the accepted set for manage_existing_data
	// (wire-probed: the Pro PUT for a SCRIPT EA requires DELETE or RETAIN).
	validManageExistingData = pro.ComputerExtensionAttributesManageExistingDataValues()

	// validInventoryDisplays is the "Inventory display" dropdown (6 tabs).
	validInventoryDisplays = pro.ComputerExtensionAttributesInventoryDisplayTypeValues()
)

// manageExistingDataDefault is sent when a SCRIPT EA update disables the EA and
// the user omitted manage_existing_data — RETAIN preserves already-collected
// inventory data, the safe default. See manageExistingDataFor for when the
// field is sent at all.
const manageExistingDataDefault = pro.ComputerExtensionAttributesManageExistingDataRetain

// Input-type discriminator constants.
const (
	inputTypeText   = pro.ComputerExtensionAttributesInputTypeText
	inputTypePopup  = pro.ComputerExtensionAttributesInputTypePopup
	inputTypeScript = pro.ComputerExtensionAttributesInputTypeScript
	inputTypeLDAP   = pro.ComputerExtensionAttributesInputTypeDirectoryServiceAttributeMapping
)
