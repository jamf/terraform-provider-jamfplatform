// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the mobile device extension
// attribute resource's CRUD path calls (crud.go). It mirrors the "SDK endpoints
// used" block in crud.go and drives the "Required Jamf permissions" table
// appended to the resource MarkdownDescription. permissions_test.go asserts
// this list stays in sync with the actual client.<Method> calls in crud.go and
// with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateMobileDeviceExtensionAttributeV1",
	"GetMobileDeviceExtensionAttributeV1",
	"UpdateMobileDeviceExtensionAttributeV1",
	"DeleteMobileDeviceExtensionAttributeV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device extension attribute resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the mobile device extension
// attribute data source calls (data_source.go). The name-lookup path resolves
// via ResolveMobileDeviceExtensionAttributeV1ByName, a synthetic resolver
// absent from the SDK privilege registry; the privilege it requires
// (extension-attributes:read) is already covered by
// GetMobileDeviceExtensionAttributeV1.
var dataSourceSDKMethods = []string{
	"GetMobileDeviceExtensionAttributeV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device extension attribute data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the mobile device extension
// attribute list resource calls (list_resource.go).
var listResourceSDKMethods = []string{
	"ListMobileDeviceExtensionAttributesV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device extension attribute list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
