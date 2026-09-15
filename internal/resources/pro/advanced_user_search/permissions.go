// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the advanced user search resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateAdvancedUserSearchByID",
	"GetAdvancedUserSearchByID",
	"UpdateAdvancedUserSearchByID",
	"DeleteAdvancedUserSearchByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced user search resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the advanced user search data
// source's Read path calls (lookup by ID or by name).
var dataSourceSDKMethods = []string{
	"GetAdvancedUserSearchByID",
	"GetAdvancedUserSearchByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced user search data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the advanced user search list
// resource's List path calls.
var listResourceSDKMethods = []string{
	"ListAdvancedUserSearches",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetAdvancedUserSearchByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the advanced user search list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
