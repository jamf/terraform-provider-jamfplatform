// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package category

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// CategoryResourceModel represents the Terraform resource model for a Jamf Pro category.
type CategoryResourceModel struct {
	ID       types.String           `tfsdk:"id"`
	Name     types.String           `tfsdk:"name"`
	Priority types.Int64            `tfsdk:"priority"`
	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// CategoryDataSourceModel represents the Terraform data source model for a Jamf Pro category.
type CategoryDataSourceModel struct {
	ID       types.String             `tfsdk:"id"`
	Name     types.String             `tfsdk:"name"`
	Priority types.Int64              `tfsdk:"priority"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// categoryIdentityModel represents the identity object for category resources and list results.
type categoryIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// CategoryListResourceModel represents the config model for category list queries.
type CategoryListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}

// CategoriesDataSourceModel represents the Terraform data source model for category searches.
type CategoriesDataSourceModel struct {
	ID         types.String                      `tfsdk:"id"`
	Categories []CategoriesDataSourceResultModel `tfsdk:"categories"`
	Filters    []filters.FilterModel             `tfsdk:"filter"`
	Timeouts   datasourceTimeouts.Value          `tfsdk:"timeouts"`
}

// CategoriesDataSourceResultModel represents a single category in the search results.
type CategoriesDataSourceResultModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Priority types.Int64  `tfsdk:"priority"`
}
