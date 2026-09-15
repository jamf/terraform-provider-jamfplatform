// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the JSON Web Token configuration
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateJsonWebTokenConfigurationByID",
	"GetJsonWebTokenConfigurationByID",
	"UpdateJsonWebTokenConfigurationByID",
	"DeleteJsonWebTokenConfigurationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the JSON Web Token configuration resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the data source's Read path calls.
// Lookup is by ID (Get) or by exact name (List + in-memory walk). It documents
// only the read privileges the data source needs, not the resource's full CRUD
// set.
var dataSourceSDKMethods = []string{
	"GetJsonWebTokenConfigurationByID",
	"ListJsonWebTokenConfigurations",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the JSON Web Token configuration data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls: List to
// enumerate identities, plus a per-item Get when IncludeResource is requested
// (config generation).
var listResourceSDKMethods = []string{
	"ListJsonWebTokenConfigurations",
	"GetJsonWebTokenConfigurationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the JSON Web Token configuration list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
