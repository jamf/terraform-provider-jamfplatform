// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package service_discovery_enrollment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the service-discovery-enrollment
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"GetServiceDiscoveryEnrollmentWellKnownSettingsV1",
	"UpdateServiceDiscoveryEnrollmentWellKnownSettingsV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the service-discovery-enrollment resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the service-discovery-enrollment
// data source calls. The data source only reads, so it requires a subset of
// the resource's privileges.
var dataSourceSDKMethods = []string{
	"GetServiceDiscoveryEnrollmentWellKnownSettingsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the service-discovery-enrollment data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
