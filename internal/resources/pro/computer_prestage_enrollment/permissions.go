// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the computer prestage enrollment
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateComputerPrestageV3",
	"GetComputerPrestageV3",
	"UpdateComputerPrestageV3",
	"DeleteComputerPrestageV3",
	"GetComputerPrestageScopeV2",
	"ReplaceComputerPrestageScopeV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer prestage enrollment resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-known SDK methods the data source
// calls. The name-lookup path also calls ResolveComputerPrestageV3IDByName,
// but synthetic Resolve<X>ByName helpers are not present in the SDK privilege
// registry (they compose the listed primitives), so only GetComputerPrestageV3
// drives the privileges table here.
var dataSourceSDKMethods = []string{
	"GetComputerPrestageV3",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer prestage enrollment data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls. The
// list fetch is ListComputerPrestagesV3; per-item scope reads (config
// generation) call GetComputerPrestageScopeV2.
var listResourceSDKMethods = []string{
	"ListComputerPrestagesV3",
	"GetComputerPrestageScopeV2",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the computer prestage enrollment list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
