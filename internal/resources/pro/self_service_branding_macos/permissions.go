// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_macos

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Self Service macOS branding
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"ListMacOSBrandingConfigurationsV1",
	"CreateMacOSBrandingConfigurationV1",
	"GetMacOSBrandingConfigurationV1",
	"UpdateMacOSBrandingConfigurationV1",
	"DeleteMacOSBrandingConfigurationV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Self Service macOS branding resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the data source's Read path calls.
// The data source only Lists the singleton, so it needs read privileges only —
// not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"ListMacOSBrandingConfigurationsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Self Service macOS branding data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
