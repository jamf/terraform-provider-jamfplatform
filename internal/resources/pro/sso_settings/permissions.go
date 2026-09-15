// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the SSO settings resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"GetSsoSettingsV3",
	"UpdateSsoSettingsV3",
	"GetSsoCertificateV2",
	"UpdateSsoCertificateV2",
	"GenerateSsoCertificateV2",
	"DeleteSsoCertificateV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the SSO settings resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the SSO settings (singular) data
// source calls in data_source.go.
var dataSourceSDKMethods = []string{
	"GetSsoSettingsV3",
	"GetSsoCertificateV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the SSO settings data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// dependenciesDataSourceSDKMethods lists the SDK methods the SSO dependencies
// data source calls in data_source_dependencies.go.
var dependenciesDataSourceSDKMethods = []string{
	"GetSsoDependenciesV3",
}

// dependenciesDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the SSO dependencies data source.
var dependenciesDataSourcePrivileges = permissions.Section(pro.Privileges, dependenciesDataSourceSDKMethods...)

// metadataDataSourceSDKMethods lists the SDK methods the SSO SP metadata data
// source calls in data_source_metadata.go.
var metadataDataSourceSDKMethods = []string{
	"GetSsoSettingsV3",
	"DownloadSsoMetadataV3",
}

// metadataDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the SSO SP metadata data source.
var metadataDataSourcePrivileges = permissions.Section(pro.Privileges, metadataDataSourceSDKMethods...)
