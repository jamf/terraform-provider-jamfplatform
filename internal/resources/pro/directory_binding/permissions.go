// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the directory binding resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateDirectoryBindingByID",
	"GetDirectoryBindingByID",
	"UpdateDirectoryBindingByID",
	"DeleteDirectoryBindingByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the directory binding resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the directory binding data source
// calls. The singular data source resolves by ID directly via
// GetDirectoryBindingByID, and by name via lookupByName, which lists with
// ListDirectoryBindings and follows up with GetDirectoryBindingByID.
var dataSourceSDKMethods = []string{
	"GetDirectoryBindingByID",
	"ListDirectoryBindings",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the directory binding data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the directory binding list
// resource calls. It lists with ListDirectoryBindings and, when
// include_resource is true, follows up per item with GetDirectoryBindingByID.
var listResourceSDKMethods = []string{
	"ListDirectoryBindings",
	"GetDirectoryBindingByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the directory binding list resource, appended to its
// schema Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
