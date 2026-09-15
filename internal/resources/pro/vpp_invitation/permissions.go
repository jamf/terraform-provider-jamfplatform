// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the VPP invitation resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateVPPInvitationByID",
	"GetVPPInvitationByID",
	"UpdateVPPInvitationByID",
	"DeleteVPPInvitationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the VPP invitation resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the VPP invitation data source
// calls (lookup by id, or list-then-filter for lookup by name).
var dataSourceSDKMethods = []string{
	"GetVPPInvitationByID",
	"ListVPPInvitations",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the VPP invitation data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the VPP invitation list resource
// calls — the full list, plus a per-item GET when IncludeResource is requested.
var listResourceSDKMethods = []string{
	"ListVPPInvitations",
	"GetVPPInvitationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the VPP invitation list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
