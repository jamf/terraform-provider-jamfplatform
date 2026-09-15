// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the computer invitation resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry. There is no update endpoint on /computerinvitations, so Update only
// GET-refreshes; the set is create + read + delete.
var resourceSDKMethods = []string{
	"CreateComputerInvitationByID",
	"GetComputerInvitationByID",
	"DeleteComputerInvitationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer invitation resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the computer invitation data
// source calls. Lookup is by numeric id or by invitation code; both are reads.
var dataSourceSDKMethods = []string{
	"GetComputerInvitationByID",
	"GetComputerInvitationByInvitation",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer invitation data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the computer invitation list
// resource calls: the list endpoint plus a per-item GET-by-id used to populate
// the full record when IncludeResource is set.
var listResourceSDKMethods = []string{
	"ListComputerInvitations",
	"GetComputerInvitationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer invitation list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
