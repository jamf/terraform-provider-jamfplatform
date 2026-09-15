// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the device group resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateDeviceGroupV1",
	"GetDeviceGroupV1",
	"UpdateDeviceGroupV2",
	"DeleteDeviceGroupV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the device group resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular device group data
// source calls: the id lookup reads one group, and the name lookup matches over
// the group list locally rather than through a synthetic resolver — see the
// "Deliberately not used" block in crud.go.
var dataSourceSDKMethods = []string{
	"GetDeviceGroupV1",
	"ListDeviceGroupsV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular device group data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural device groups data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListDeviceGroupsV2",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural device groups data source.
var pluralDataSourcePrivileges = permissions.Section(securitycloud.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the device group list resource
// calls.
var listResourceSDKMethods = []string{
	"ListDeviceGroupsV2",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device group list resource.
var listResourcePrivileges = permissions.Section(securitycloud.Privileges, listResourceSDKMethods...)
