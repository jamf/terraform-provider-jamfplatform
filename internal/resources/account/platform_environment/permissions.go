// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the platform environment resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
//
// ListEnvironments appears because it is how an environment is read: Jamf
// Account exposes no read of a single one. ListTenants appears because Delete
// reads the tenant list to refuse an environment that still holds tenants.
//
// The privileges these render come from the gateway's own authorization policy
// rather than the published spec, which declares none for this namespace. The
// SDK records that provenance per method, which matters because a spec-declared
// empty set would otherwise render as "no permission needed".
var resourceSDKMethods = []string{
	"CreateEnvironment",
	"ListEnvironments",
	"UpdateEnvironment",
	"DeleteEnvironment",
	"ListTenants",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the platform environment resource.
var resourcePrivileges = permissions.Section(account.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular data source calls.
var dataSourceSDKMethods = []string{
	"ListEnvironments",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular platform environment data source.
var dataSourcePrivileges = permissions.Section(account.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListEnvironments",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural platform environments data source.
var pluralDataSourcePrivileges = permissions.Section(account.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListEnvironments",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the platform environment list resource.
var listResourcePrivileges = permissions.Section(account.Privileges, listResourceSDKMethods...)
