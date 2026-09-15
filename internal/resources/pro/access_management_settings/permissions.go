// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package access_management_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Access Management settings
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"GetEnrollmentAccessManagementV4",
	"UpdateEnrollmentAccessManagementV4",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Access Management settings resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the Access Management settings
// data source's Read path calls. A data source documents only the privileges
// it needs — the read endpoint, not the resource's full CRUD set.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in data_source.go and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetEnrollmentAccessManagementV4",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Access Management settings data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
