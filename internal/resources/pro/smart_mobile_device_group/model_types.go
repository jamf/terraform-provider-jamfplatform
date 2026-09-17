// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// SmartMobileDeviceGroupResourceModel is the Terraform model for a smart mobile
// device group. The criteria element type is the shared criteria.CriterionModel
// rather than a package-local copy, so the directory-service group resolve,
// restore, read-back and suppression helpers take it without a bridging
// conversion.
//
// There is deliberately no membership count here. Jamf Pro recomputes a smart
// group's membership from inventory on its own schedule, so a count in resource
// state would move without any configuration change and show as drift on every
// refresh. The plural data source reports it instead.
type SmartMobileDeviceGroupResourceModel struct {
	ID          types.String              `tfsdk:"id"`
	PlatformID  types.String              `tfsdk:"platform_id"`
	Name        types.String              `tfsdk:"name"`
	Description types.String              `tfsdk:"description"`
	SiteID      types.String              `tfsdk:"site_id"`
	Criteria    []criteria.CriterionModel `tfsdk:"criteria"`
	Timeouts    resourceTimeouts.Value    `tfsdk:"timeouts"`
}

// smartMobileDeviceGroupIdentityModel is the identity object for the resource,
// for import and for list results. The Jamf Pro identifier is the key: it is
// what every group route takes, and what the singular data source reports.
type smartMobileDeviceGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// SmartMobileDeviceGroupDataSourceModel is the Terraform model for the singular
// data source. Either id or name is supplied, enforced by ExactlyOneOf.
type SmartMobileDeviceGroupDataSourceModel struct {
	ID          types.String              `tfsdk:"id"`
	PlatformID  types.String              `tfsdk:"platform_id"`
	Name        types.String              `tfsdk:"name"`
	Description types.String              `tfsdk:"description"`
	SiteID      types.String              `tfsdk:"site_id"`
	Criteria    []criteria.CriterionModel `tfsdk:"criteria"`
	Timeouts    datasourceTimeouts.Value  `tfsdk:"timeouts"`
}

// SmartMobileDeviceGroupsDataSourceModel is the Terraform model for the plural
// data source.
type SmartMobileDeviceGroupsDataSourceModel struct {
	ID       types.String                                   `tfsdk:"id"`
	Groups   []SmartMobileDeviceGroupsDataSourceResultModel `tfsdk:"smart_mobile_device_groups"`
	Filters  []filters.FilterModel                          `tfsdk:"filter"`
	Timeouts datasourceTimeouts.Value                       `tfsdk:"timeouts"`
}

// SmartMobileDeviceGroupsDataSourceResultModel is one entry in the plural data
// source's results. The collection read returns a summary of each group and no
// criteria at all, so this shape is a strict subset of the singular model and
// the omission is declared here rather than implied: a group's criteria need a
// singular lookup.
type SmartMobileDeviceGroupsDataSourceResultModel struct {
	ID          types.String `tfsdk:"id"`
	PlatformID  types.String `tfsdk:"platform_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SiteID      types.String `tfsdk:"site_id"`
	MemberCount types.Int64  `tfsdk:"member_count"`
}

// SmartMobileDeviceGroupListResourceModel is the config model for list queries.
type SmartMobileDeviceGroupListResourceModel struct {
	Filters []filters.FilterModel `tfsdk:"filter"`
}
