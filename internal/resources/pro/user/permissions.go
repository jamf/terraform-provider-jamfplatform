// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the singular user data source
// (data_source.go) calls. It drives the "Required Jamf permissions" table
// appended to the data source MarkdownDescription. permissions_test.go asserts
// this list stays in sync with the actual client.<Method> calls in
// data_source.go and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetUserV1",
	"ListUsersV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular user data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural users data source
// (datasource_plural.go) calls. It drives the "Required Jamf permissions" table
// appended to the plural data source MarkdownDescription.
var pluralDataSourceSDKMethods = []string{
	"ListUsersV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural users data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)
