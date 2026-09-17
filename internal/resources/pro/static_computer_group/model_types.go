// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// StaticComputerGroupResourceModel is the Terraform model for a Jamf Pro static
// computer group.
//
// ID holds the Jamf Pro numeric identifier, which is what every route in this
// family takes and what import expects. PlatformID holds the identifier the
// jamfplatform_device_* constructs key on.
type StaticComputerGroupResourceModel struct {
	ID                  types.String           `tfsdk:"id"`
	PlatformID          types.String           `tfsdk:"platform_id"`
	Name                types.String           `tfsdk:"name"`
	Description         types.String           `tfsdk:"description"`
	SiteID              types.String           `tfsdk:"site_id"`
	AssignedComputerIDs types.Set              `tfsdk:"assigned_computer_ids"`
	Timeouts            resourceTimeouts.Value `tfsdk:"timeouts"`
}

// StaticComputerGroupDataSourceModel is the model for the singular data source.
// Exactly one of ID or Name is supplied, enforced by ExactlyOneOf.
type StaticComputerGroupDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	PlatformID          types.String             `tfsdk:"platform_id"`
	Name                types.String             `tfsdk:"name"`
	Description         types.String             `tfsdk:"description"`
	SiteID              types.String             `tfsdk:"site_id"`
	AssignedComputerIDs types.Set                `tfsdk:"assigned_computer_ids"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// staticComputerGroupIdentityModel is the identity object carried by the
// resource and by each list result.
type staticComputerGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// StaticComputerGroupsDataSourceModel is the model for the plural data source.
type StaticComputerGroupsDataSourceModel struct {
	ID                   types.String                                `tfsdk:"id"`
	StaticComputerGroups []StaticComputerGroupsDataSourceResultModel `tfsdk:"static_computer_groups"`
	Filters              []filters.FilterModel                       `tfsdk:"filter"`
	Timeouts             datasourceTimeouts.Value                    `tfsdk:"timeouts"`
}

// StaticComputerGroupsDataSourceResultModel is one group in a plural read.
//
// MemberCount comes from the search result itself and so costs no extra
// request. The member identifiers do not appear here: reading them means one
// request per group against a second API surface, which is what the singular
// data source is for.
type StaticComputerGroupsDataSourceResultModel struct {
	ID          types.String `tfsdk:"id"`
	PlatformID  types.String `tfsdk:"platform_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SiteID      types.String `tfsdk:"site_id"`
	MemberCount types.Int64  `tfsdk:"member_count"`
}

// StaticComputerGroupListResourceModel is the config model for list queries.
type StaticComputerGroupListResourceModel struct {
	Filters []filters.FilterModel `tfsdk:"filter"`
}
