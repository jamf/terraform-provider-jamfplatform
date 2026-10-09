// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DeviceGroupResourceModel represents the Terraform resource model for a Jamf
// Security Cloud device group.
type DeviceGroupResourceModel struct {
	ID       types.String           `tfsdk:"id"`
	Name     types.String           `tfsdk:"name"`
	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// deviceGroupIdentityModel represents the identity object for device group
// resources and list results.
type deviceGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DeviceGroupDataSourceModel represents the Terraform data source model for a
// single Jamf Security Cloud device group.
type DeviceGroupDataSourceModel struct {
	ID       types.String             `tfsdk:"id"`
	Name     types.String             `tfsdk:"name"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// DeviceGroupsDataSourceModel represents the Terraform data source model for the
// plural device group lookup.
type DeviceGroupsDataSourceModel struct {
	ID           types.String                        `tfsdk:"id"`
	DeviceGroups []DeviceGroupsDataSourceResultModel `tfsdk:"device_groups"`
	Timeouts     datasourceTimeouts.Value            `tfsdk:"timeouts"`
}

// DeviceGroupsDataSourceResultModel represents a single device group in the
// plural data source results.
//
// ID is nullable and BuiltIn exists solely because of the implicit "Default
// Group": the list endpoint returns it with no identifier, so it is the one
// element whose ID is null. See the plural data source's schema for why it is
// reported rather than filtered out.
type DeviceGroupsDataSourceResultModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	BuiltIn types.Bool   `tfsdk:"built_in"`
}

// DeviceGroupListResourceModel represents the config model for device group list
// queries. Jamf Security Cloud exposes no filter parameters on the group list
// endpoint, so the model carries only the `timeouts` attribute.
type DeviceGroupListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
