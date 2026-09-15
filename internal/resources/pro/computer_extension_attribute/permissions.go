// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the computer extension attribute
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateComputerExtensionAttributeV1",
	"GetComputerExtensionAttributeV1",
	"UpdateComputerExtensionAttributeV1",
	"DeleteComputerExtensionAttributeV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer extension attribute resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the privilege-bearing SDK methods the data source
// calls. data_source.go also calls ResolveComputerExtensionAttributeV1ByName,
// but that resolver is a name-lookup wrapper that delegates to the same
// extension-attributes:read endpoint and is not itself a key in
// the SDK privilege registry, so only the GET is listed here.
var dataSourceSDKMethods = []string{
	"GetComputerExtensionAttributeV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer extension attribute data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListComputerExtensionAttributesV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer extension attribute list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
