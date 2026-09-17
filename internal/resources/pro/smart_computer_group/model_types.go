// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// SmartComputerGroupResourceModel is the Terraform resource model for a Jamf Pro
// smart computer group.
type SmartComputerGroupResourceModel struct {
	ID          types.String              `tfsdk:"id"`
	PlatformID  types.String              `tfsdk:"platform_id"`
	Name        types.String              `tfsdk:"name"`
	Description types.String              `tfsdk:"description"`
	SiteID      types.String              `tfsdk:"site_id"`
	Criteria    []criteria.CriterionModel `tfsdk:"criteria"`
	Timeouts    resourceTimeouts.Value    `tfsdk:"timeouts"`
}

// SmartComputerGroupDataSourceModel is the Terraform model for the singular data
// source. Exactly one of id or name is supplied, enforced by ExactlyOneOf.
type SmartComputerGroupDataSourceModel struct {
	ID          types.String              `tfsdk:"id"`
	PlatformID  types.String              `tfsdk:"platform_id"`
	Name        types.String              `tfsdk:"name"`
	Description types.String              `tfsdk:"description"`
	SiteID      types.String              `tfsdk:"site_id"`
	Criteria    []criteria.CriterionModel `tfsdk:"criteria"`
	Timeouts    datasourceTimeouts.Value  `tfsdk:"timeouts"`
}

// smartComputerGroupIdentityModel is the identity object for the resource and
// for every list result.
type smartComputerGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// SmartComputerGroupsDataSourceModel is the Terraform model for the plural data
// source.
type SmartComputerGroupsDataSourceModel struct {
	ID       types.String                               `tfsdk:"id"`
	Groups   []SmartComputerGroupsDataSourceResultModel `tfsdk:"smart_computer_groups"`
	Filters  []filters.FilterModel                      `tfsdk:"filter"`
	Timeouts datasourceTimeouts.Value                   `tfsdk:"timeouts"`
}

// SmartComputerGroupsDataSourceResultModel is one group in the plural results.
//
// It carries its own shape because the search Jamf Pro answers with is a strict
// subset of a single group's: it adds the membership count and omits the
// criteria entirely. Reading the criteria of every group on a page would mean one
// request per group, so the plural data source reports what the search returns
// and points a consumer needing criteria at the singular lookup — the same split
// jamfplatform_pro_user_groups makes for the same reason.
type SmartComputerGroupsDataSourceResultModel struct {
	ID          types.String `tfsdk:"id"`
	PlatformID  types.String `tfsdk:"platform_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SiteID      types.String `tfsdk:"site_id"`
	MemberCount types.Int64  `tfsdk:"member_count"`
}

// SmartComputerGroupListResourceModel is the config model for list queries.
type SmartComputerGroupListResourceModel struct {
	Filters []filters.FilterModel `tfsdk:"filter"`
}
