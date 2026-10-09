// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DepartmentResourceModel represents the Terraform resource model for a Jamf Pro department.
type DepartmentResourceModel struct {
	ID       types.String           `tfsdk:"id"`
	Name     types.String           `tfsdk:"name"`
	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// DepartmentDataSourceModel represents the Terraform data source model for a Jamf Pro department.
type DepartmentDataSourceModel struct {
	ID       types.String             `tfsdk:"id"`
	Name     types.String             `tfsdk:"name"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// departmentIdentityModel represents the identity object for department resources and list results.
type departmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DepartmentListResourceModel represents the config model for department list queries.
type DepartmentListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}

// DepartmentsDataSourceModel represents the Terraform data source model for department searches.
type DepartmentsDataSourceModel struct {
	ID          types.String                       `tfsdk:"id"`
	Departments []DepartmentsDataSourceResultModel `tfsdk:"departments"`
	Filters     []filters.FilterModel              `tfsdk:"filter"`
	Timeouts    datasourceTimeouts.Value           `tfsdk:"timeouts"`
}

// DepartmentsDataSourceResultModel represents a single department in the search results.
type DepartmentsDataSourceResultModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}
