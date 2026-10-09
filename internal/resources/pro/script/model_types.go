// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// ScriptResourceModel represents the Terraform resource model for a Jamf Pro script.
type ScriptResourceModel struct {
	ID             types.String           `tfsdk:"id"`
	Name           types.String           `tfsdk:"name"`
	CategoryID     types.String           `tfsdk:"category_id"`
	CategoryName   types.String           `tfsdk:"category_name"`
	Info           types.String           `tfsdk:"info"`
	Notes          types.String           `tfsdk:"notes"`
	OsRequirements types.String           `tfsdk:"os_requirements"`
	Priority       types.String           `tfsdk:"priority"`
	Parameter4     types.String           `tfsdk:"parameter_4"`
	Parameter5     types.String           `tfsdk:"parameter_5"`
	Parameter6     types.String           `tfsdk:"parameter_6"`
	Parameter7     types.String           `tfsdk:"parameter_7"`
	Parameter8     types.String           `tfsdk:"parameter_8"`
	Parameter9     types.String           `tfsdk:"parameter_9"`
	Parameter10    types.String           `tfsdk:"parameter_10"`
	Parameter11    types.String           `tfsdk:"parameter_11"`
	ScriptContents types.String           `tfsdk:"script_contents"`
	Timeouts       resourceTimeouts.Value `tfsdk:"timeouts"`
}

// ScriptDataSourceModel represents the Terraform data source model for a Jamf Pro script.
type ScriptDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	Name           types.String             `tfsdk:"name"`
	CategoryID     types.String             `tfsdk:"category_id"`
	CategoryName   types.String             `tfsdk:"category_name"`
	Info           types.String             `tfsdk:"info"`
	Notes          types.String             `tfsdk:"notes"`
	OsRequirements types.String             `tfsdk:"os_requirements"`
	Priority       types.String             `tfsdk:"priority"`
	Parameter4     types.String             `tfsdk:"parameter_4"`
	Parameter5     types.String             `tfsdk:"parameter_5"`
	Parameter6     types.String             `tfsdk:"parameter_6"`
	Parameter7     types.String             `tfsdk:"parameter_7"`
	Parameter8     types.String             `tfsdk:"parameter_8"`
	Parameter9     types.String             `tfsdk:"parameter_9"`
	Parameter10    types.String             `tfsdk:"parameter_10"`
	Parameter11    types.String             `tfsdk:"parameter_11"`
	ScriptContents types.String             `tfsdk:"script_contents"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// scriptIdentityModel represents the identity object for script resources and list results.
type scriptIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// ScriptListResourceModel represents the config model for script list queries.
type ScriptListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}

// ScriptsDataSourceModel represents the Terraform data source model for script searches.
type ScriptsDataSourceModel struct {
	ID       types.String                   `tfsdk:"id"`
	Scripts  []ScriptsDataSourceResultModel `tfsdk:"scripts"`
	Filters  []filters.FilterModel          `tfsdk:"filter"`
	Timeouts datasourceTimeouts.Value       `tfsdk:"timeouts"`
}

// ScriptsDataSourceResultModel represents a single script in the search results.
type ScriptsDataSourceResultModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	CategoryID     types.String `tfsdk:"category_id"`
	CategoryName   types.String `tfsdk:"category_name"`
	Info           types.String `tfsdk:"info"`
	Notes          types.String `tfsdk:"notes"`
	OsRequirements types.String `tfsdk:"os_requirements"`
	Priority       types.String `tfsdk:"priority"`
	Parameter4     types.String `tfsdk:"parameter_4"`
	Parameter5     types.String `tfsdk:"parameter_5"`
	Parameter6     types.String `tfsdk:"parameter_6"`
	Parameter7     types.String `tfsdk:"parameter_7"`
	Parameter8     types.String `tfsdk:"parameter_8"`
	Parameter9     types.String `tfsdk:"parameter_9"`
	Parameter10    types.String `tfsdk:"parameter_10"`
	Parameter11    types.String `tfsdk:"parameter_11"`
	ScriptContents types.String `tfsdk:"script_contents"`
}
