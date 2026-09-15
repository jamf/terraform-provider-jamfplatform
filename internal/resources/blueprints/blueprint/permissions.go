// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	bp "github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the blueprint resource's CRUD path
// calls. It mirrors the client.<Method> calls in crud.go and drives the
// "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateBlueprint",
	"GetBlueprint",
	"UpdateBlueprint",
	"DeleteBlueprint",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the blueprint resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(bp.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the blueprint data source's Read
// path calls. It mirrors the client.<Method> calls in data_source.go and drives
// the "Required Jamf permissions" table appended to the data source
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in data_source.go and with the SDK privilege
// registry.
var dataSourceSDKMethods = []string{
	"GetBlueprint",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the blueprint data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(bp.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the blueprint list resource's
// List path calls. It mirrors the client.<Method> calls in list_resource.go and
// drives the "Required Jamf permissions" table appended to the list resource
// description. permissions_test.go asserts this list stays in sync with the
// actual client.<Method> calls in list_resource.go and with the SDK privilege
// registry.
var listResourceSDKMethods = []string{
	"ListBlueprints",
	"GetBlueprint",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the blueprint list resource, appended to its description.
var listResourcePrivileges = permissions.Section(bp.Privileges, listResourceSDKMethods...)
