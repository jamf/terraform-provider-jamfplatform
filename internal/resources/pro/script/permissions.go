// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the script resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateScriptV1",
	"GetScriptV1",
	"UpdateScriptV1",
	"DeleteScriptV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the script resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular script data source
// calls (data_source.go). A data source documents only the privileges it needs.
var dataSourceSDKMethods = []string{
	"GetScriptV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the script data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural scripts data
// source calls (datasource_plural.go).
var pluralDataSourceSDKMethods = []string{
	"ListScriptsV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural scripts data source, appended to its
// MarkdownDescription.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the script list resource calls
// (list_resource.go).
var listResourceSDKMethods = []string{
	"ListScriptsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the script list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
