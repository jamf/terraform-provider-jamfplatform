// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the user group resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateUserGroupByID",
	"GetUserGroupByID",
	"UpdateUserGroupByID",
	"DeleteUserGroupByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user group resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular user group data
// source calls (ID or name lookup).
var dataSourceSDKMethods = []string{
	"GetUserGroupByID",
	"GetUserGroupByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular user group data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural user groups data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListUserGroups",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural user groups data source.
var pluralDataSourcePrivileges = permissions.Section(proclassic.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the user group list resource
// calls: the list itself plus a per-item GET to hydrate on include_resource.
var listResourceSDKMethods = []string{
	"ListUserGroups",
	"GetUserGroupByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user group list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
