// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// privilegeRegistry merges the Pro and ProClassic privilege registries, because
// the resource's write path spans both: it attempts the group endpoint and, when
// that endpoint refuses a criterion or an operator it offers in the admin UI,
// repeats the write through Jamf Pro's older group interface. The two registries
// carry none of this construct's methods in common, so the merge cannot prefer
// one family's entry over the other's, and TestPrivilegeRegistryMergeIsDisjoint
// pins that.
//
// Only the resource renders from it. The data sources and the list resource read
// through the group endpoint alone and stay on the Pro registry, so a classic
// call appearing in one of their files is a change worth noticing rather than one
// the table absorbs.
var privilegeRegistry = permissions.Merge(pro.Privileges, proclassic.Privileges)

// resourceSDKMethods lists the SDK methods the resource's CRUD path calls. It
// mirrors the "SDK endpoints used" block in crud.go and drives the "Required
// Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
//
// The last three are the fallback write path: the older group interface accepts
// the criterion and operator combinations the group endpoint refuses, and the
// merge patch sets the description that interface has no field for. All three
// need the device-group permissions the group endpoint already declares, so they
// add no row — they are declared because the call-site guard reads this list as
// the record of what crud.go calls, not as the set of distinct permissions.
//
// Two calls the CRUD path reaches indirectly are deliberately absent, and both
// would add nothing to the rendered table. The platform_id bridge reads the
// device-group collection, which needs the same read permission the group read
// already declares. A directory-service group criterion resolves through the
// shared directory search, which needs the directory-server read permission and
// only when such a criterion is used; the criterion's own diagnostic names that
// case when it fails, which serves an operator better than a permission an
// ordinary group never touches.
var resourceSDKMethods = []string{
	"CreateSmartMobileDeviceGroupV2",
	"GetSmartMobileDeviceGroupV2",
	"UpdateSmartMobileDeviceGroupV2",
	"DeleteSmartMobileDeviceGroupV2",
	"CreateMobileDeviceGroupByID",
	"UpdateMobileDeviceGroupByID",
	"PatchGroupV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the resource.
var resourcePrivileges = permissions.Section(privilegeRegistry, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-backed SDK methods the singular data
// source's Read path calls. The name lookup also calls
// ResolveSmartMobileDeviceGroupV2IDByName, a resolver wrapper with no registry
// entry of its own: it queries the group collection and then reads, needing only
// the read permission the by-id lookup already declares. The match test filters
// it out.
var dataSourceSDKMethods = []string{
	"GetSmartMobileDeviceGroupV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListSmartMobileDeviceGroupsV2",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls: the
// collection read, plus a per-item read to hydrate each result when Terraform
// asks for the resource body.
var listResourceSDKMethods = []string{
	"ListSmartMobileDeviceGroupsV2",
	"GetSmartMobileDeviceGroupV2",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
