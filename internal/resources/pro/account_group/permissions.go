// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the account-group resource's CRUD
// path calls. It mirrors the client.<Method> calls in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with crud.go and with the
// SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateAccountGroupByID",
	"GetAccountGroupByID",
	"UpdateAccountGroupByID",
	"DeleteAccountGroupByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the account-group resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the account-group data source's
// Read path calls. It mirrors the client.<Method> calls in data_source.go.
var dataSourceSDKMethods = []string{
	"GetAccountGroupByID",
	"GetAccountGroupByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the account-group data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the account-group list resource's
// List path calls. It mirrors the client.<Method> calls in list_resource.go.
var listResourceSDKMethods = []string{
	"GetAccountGroupByID",
	"ListAccounts",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the account-group list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
