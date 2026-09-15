// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the App Installer resource's CRUD
// path reaches — the four deployment operations it calls directly, plus the one
// catalog read it reaches through the package's own name-resolution helpers. Both
// directions of the title name mapping answer from that single list
// (resolveAppTitleID resolves app_title_name → id, titleNameForID reverse-resolves
// the id back to the name), because the catalog snapshot is cached per provider
// instance and carries both fields — so the resource issues no per-title GET. It
// mirrors the "SDK endpoints used" block in crud.go and drives the "Required
// Jamf privileges" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the SDK calls
// reachable from crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateAppInstallerDeploymentV1",
	"GetAppInstallerDeploymentV1",
	"UpdateAppInstallerDeploymentV1",
	"DeleteAppInstallerDeploymentV1",
	"ListAppInstallerTitlesV1",
}

// resourcePrivileges is the rendered "Required Jamf privileges" Markdown
// section for the App Installer resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular App Installer data
// source reaches. A lookup by id calls the deployment GET directly; a lookup by
// name first goes through the package's own resolveDeploymentIDByName, which
// lists the deployments and decides the match in the provider because Jamf Pro's
// name filter is a case-insensitive glob (see name_lookup.go). Either way the
// title name is reverse-resolved from its id by titleNameForID, out of the cached
// title list rather than a per-title GET. All three are applications:read, so the
// rendered table is unchanged by the two indirect reads, but they are declared here
// so the list describes the requests the data source actually issues.
var dataSourceSDKMethods = []string{
	"GetAppInstallerDeploymentV1",
	"ListAppInstallerDeploymentsV1",
	"ListAppInstallerTitlesV1",
}

// dataSourcePrivileges is the rendered "Required Jamf privileges" Markdown
// section for the singular App Installer data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural App Installers
// data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListAppInstallerDeploymentsV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf privileges" Markdown
// section for the plural App Installers data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the App Installer list resource
// reaches: List for the query, Get for per-item hydration when IncludeResource
// is requested, and the catalog read titleNameForID answers from — hydration has
// to name the title behind app_title_id, which the deployment read reports only
// as an id. The catalog read is on the hydration path alone, so a plain query
// issues the list request and nothing else.
var listResourceSDKMethods = []string{
	"ListAppInstallerDeploymentsV1",
	"GetAppInstallerDeploymentV1",
	"ListAppInstallerTitlesV1",
}

// listResourcePrivileges is the rendered "Required Jamf privileges" Markdown
// section for the App Installer list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
