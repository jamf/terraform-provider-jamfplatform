// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Cloud Identity Provider
// resource's CRUD and plan-time paths call. The CRUD path is split across
// crud.go (provider discovery on import), crud_azure.go (Entra ID branch),
// crud_google.go (Google Secure LDAP branch), and the keystore verify in
// plan_modifiers.go. This list drives the "Required Jamf permissions" table
// appended to the resource MarkdownDescription. permissions_test.go asserts it
// stays in sync with the actual client.<Method> calls across those files and
// with the SDK privilege registry.
var resourceSDKMethods = []string{
	"GetCloudIdpV1",
	"CreateCloudAzureV1",
	"GetCloudAzureV1",
	"UpdateCloudAzureV1",
	"DeleteCloudAzureV1",
	"CreateCloudLdapV2",
	"GetCloudLdapV2",
	"UpdateCloudLdapV2",
	"DeleteCloudLdapV2",
	"VerifyLdapKeystoreV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Cloud Identity Provider resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular Cloud Identity
// Provider data source calls. It looks up by id or display_name, both served
// client-side from a single ListCloudIdpV1 call.
var dataSourceSDKMethods = []string{
	"ListCloudIdpV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural Cloud Identity
// Provider data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListCloudIdpV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// defaultsDataSourceSDKMethods lists the SDK methods the cloud identity
// provider defaults data source calls. Entra ID needs one read because its
// server-configuration response carries the mappings inline; Google needs two.
var defaultsDataSourceSDKMethods = []string{
	"GetCloudAzureDefaultServerConfigurationV1",
	"GetCloudLdapDefaultServerConfigurationV2",
	"GetCloudLdapDefaultMappingsV2",
}

// defaultsDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the defaults data source.
var defaultsDataSourcePrivileges = permissions.Section(pro.Privileges, defaultsDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the Cloud Identity Provider
// list resource calls. A plain query needs only the registry list; config
// generation additionally reads each Entra ID provider individually, so an
// integration that can list but not read one gets a warning naming the
// providers left out rather than a silently short configuration.
var listResourceSDKMethods = []string{
	"ListCloudIdpV1",
	"GetCloudAzureV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
