// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the user extension attribute
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateUserExtensionAttributeByID",
	"GetUserExtensionAttributeByID",
	"UpdateUserExtensionAttributeByID",
	"DeleteUserExtensionAttributeByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user extension attribute resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the data source's Read path calls
// that map to a privilege. The name-lookup path also calls
// ResolveUserExtensionAttributeByName, a resolver wrapper that is not a
// privilege-bearing registry method (it delegates to the same read endpoint),
// so it is intentionally omitted here.
var dataSourceSDKMethods = []string{
	"GetUserExtensionAttributeByID",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user extension attribute data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource's List path
// calls: the list endpoint plus the per-item hydration GET issued when
// include_resource is requested.
var listResourceSDKMethods = []string{
	"ListUserExtensionAttributes",
	"GetUserExtensionAttributeByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user extension attribute list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
