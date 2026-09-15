// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the SSO connection resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
//
// ListConnections appears on the resource because it is half of every read: no
// single call returns a whole connection, and the collection read is also what
// tells a removed connection apart from one Jamf lists but cannot read on its
// own identifier.
//
// The privileges these render come from the gateway's own authorization policy
// rather than the published specification, which declares none for this
// namespace. The SDK records that provenance per method; the distinction matters
// because a specification-declared empty set would otherwise render as "no
// permission needed", which for these operations is false.
var resourceSDKMethods = []string{
	"CreateConnection",
	"GetConnection",
	"ListConnections",
	"UpdateConnection",
	"DeleteConnection",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the SSO connection resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(account.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular SSO connection data
// source calls. The collection read is both how a name is resolved to an
// identifier and the only place the enabled products and the consent ticket
// appear.
var dataSourceSDKMethods = []string{
	"ListConnections",
	"GetConnection",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular SSO connection data source.
var dataSourcePrivileges = permissions.Section(account.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural SSO connections
// data source calls. Only the collection read: reporting the per-provider
// settings would mean one extra read per connection in the organization, which
// is what the singular data source is for.
var pluralDataSourceSDKMethods = []string{
	"ListConnections",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural SSO connections data source.
var pluralDataSourcePrivileges = permissions.Section(account.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the SSO connection list resource
// calls. It reads each listed connection as well as the collection, because the
// two classes of connection that cannot be imported are only distinguishable from
// the single read — and offering one for a bulk import would leave an entry
// Terraform could never reconcile.
var listResourceSDKMethods = []string{
	"ListConnections",
	"GetConnection",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the SSO connection list resource.
var listResourcePrivileges = permissions.Section(account.Privileges, listResourceSDKMethods...)
