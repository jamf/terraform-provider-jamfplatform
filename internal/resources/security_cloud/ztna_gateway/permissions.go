// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the gateway resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateZtnaGatewayV1",
	"GetZtnaGatewayV1",
	"UpdateZtnaGatewayV1",
	"DeleteZtnaGatewayV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the gateway resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular gateway data source
// calls. The name lookup goes through ResolveZtnaGatewayV1ByName, a synthetic
// helper the privilege registry does not carry; it reads the gateway list, so the
// list method stands in for it here.
var dataSourceSDKMethods = []string{
	"GetZtnaGatewayV1",
	"ListZtnaGatewaysV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular gateway data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural gateways data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListZtnaGatewaysV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural gateways data source.
var pluralDataSourcePrivileges = permissions.Section(securitycloud.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the gateway list resource calls.
var listResourceSDKMethods = []string{
	"ListZtnaGatewaysV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the gateway list resource.
var listResourcePrivileges = permissions.Section(securitycloud.Privileges, listResourceSDKMethods...)
