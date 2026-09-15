// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the ZTNA app resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateZtnaAppV1",
	"GetZtnaAppV1",
	"UpdateZtnaAppV1",
	"DeleteZtnaAppV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the ZTNA app resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular ZTNA app data source
// calls. Both the ID and the name lookup are covered by these two: the name path
// reads the app list, because Jamf Security Cloud does not require application
// names to be unique and the provider has to see every match to report an
// ambiguous one.
var dataSourceSDKMethods = []string{
	"GetZtnaAppV1",
	"ListZtnaAppsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the singular ZTNA app data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural ZTNA apps data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListZtnaAppsV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural ZTNA apps data source.
var pluralDataSourcePrivileges = permissions.Section(securitycloud.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the ZTNA app list resource calls.
var listResourceSDKMethods = []string{
	"ListZtnaAppsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the ZTNA app list resource.
var listResourcePrivileges = permissions.Section(securitycloud.Privileges, listResourceSDKMethods...)
