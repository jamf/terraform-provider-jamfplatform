// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// One method list per construct, each mirroring the "SDK endpoints used" block
// in the file that makes the calls, and each rendered into that construct's
// description. permissions_test.go holds both halves honest: the list against
// the SDK privilege registry, and the list against the calls the file actually
// makes.
//
// One privilege is required but not declared here. Resolving platform_id goes
// through progroups, which reads the whole-tenant group list, and that read
// needs device-groups:read — already required by every construct below, so the
// rendered table is identical either way. Declaring it would name a method this
// package never calls by that name, and the helper raises its own diagnostic
// when the read is refused.
var resourceSDKMethods = []string{
	"CreateStaticMobileDeviceGroupV2",
	"GetStaticMobileDeviceGroupV2",
	"PatchStaticMobileDeviceGroupV2",
	"DeleteStaticMobileDeviceGroupV2",
	"ListStaticMobileDeviceGroupMembershipV2",
}

// resourcePrivileges is the rendered permissions section for the resource.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists what the singular data source calls. The name
// lookup resolves through the same list route the plural constructs read and
// carries no privilege of its own in the registry.
var dataSourceSDKMethods = []string{
	"GetStaticMobileDeviceGroupV2",
	"ListStaticMobileDeviceGroupMembershipV2",
}

// dataSourcePrivileges is the rendered permissions section for the singular data
// source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists what the plural data source calls. Membership
// is absent by design: it would cost a request per group.
var pluralDataSourceSDKMethods = []string{
	"ListStaticMobileDeviceGroupsV2",
}

// pluralDataSourcePrivileges is the rendered permissions section for the plural
// data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists what the list resource calls. One page read
// hydrates every result, so there is no per-item fetch.
var listResourceSDKMethods = []string{
	"ListStaticMobileDeviceGroupsV2",
}

// listResourcePrivileges is the rendered permissions section for the list
// resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
