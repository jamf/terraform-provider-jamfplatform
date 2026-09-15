// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// accountGroupTimeoutAttributeTypes defines the timeout attribute types for the
// account group resource operations.
var accountGroupTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// accessLevelValues are the classic access_level enum values for a group.
// UI: "Access Level".
//
// Deliberately narrower than proclassic.GroupAccessLevelValues(), which also
// generates "Group Access": a group cannot itself be group-scoped. The set is
// curated; the spellings are the SDK's. Unlike jamfplatform_pro_account, this
// resource is classic end to end, so these labels are the wire vocabulary and
// aliasing them ties the schema to the endpoint it actually writes.
var accessLevelValues = []string{
	proclassic.GroupAccessLevelFullAccess,
	proclassic.GroupAccessLevelSiteAccess,
}

// privilegeSetValues are the classic privilege_set enum values. UI: "Privilege Set".
var privilegeSetValues = proclassic.GroupPrivilegeSetValues()
