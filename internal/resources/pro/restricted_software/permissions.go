// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"slices"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the restricted software resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateRestrictedSoftwareByID",
	"GetRestrictedSoftwareByID",
	"UpdateRestrictedSoftwareByID",
	"DeleteRestrictedSoftwareByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the restricted software resource, appended to its
// MarkdownDescription.
// collisionSDKMethods lists the SDK methods the plan-time name collision check calls. The
// check lists every existing restricted software record once per plan, so the table documents the read it needs.
var collisionSDKMethods = []string{
	"ListRestrictedSoftware",
}

var resourcePrivileges = permissions.Section(proclassic.Privileges, slices.Concat(resourceSDKMethods, collisionSDKMethods)...)

// dataSourceSDKMethods lists the SDK methods the restricted software data
// source calls (ID or name lookup).
var dataSourceSDKMethods = []string{
	"GetRestrictedSoftwareByID",
	"GetRestrictedSoftwareByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the restricted software data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the restricted software list
// resource calls: the list fetch plus the per-item hydration GET used when
// IncludeResource is requested.
var listResourceSDKMethods = []string{
	"ListRestrictedSoftware",
	"GetRestrictedSoftwareByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the restricted software list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
