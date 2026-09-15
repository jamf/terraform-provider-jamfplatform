// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_groups

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// DeviceGroupsDataSource implements the Terraform data source for Jamf device groups.
type DeviceGroupsDataSource struct {
	client *devicegroups.Client
}

// DeviceGroupsDataSourceModel represents the Terraform data source model for Jamf device groups.
type DeviceGroupsDataSourceModel struct {
	ID           types.String                        `tfsdk:"id"`
	Filters      []filters.FilterModel               `tfsdk:"filter"`
	Timeouts     timeouts.Value                      `tfsdk:"timeouts"`
	DeviceGroups []DeviceGroupsDataSourceResultModel `tfsdk:"device_groups"`
}

// DeviceGroupsDataSourceResultModel represents a single entry returned by the data source.
type DeviceGroupsDataSourceResultModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	DeviceType  types.String `tfsdk:"device_type"`
	GroupType   types.String `tfsdk:"group_type"`
	MemberCount types.Int64  `tfsdk:"member_count"`
}
