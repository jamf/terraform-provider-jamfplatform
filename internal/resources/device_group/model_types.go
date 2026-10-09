// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DeviceGroupResourceModel represents the Terraform resource model for a Jamf device group.
type DeviceGroupResourceModel struct {
	ID          types.String               `tfsdk:"id"`
	JamfProID   types.String               `tfsdk:"jamf_pro_id"`
	Name        types.String               `tfsdk:"name"`
	Description types.String               `tfsdk:"description"`
	DeviceType  types.String               `tfsdk:"device_type"`
	GroupType   types.String               `tfsdk:"group_type"`
	Criteria    []DeviceGroupCriteriaModel `tfsdk:"criteria"`
	Members     types.Set                  `tfsdk:"members"`
	MemberCount types.Int64                `tfsdk:"member_count"`
	Timeouts    resourceTimeouts.Value     `tfsdk:"timeouts"`
}

// DeviceGroupCriteriaModel represents a smart group criterion definition.
type DeviceGroupCriteriaModel struct {
	Order                 types.Int64  `tfsdk:"order"`
	AttributeName         types.String `tfsdk:"criteria"`
	Operator              types.String `tfsdk:"operator"`
	AttributeValue        types.String `tfsdk:"value"`
	JoinType              types.String `tfsdk:"and_or"`
	HasOpeningParenthesis types.Bool   `tfsdk:"has_opening_parenthesis"`
	HasClosingParenthesis types.Bool   `tfsdk:"has_closing_parenthesis"`
}

// DeviceGroupDataSourceModel represents the Terraform data source model for a Jamf device group.
type DeviceGroupDataSourceModel struct {
	ID          types.String               `tfsdk:"id"`
	JamfProID   types.String               `tfsdk:"jamf_pro_id"`
	Name        types.String               `tfsdk:"name"`
	Description types.String               `tfsdk:"description"`
	DeviceType  types.String               `tfsdk:"device_type"`
	GroupType   types.String               `tfsdk:"group_type"`
	Timeouts    datasourceTimeouts.Value   `tfsdk:"timeouts"`
	Criteria    []DeviceGroupCriteriaModel `tfsdk:"criteria"`
	Members     types.List                 `tfsdk:"members"`
	MemberCount types.Int64                `tfsdk:"member_count"`
}

// deviceGroupIdentityModel represents the identity for device group resources and list results.
type deviceGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DeviceGroupListResourceModel represents the config model for device group list queries.
type DeviceGroupListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}
