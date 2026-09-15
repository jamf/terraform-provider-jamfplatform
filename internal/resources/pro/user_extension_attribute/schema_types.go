// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// userExtensionAttributeTimeoutAttributeTypes defines the timeout attribute
// types for the resource operations.
var userExtensionAttributeTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// Enum value sets, taken from the SDK's generated user-EA vocabularies. The
// Classic API is permissive (a live-tenant probe on 2026-06-02 accepted
// out-of-set values), but the admin UI offers only these, so OneOf validators
// constrain to the UI-valid set. Classic strings are human-cased, not
// SCREAMING_SNAKE.
//
// These are the UserExtensionAttribute* vocabularies specifically. The
// computer and mobile-device classic EA types spell the same data types
// identically but carry a third input type each (LDAP Mapping / LDAP Attribute
// Mapping) that the user-EA payload has no field for — the server silently
// coerces it to Text Field — so the generated user-EA set is already exactly
// the UI-valid one and nothing needs narrowing.
var (
	// validDataTypes is the "Data Type" dropdown: String / Integer / Date.
	validDataTypes = proclassic.UserExtensionAttributeDataTypeValues()

	// validInputTypes is the "Input Type" dropdown: Text Field / Pop-up Menu.
	validInputTypes = proclassic.UserExtensionAttributeInputTypeTypeValues()
)

// Input-type discriminator constants.
const (
	inputTypeTextField = proclassic.UserExtensionAttributeInputTypeTypeTextField
	inputTypePopupMenu = proclassic.UserExtensionAttributeInputTypeTypePopUpMenu
)
