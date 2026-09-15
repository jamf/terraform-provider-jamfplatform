// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the UEM Connect resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateUemConnectorV1",
	"GetUemConnectorV1",
	"UpdateUemConnectorSyncSettingsV1",
	"EnableUemConnectorV1",
	"DeleteUemConnectorV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the UEM Connect resource.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the UEM Connect data source calls.
// The list read is what finds the tenant's single integration, since the data
// source takes no ID.
var dataSourceSDKMethods = []string{
	"ListUemConnectorsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the UEM Connect data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the UEM Connect list resource calls.
var listResourceSDKMethods = []string{
	"ListUemConnectorsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the UEM Connect list resource.
var listResourcePrivileges = permissions.Section(securitycloud.Privileges, listResourceSDKMethods...)
