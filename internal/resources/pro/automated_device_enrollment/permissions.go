// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package automated_device_enrollment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the automated device enrollment
// resource's CRUD path calls (crud.go). It mirrors the "SDK endpoints used"
// block in crud.go and drives the "Required Jamf permissions" table appended to
// the resource MarkdownDescription. permissions_test.go asserts this list stays
// in sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"UploadDeviceEnrollmentTokenV1",
	"GetDeviceEnrollmentV1",
	"UpdateDeviceEnrollmentV1",
	"ReplaceDeviceEnrollmentTokenV1",
	"DeleteDeviceEnrollmentV1",
	"GetLatestDeviceEnrollmentSyncV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the automated device enrollment resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the automated device enrollment
// data source calls (data_source.go). The name-lookup path resolves via
// ResolveDeviceEnrollmentV1ByName, which is a synthetic resolver absent from the
// SDK privilege registry; the privilege it requires
// (device-enrollment-program-instances:read) is already covered by
// GetDeviceEnrollmentV1.
var dataSourceSDKMethods = []string{
	"GetDeviceEnrollmentV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the automated device enrollment data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the automated device enrollment
// list resource calls (list_resource.go).
var listResourceSDKMethods = []string{
	"ListDeviceEnrollmentsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the automated device enrollment list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
