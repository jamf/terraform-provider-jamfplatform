// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_mobile_device_search

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the advanced mobile device search
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateAdvancedMobileDeviceSearchV1",
	"GetAdvancedMobileDeviceSearchV1",
	"UpdateAdvancedMobileDeviceSearchV1",
	"DeleteAdvancedMobileDeviceSearchV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced mobile device search resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-backed SDK methods the data source's
// Read path calls. The name lookup also calls
// ResolveAdvancedMobileDeviceSearchV1ByName, a resolver wrapper that is not a
// distinct privilege registry entry (it resolves by name then reads, requiring
// only the read privilege GetAdvancedMobileDeviceSearchV1 already covers), so it
// is intentionally omitted here; the match test filters it out.
var dataSourceSDKMethods = []string{
	"GetAdvancedMobileDeviceSearchV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced mobile device search data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource's List path
// calls.
var listResourceSDKMethods = []string{
	"ListAdvancedMobileDeviceSearchesV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced mobile device search list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
