// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// assignDeviceGroupResourceModel populates a resource model from a Group response.
//
// The ID guard mirrors the DNS zone assigner: an update response echoes the ID,
// but nothing downstream should depend on that, and an empty echo must not blank
// an ID the caller already holds.
func assignDeviceGroupResourceModel(state *DeviceGroupResourceModel, g *securitycloud.Group) {
	if g.ID != "" {
		state.ID = types.StringValue(g.ID)
	}
	state.Name = types.StringValue(g.Name)
}

// assignDeviceGroupDataSourceModel populates the singular data source model from
// a Group response.
func assignDeviceGroupDataSourceModel(state *DeviceGroupDataSourceModel, g *securitycloud.Group) {
	state.ID = types.StringValue(g.ID)
	state.Name = types.StringValue(g.Name)
}

// buildDeviceGroupsResultModel maps one list item into a plural data source
// result element.
//
// The implicit "Default Group" is the reason this is not a straight field copy:
// the list endpoint returns it with no `id` key, so its ID decodes as the empty
// string. Reporting that as `""` would be a lie about a value the API does not
// have, so it becomes a null and `built_in` becomes true. Every stored group has
// an ID and reports `built_in` false.
func buildDeviceGroupsResultModel(item securitycloud.GroupListItem) DeviceGroupsDataSourceResultModel {
	if item.ID == "" {
		return DeviceGroupsDataSourceResultModel{
			ID:      types.StringNull(),
			Name:    types.StringValue(item.Name),
			BuiltIn: types.BoolValue(true),
		}
	}
	return DeviceGroupsDataSourceResultModel{
		ID:      types.StringValue(item.ID),
		Name:    types.StringValue(item.Name),
		BuiltIn: types.BoolValue(false),
	}
}

// manageableGroups drops the entries Terraform cannot manage from a group list.
//
// The only such entry is the implicit "Default Group", identified by having no
// identifier rather than by matching its name — a tenant whose built-in group is
// labelled differently, or a second unidentified entry appearing later, is handled
// on the same rule. A list result must carry an identity, so an entry without one
// would produce a result Terraform could neither import nor refresh.
func manageableGroups(items []securitycloud.GroupListItem) []securitycloud.GroupListItem {
	manageable := make([]securitycloud.GroupListItem, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		manageable = append(manageable, item)
	}
	return manageable
}
