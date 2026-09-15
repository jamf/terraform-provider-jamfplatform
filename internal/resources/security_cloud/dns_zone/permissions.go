// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the DNS zone resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
//
// The custom DNS operations are published as requiring the `ztna:*` privileges
// rather than a dns- or zone-specific one. That is what the spec records and
// what the table therefore renders; it has not been independently confirmed
// against the gateway, and if the spec is wrong the fix belongs upstream in the
// SDK rather than here.
var resourceSDKMethods = []string{
	"CreateDnsZoneV1",
	"GetDnsZoneV1",
	"UpdateDnsZoneV1",
	"DeleteDnsZoneV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the DNS zone resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular DNS zone data source
// calls. The name lookup goes through ResolveDnsZoneV1ByName, a synthetic helper
// the privilege registry does not carry; it reads the zone list, so the list
// method stands in for it here.
var dataSourceSDKMethods = []string{
	"GetDnsZoneV1",
	"ListDnsZonesV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular DNS zone data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural DNS zones data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListDnsZonesV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural DNS zones data source.
var pluralDataSourcePrivileges = permissions.Section(securitycloud.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the DNS zone list resource calls.
var listResourceSDKMethods = []string{
	"ListDnsZonesV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the DNS zone list resource.
var listResourcePrivileges = permissions.Section(securitycloud.Privileges, listResourceSDKMethods...)
