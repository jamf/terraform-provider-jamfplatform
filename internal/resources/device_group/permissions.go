// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the device_group resource's CRUD
// path calls. It mirrors the client.<Method> calls in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateDeviceGroup",
	"GetDeviceGroup",
	"UpdateDeviceGroup",
	"DeleteDeviceGroup",
	"ListDeviceGroupMembers",
	"UpdateDeviceGroupMembers",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device_group resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(devicegroups.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the device_group data source's
// Read path calls. A data source documents only the privileges it needs, not
// the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetDeviceGroup",
	"ListDeviceGroupMembers",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device_group data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(devicegroups.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the device_group list resource's
// List path calls.
var listResourceSDKMethods = []string{
	"ListDeviceGroups",
	"GetDeviceGroup",
	"ListDeviceGroupMembers",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device_group list resource, appended to its
// ListResourceConfigSchema Description.
var listResourcePrivileges = permissions.Section(devicegroups.Privileges, listResourceSDKMethods...)
