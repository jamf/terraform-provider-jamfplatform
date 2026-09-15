// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the resource's CRUD path calls. It
// mirrors the client.<Method> calls in crud.go and drives the "Required Jamf
// privileges" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateMobileDeviceConfigurationProfileByID",
	"GetMobileDeviceConfigurationProfileByID",
	"UpdateMobileDeviceConfigurationProfileByID",
	"DeleteMobileDeviceConfigurationProfileByID",
}

// dataSourceSDKMethods lists the SDK methods the data source calls.
var dataSourceSDKMethods = []string{
	"GetMobileDeviceConfigurationProfileByID",
	"GetMobileDeviceConfigurationProfileByName",
}

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListMobileDeviceConfigurationProfiles",
	"GetMobileDeviceConfigurationProfileByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
