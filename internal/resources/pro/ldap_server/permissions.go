// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the LDAP server resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateLDAPServerByID",
	"GetLDAPServerByID",
	"UpdateLDAPServerByID",
	"DeleteLDAPServerByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the LDAP server resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the LDAP server data source calls.
// It resolves by ID via GetLDAPServerByID and by exact name via
// GetLDAPServerByName.
var dataSourceSDKMethods = []string{
	"GetLDAPServerByID",
	"GetLDAPServerByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the LDAP server data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the LDAP server list resource
// calls. It lists with ListLDAPServers and, when include_resource is true,
// follows up per item with GetLDAPServerByID.
var listResourceSDKMethods = []string{
	"ListLDAPServers",
	"GetLDAPServerByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the LDAP server list resource, appended to its schema
// Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
