// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// privilegeRegistry merges the Pro and ProClassic registries, because the
// resource spans both: every read and the first attempt at a write are Pro
// calls, and the fallback write Jamf Pro's own refusals force is the one call on
// the older interface. The two are disjoint over the methods named below, which
// permissions_test.go asserts, so the merge cannot mask either.
//
// Only the resource renders from it. The data sources and the list resource
// never reach the older interface and stay on the Pro registry alone.
var privilegeRegistry = permissions.Merge(pro.Privileges, proclassic.Privileges)

// resourceSDKMethods lists the SDK methods the resource's CRUD path reaches,
// directly or through a shared helper. It mirrors the "SDK endpoints used" block
// in crud.go and drives the "Required Jamf permissions" table appended to the
// resource description. permissions_test.go keeps the two in step.
//
// Three of these are not called as client.<Method> in crud.go and are easy to
// miss. GetGroupV2 is what progroups.JamfProIDForPlatformID issues, the bridge
// this construct alone needs. ListGroupsV2 is what progroups.PlatformIDsByJamfProID
// issues to fill platform_id on an import and on a fallback create.
// SearchLdapGroupsV1 is what the shared directory-service group criterion
// helpers issue to turn a group name into the value Jamf Pro stores, so a
// configuration using one of those criteria needs it as well.
//
// The last three are the fallback write path. They are listed unconditionally
// because nothing at plan time predicts whether a configuration will need it:
// the trigger is a refusal from Jamf Pro at apply, so an integration without
// these permissions would fail on the one group that needed them and nowhere
// else.
var resourceSDKMethods = []string{
	"CreateSmartComputerGroupV3",
	"GetSmartComputerGroupV3",
	"UpdateSmartComputerGroupV3",
	"DeleteSmartComputerGroupV3",
	"GetGroupV2",
	"ListGroupsV2",
	"SearchLdapGroupsV1",
	"CreateComputerGroupByID",
	"UpdateComputerGroupByID",
	"PatchGroupV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" section for the
// resource.
var resourcePrivileges = permissions.Section(privilegeRegistry, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular data source reaches.
// The name lookup runs through the smart-group search, and ListGroupsV2 fills
// platform_id.
var dataSourceSDKMethods = []string{
	"GetSmartComputerGroupV3",
	"ListSmartComputerGroupsV3",
	"ListGroupsV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" section for
// the singular data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural data source
// reaches.
var pluralDataSourceSDKMethods = []string{
	"ListSmartComputerGroupsV3",
	"ListGroupsV2",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" section
// for the plural data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource reaches: the
// search, the platform identifier bridge, and a per-item read to hydrate a
// result when include_resource is set.
var listResourceSDKMethods = []string{
	"ListSmartComputerGroupsV3",
	"ListGroupsV2",
	"GetSmartComputerGroupV3",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" section for
// the list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
