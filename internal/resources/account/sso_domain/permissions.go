// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the SSO domain resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
//
// ListDomains appears on the resource because it is how a claim is read: Jamf
// Account exposes no read of a single domain, so the collection read stands in
// for one. `sso-domains:update` is deliberately absent — the only operation
// needing it is the verification, which lives in the
// jamfplatform_account_sso_domain_verify action.
//
// The privileges these render come from the gateway's own authorization policy
// rather than the published spec, which declares none for this namespace. The
// SDK records that provenance per method; the distinction matters because a
// spec-declared empty set would otherwise render as "no permission needed",
// which for these operations is false.
var resourceSDKMethods = []string{
	"CreateDomain",
	"ListDomains",
	"DeleteDomain",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the SSO domain resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(account.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular SSO domain data source
// calls. The assignment lookup is a second operation keyed on the domain name,
// which is what lets the data source report the connections a claim is in use by.
var dataSourceSDKMethods = []string{
	"ListDomains",
	"GetDomainAllocation",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular SSO domain data source.
var dataSourcePrivileges = permissions.Section(account.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural SSO domains data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListDomains",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural SSO domains data source.
var pluralDataSourcePrivileges = permissions.Section(account.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the SSO domain list resource calls.
var listResourceSDKMethods = []string{
	"ListDomains",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the SSO domain list resource.
var listResourcePrivileges = permissions.Section(account.Privileges, listResourceSDKMethods...)
