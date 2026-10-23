// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenants

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TenantsDataSourceModel represents the Terraform data source model for the
// tenants of a Jamf Account organization.
type TenantsDataSourceModel struct {
	ID       types.String   `tfsdk:"id"`
	Tenants  []TenantModel  `tfsdk:"tenants"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

// TenantModel represents one tenant in the data source results.
type TenantModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Product               types.String `tfsdk:"product"`
	Type                  types.String `tfsdk:"type"`
	URL                   types.String `tfsdk:"url"`
	Region                types.String `tfsdk:"region"`
	PlatformEnvironmentID types.String `tfsdk:"platform_environment_id"`
}
