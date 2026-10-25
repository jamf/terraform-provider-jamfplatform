// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"slices"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the dock item resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateDockItemByID",
	"GetDockItemByID",
	"UpdateDockItemByID",
	"DeleteDockItemByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the dock item resource, appended to its MarkdownDescription.
// collisionSDKMethods lists the SDK methods the plan-time name collision check calls. The
// check lists every existing Dock item once per plan, so the table documents the read it needs.
var collisionSDKMethods = []string{
	"ListDockItems",
}

var resourcePrivileges = permissions.Section(proclassic.Privileges, slices.Concat(resourceSDKMethods, collisionSDKMethods)...)

// dataSourceSDKMethods lists the SDK methods the dock item data source's Read
// path calls (lookup by ID or by name). Drives the privileges table appended to
// the data source MarkdownDescription.
var dataSourceSDKMethods = []string{
	"GetDockItemByID",
	"GetDockItemByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the dock item data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the dock item list resource's
// List path calls (a list, plus a per-item GET when IncludeResource is set).
// Drives the privileges table appended to the list resource Description.
var listResourceSDKMethods = []string{
	"ListDockItems",
	"GetDockItemByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the dock item list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
