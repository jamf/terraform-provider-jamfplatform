// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// mobileDeviceExtensionAttributeTimeoutAttributeTypes defines the timeout
// attribute types for the resource operations.
var mobileDeviceExtensionAttributeTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// Enum value sets, taken from the SDK's generated helpers so the OneOf
// validators cannot drift from the API. A live-tenant probe (2026-06-02)
// agreed with the spec on all three.
//
// The two ways mobile EAs differ from computer EAs are already encoded in the
// generated sets, so there is nothing to narrow by hand: mobile devices cannot
// run scripts, so the input-type set has no SCRIPT, and OPERATING_SYSTEM is not
// a mobile inventory display tab.
var (
	// validDataTypes is the "Data type" dropdown: String / Integer / Date.
	validDataTypes = pro.MobileDeviceExtensionAttributesDataTypeValues()

	// validInputTypes is the "Input type" set.
	validInputTypes = pro.MobileDeviceExtensionAttributesInputTypeValues()

	// validInventoryDisplays is the "Inventory display" dropdown (5 tabs).
	validInventoryDisplays = pro.MobileDeviceExtensionAttributesInventoryDisplayTypeValues()
)

// Input-type discriminator constants.
const (
	inputTypeText  = pro.MobileDeviceExtensionAttributesInputTypeText
	inputTypePopup = pro.MobileDeviceExtensionAttributesInputTypePopup
	inputTypeLDAP  = pro.MobileDeviceExtensionAttributesInputTypeDirectoryServiceAttributeMapping
)
