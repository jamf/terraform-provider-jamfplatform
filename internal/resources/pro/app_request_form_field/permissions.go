// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the App Request form field resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateAppRequestFormInputFieldV1",
	"GetAppRequestFormInputFieldV1",
	"UpdateAppRequestFormInputFieldV1",
	"DeleteAppRequestFormInputFieldV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the App Request form field resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the App Request form field data source
// calls that resolve to a privilege entry. data_source.go also calls
// ResolveAppRequestFormInputFieldV1ByName, but that resolver is a wrapper around the
// list endpoint and is not itself a privilege-registry entry; its required privilege
// (app-request:read) is identical to GetAppRequestFormInputFieldV1, so
// the rendered table is complete. The drift-guard test filters file calls to methods
// known to the registry, so the resolver is excluded there too.
var dataSourceSDKMethods = []string{
	"GetAppRequestFormInputFieldV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the App Request form field data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the App Request form field list
// resource calls. Drives the "Required Jamf permissions" table appended to the list
// resource Description.
var listResourceSDKMethods = []string{
	"ListAppRequestFormInputFieldsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the App Request form field list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
