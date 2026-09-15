// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the grouped gateway resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateZtnaGroupedGatewayV1",
	"GetZtnaGroupedGatewayV1",
	"UpdateZtnaGroupedGatewayV1",
	"DeleteZtnaGroupedGatewayV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the grouped gateway resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular grouped gateway data
// source calls. The name lookup goes through ResolveZtnaGroupedGatewayV1ByName, a
// synthetic helper the privilege registry does not carry; it reads the list, so
// the list method stands in for it here.
var dataSourceSDKMethods = []string{
	"GetZtnaGroupedGatewayV1",
	"ListZtnaGroupedGatewaysV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the singular grouped gateway data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural grouped gateways
// data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListZtnaGroupedGatewaysV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural grouped gateways data source.
var pluralDataSourcePrivileges = permissions.Section(securitycloud.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the grouped gateway list resource
// calls.
var listResourceSDKMethods = []string{
	"ListZtnaGroupedGatewaysV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the grouped gateway list resource.
var listResourcePrivileges = permissions.Section(securitycloud.Privileges, listResourceSDKMethods...)
