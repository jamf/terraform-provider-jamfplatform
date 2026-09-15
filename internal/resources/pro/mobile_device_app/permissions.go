// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the ProClassic SDK methods the mobile device app
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateMobileDeviceApplicationByID",
	"GetMobileDeviceApplicationByID",
	"UpdateMobileDeviceApplicationByID",
	"DeleteMobileDeviceApplicationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device app resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the ProClassic SDK methods the mobile device app
// data source calls (lookup by ID or by exact name).
var dataSourceSDKMethods = []string{
	"GetMobileDeviceApplicationByID",
	"GetMobileDeviceApplicationByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device app data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the ProClassic SDK methods the mobile device app
// list resource calls: the list fetch plus the per-item hydration GET issued
// when IncludeResource is requested (config generation).
var listResourceSDKMethods = []string{
	"ListMobileDeviceApplications",
	"GetMobileDeviceApplicationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the mobile device app list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
