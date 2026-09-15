// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourcePrivilegeRegistry merges the Pro and ProClassic SDK privilege
// registries: the account resource is a hybrid that writes base fields via the
// Pro API and the Custom privilege grid via the classic API, so its required
// privileges span both families.
var resourcePrivilegeRegistry = permissions.Merge(pro.Privileges, proclassic.Privileges)

// resourceSDKMethods lists the SDK methods the account resource's CRUD path
// calls (crud.go). It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry. Synthetic resolver methods (e.g. ResolveAccountV1ByName) are not
// listed because the SDK privilege registry does not carry them; the privilege
// they require is already covered by the underlying read method.
var resourceSDKMethods = []string{
	"CreateAccountV1",
	"GetAccountV1",
	"UpdateAccountV1",
	"DeleteAccountV1",
	"GetAccountByUserID",
	"UpdateAccountByUserID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the account resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(resourcePrivilegeRegistry, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the account data source calls
// (data_source.go). The username-lookup path resolves via ResolveAccountV1ByName,
// which is a synthetic resolver absent from the SDK privilege registry; the
// privilege it requires (accounts:read) is already covered by GetAccountV1.
var dataSourceSDKMethods = []string{
	"GetAccountV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the account data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the account list resource calls
// (list_resource.go). Config generation hydrates the Custom privilege grid from
// the classic account representation, so the list resource spans both families
// exactly as the resource does.
var listResourceSDKMethods = []string{
	"ListAccountsV1",
	"GetAccountByUserID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the account list resource. It renders from the merged Pro +
// ProClassic registry because GetAccountByUserID is a classic method.
var listResourcePrivileges = permissions.Section(resourcePrivilegeRegistry, listResourceSDKMethods...)
