// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_privileges

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the proclassic SDK methods the account_privileges
// data source's Read path reaches. The data source delegates to
// accountprivileges.DiscoverCategorized, which walks the tenant's accounts and
// groups to source the privilege catalog from an Administrator privilege set;
// those three GETs are the privileges this data source actually needs. The list
// drives the "Required Jamf permissions" table appended to the data source
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual SDK calls made in the discovery path and with the SDK privilege
// registry.
var dataSourceSDKMethods = []string{
	"ListAccounts",
	"GetAccountGroupByID",
	"GetAccountByUserID",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the account_privileges data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)
