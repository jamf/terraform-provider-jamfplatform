// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_title

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the singular App Installer title
// data source's Read path calls. It mirrors the client.<Method> calls in
// data_source.go and drives the "Required Jamf privileges" table appended to
// the data source MarkdownDescription. permissions_test.go asserts this list
// stays in sync with data_source.go and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetAppInstallerTitleV1",
	"ListAppInstallerTitleVersionsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf privileges" Markdown
// section for the singular App Installer title data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural App Installer
// titles data source's Read path calls. It mirrors the client.<Method> calls in
// datasource_plural.go and drives the "Required Jamf privileges" table appended
// to the plural data source MarkdownDescription.
var pluralDataSourceSDKMethods = []string{
	"ListAppInstallerTitlesV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf privileges" Markdown
// section for the plural App Installer titles data source, appended to its
// MarkdownDescription.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)
