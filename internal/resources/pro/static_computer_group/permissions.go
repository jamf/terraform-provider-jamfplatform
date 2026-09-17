// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// privilegeRegistry merges the Pro and ProClassic privilege registries, because
// this construct spans both: the writes and the scalar read are Pro calls, and
// the membership read is the one classic call in the family. The two registries
// are disjoint over the methods named below, so the merge cannot mask either.
//
// It renders all four tables rather than only the two constructs that make a
// classic call, so a method moving between files cannot also change which
// registry answers for it.
var privilegeRegistry = permissions.Merge(pro.Privileges, proclassic.Privileges)

// resourceSDKMethods lists the SDK methods the resource's CRUD path calls. It
// mirrors the "SDK endpoints used" block in crud.go and drives the "Required
// Jamf permissions" table appended to the resource description.
// permissions_test.go asserts it stays in step with the calls crud.go really
// makes and with the SDK privilege registry.
//
// pro.ListGroupsV2 is deliberately absent. Every construct here reaches it
// through progroups.PlatformIDsByJamfProID rather than calling it directly, and
// it requires device-groups:read, which each construct's own group read already
// puts in the table. Listing it would add no row and would take the call-site
// guard away from the file it guards.
var resourceSDKMethods = []string{
	"CreateStaticComputerGroupV3",
	"GetStaticComputerGroupV3",
	"UpdateStaticComputerGroupV3",
	"DeleteStaticComputerGroupV3",
	"GetComputerGroupByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" section for the
// resource.
var resourcePrivileges = permissions.Section(privilegeRegistry, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular data source calls.
// The name lookup goes through ResolveStaticComputerGroupV3IDByName, a synthetic
// resolver the privilege registry does not carry; the privilege it needs
// (device-groups:read) is already covered by GetStaticComputerGroupV3.
var dataSourceSDKMethods = []string{
	"GetStaticComputerGroupV3",
	"GetComputerGroupByID",
}

// dataSourcePrivileges is the rendered section for the singular data source.
var dataSourcePrivileges = permissions.Section(privilegeRegistry, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural data source calls.
// It reads no membership, so no classic method appears.
var pluralDataSourceSDKMethods = []string{
	"ListStaticComputerGroupsV3",
}

// pluralDataSourcePrivileges is the rendered section for the plural data source.
var pluralDataSourcePrivileges = permissions.Section(privilegeRegistry, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls. Config
// generation hydrates from the search result alone, so there is no per-item read
// and no classic method here either.
var listResourceSDKMethods = []string{
	"ListStaticComputerGroupsV3",
}

// listResourcePrivileges is the rendered section for the list resource.
var listResourcePrivileges = permissions.Section(privilegeRegistry, listResourceSDKMethods...)
