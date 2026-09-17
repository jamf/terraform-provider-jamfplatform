// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// StaticMobileDeviceGroupResourceModel is the Terraform resource model for a
// static Jamf Pro mobile device group.
//
// AssignedMobileDeviceIDs is Optional-only, never Computed: a null value means
// the configuration does not manage membership, and Read must leave it null so
// that "unmanaged" cannot be confused with "empty". See assignmentsForUpdate for
// what each of the three states sends.
type StaticMobileDeviceGroupResourceModel struct {
	ID                      types.String           `tfsdk:"id"`
	PlatformID              types.String           `tfsdk:"platform_id"`
	Name                    types.String           `tfsdk:"name"`
	Description             types.String           `tfsdk:"description"`
	SiteID                  types.String           `tfsdk:"site_id"`
	AssignedMobileDeviceIDs types.Set              `tfsdk:"assigned_mobile_device_ids"`
	MemberCount             types.Int64            `tfsdk:"member_count"`
	Timeouts                resourceTimeouts.Value `tfsdk:"timeouts"`
}

// StaticMobileDeviceGroupDataSourceModel is the Terraform model for the singular
// data source. Either id or name is supplied, enforced by ExactlyOneOf.
//
// The membership list is a Computed list rather than the resource's set: nothing
// here is user-supplied, and a data source reports what Jamf Pro returned in the
// order it returned it.
type StaticMobileDeviceGroupDataSourceModel struct {
	ID                      types.String             `tfsdk:"id"`
	PlatformID              types.String             `tfsdk:"platform_id"`
	Name                    types.String             `tfsdk:"name"`
	Description             types.String             `tfsdk:"description"`
	SiteID                  types.String             `tfsdk:"site_id"`
	AssignedMobileDeviceIDs []types.String           `tfsdk:"assigned_mobile_device_ids"`
	MemberCount             types.Int64              `tfsdk:"member_count"`
	Timeouts                datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// staticMobileDeviceGroupIdentityModel is the identity object used for import and
// for list results. It carries the Jamf Pro identifier, which is what every route
// in this family takes and what the resource stores as its id.
type staticMobileDeviceGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// StaticMobileDeviceGroupsDataSourceModel is the Terraform model for the plural
// data source.
type StaticMobileDeviceGroupsDataSourceModel struct {
	ID       types.String                                    `tfsdk:"id"`
	Groups   []StaticMobileDeviceGroupsDataSourceResultModel `tfsdk:"static_mobile_device_groups"`
	Filters  []filters.FilterModel                           `tfsdk:"filter"`
	Timeouts datasourceTimeouts.Value                        `tfsdk:"timeouts"`
}

// StaticMobileDeviceGroupsDataSourceResultModel is one group in the plural
// results. Membership is deliberately absent: it needs a request per group, and
// the singular data source is where a caller asks for it.
type StaticMobileDeviceGroupsDataSourceResultModel struct {
	ID          types.String `tfsdk:"id"`
	PlatformID  types.String `tfsdk:"platform_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SiteID      types.String `tfsdk:"site_id"`
	MemberCount types.Int64  `tfsdk:"member_count"`
}

// StaticMobileDeviceGroupListResourceModel is the config model for list queries.
type StaticMobileDeviceGroupListResourceModel struct {
	Filters []filters.FilterModel `tfsdk:"filter"`
}
