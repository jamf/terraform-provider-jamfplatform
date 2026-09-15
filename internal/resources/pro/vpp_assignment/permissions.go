// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the VPP assignment resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateVPPAssignmentByID",
	"GetVPPAssignmentByID",
	"UpdateVPPAssignmentByID",
	"DeleteVPPAssignmentByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the VPP assignment resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the VPP assignment data source's
// Read path calls (name resolution via list, then a get-by-id).
var dataSourceSDKMethods = []string{
	"ListVPPAssignments",
	"GetVPPAssignmentByID",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the VPP assignment data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the VPP assignment list
// resource's List path calls (list, then optional per-item get-by-id when
// IncludeResource is requested).
var listResourceSDKMethods = []string{
	"ListVPPAssignments",
	"GetVPPAssignmentByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the VPP assignment list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
