// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the mobile device invitation
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry. (The endpoint is create + delete only; the server rejects
// PUT, so no update method is called.)
var resourceSDKMethods = []string{
	"CreateMobileDeviceInvitationByID",
	"GetMobileDeviceInvitationByID",
	"DeleteMobileDeviceInvitationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device invitation resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the mobile device invitation data
// source calls. Selection is by numeric id OR by the server-minted invitation
// code, so both lookups are documented.
var dataSourceSDKMethods = []string{
	"GetMobileDeviceInvitationByID",
	"GetMobileDeviceInvitationByInvitation",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device invitation data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the mobile device invitation
// list resource calls: the list endpoint plus a GET-by-id re-fetch when the
// full record is requested.
var listResourceSDKMethods = []string{
	"ListMobileDeviceInvitations",
	"GetMobileDeviceInvitationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device invitation list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
