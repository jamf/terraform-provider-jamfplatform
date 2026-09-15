// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the mobile device prestage
// enrollment resource's CRUD path calls (crud.go). It mirrors the "SDK
// endpoints used" block in crud.go and drives the "Required Jamf permissions"
// table appended to the resource MarkdownDescription. permissions_test.go
// asserts this list stays in sync with the actual client.<Method> calls in
// crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateMobileDevicePrestageV3",
	"GetMobileDevicePrestageV3",
	"UpdateMobileDevicePrestageV3",
	"DeleteMobileDevicePrestageV3",
	"GetMobileDevicePrestageScopeV2",
	"ReplaceMobileDevicePrestageScopeV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-known SDK methods the data source
// (data_source.go) calls. The name-lookup path also calls
// ResolveMobileDevicePrestageV3IDByName, a resolver wrapper that is not a
// registry entry; its required privilege (read) is already covered by
// GetMobileDevicePrestageV3, so the rendered table is complete with the GET
// alone.
var dataSourceSDKMethods = []string{
	"GetMobileDevicePrestageV3",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource
// (list_resource.go) calls.
var listResourceSDKMethods = []string{
	"ListMobileDevicePrestagesV3",
	"GetMobileDevicePrestageScopeV2",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the list resource, appended to its top-level Description.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
