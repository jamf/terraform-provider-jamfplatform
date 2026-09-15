// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package return_to_service

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Return to Service resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateReturnToServiceConfigurationV1",
	"GetReturnToServiceConfigurationV1",
	"UpdateReturnToServiceConfigurationV1",
	"DeleteReturnToServiceConfigurationV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Return to Service resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-known SDK methods the data source's
// Read path calls. The by-name path additionally calls
// ResolveReturnToServiceConfigurationV1ByName, an SDK resolver wrapper that is
// not itself a privileged endpoint (it filters the same /return-to-service
// read), so it is not listed here — its privilege is already covered by
// GetReturnToServiceConfigurationV1's read scope.
var dataSourceSDKMethods = []string{
	"GetReturnToServiceConfigurationV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Return to Service data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource's List path
// calls.
var listResourceSDKMethods = []string{
	"ListReturnToServiceConfigurationsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Return to Service list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
