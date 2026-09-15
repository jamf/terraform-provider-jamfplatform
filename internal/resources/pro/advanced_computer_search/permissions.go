// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_computer_search

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the advanced computer search
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateAdvancedComputerSearchByID",
	"GetAdvancedComputerSearchByID",
	"UpdateAdvancedComputerSearchByID",
	"DeleteAdvancedComputerSearchByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced computer search resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the advanced computer search data
// source calls (lookup by ID or by name). permissions_test.go asserts this
// list stays in sync with the client.<Method> calls in data_source.go and with
// the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetAdvancedComputerSearchByID",
	"GetAdvancedComputerSearchByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced computer search data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the advanced computer search
// list resource calls. permissions_test.go asserts this list stays in sync
// with the client.<Method> calls in list_resource.go and with the SDK
// privilege registry.
var listResourceSDKMethods = []string{
	"ListAdvancedComputerSearches",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetAdvancedComputerSearchByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced computer search list resource, appended to its
// MarkdownDescription.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
