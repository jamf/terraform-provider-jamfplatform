// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// PlatformEnvironmentResourceModel represents the Terraform resource model for a
// Jamf Account platform environment.
type PlatformEnvironmentResourceModel struct {
	ID       types.String           `tfsdk:"id"`
	Name     types.String           `tfsdk:"name"`
	Type     types.String           `tfsdk:"type"`
	Region   types.String           `tfsdk:"region"`
	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// platformEnvironmentIdentityModel represents the identity object for platform
// environment resources and list results.
type platformEnvironmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// PlatformEnvironmentDataSourceModel represents the Terraform data source model
// for a single platform environment looked up by identifier or name.
type PlatformEnvironmentDataSourceModel struct {
	ID       types.String             `tfsdk:"id"`
	Name     types.String             `tfsdk:"name"`
	Type     types.String             `tfsdk:"type"`
	Region   types.String             `tfsdk:"region"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// PlatformEnvironmentsDataSourceModel represents the Terraform data source model
// for the plural platform environment lookup.
type PlatformEnvironmentsDataSourceModel struct {
	ID                   types.String                              `tfsdk:"id"`
	PlatformEnvironments []PlatformEnvironmentsDataSourceResultRow `tfsdk:"platform_environments"`
	Timeouts             datasourceTimeouts.Value                  `tfsdk:"timeouts"`
}

// PlatformEnvironmentsDataSourceResultRow represents one platform environment in
// the plural data source results.
type PlatformEnvironmentsDataSourceResultRow struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Type   types.String `tfsdk:"type"`
	Region types.String `tfsdk:"region"`
}

// PlatformEnvironmentListResourceModel represents the config model for platform
// environment list queries. Jamf Account exposes no filter parameters on the
// collection, so the model carries only the list-level timeouts.
type PlatformEnvironmentListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
