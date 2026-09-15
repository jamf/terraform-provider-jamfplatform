// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the enrollment profile resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateMobileDeviceEnrollmentProfileByID",
	"GetMobileDeviceEnrollmentProfileByID",
	"UpdateMobileDeviceEnrollmentProfileByID",
	"DeleteMobileDeviceEnrollmentProfileByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the enrollment profile resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the enrollment profile data
// source calls — one selector lookup per id/name/invitation branch in
// data_source.go.
var dataSourceSDKMethods = []string{
	"GetMobileDeviceEnrollmentProfileByID",
	"GetMobileDeviceEnrollmentProfileByName",
	"GetMobileDeviceEnrollmentProfileByInvitation",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the enrollment profile data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the enrollment profile list
// resource calls.
var listResourceSDKMethods = []string{
	"ListMobileDeviceEnrollmentProfiles",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetMobileDeviceEnrollmentProfileByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the enrollment profile list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
