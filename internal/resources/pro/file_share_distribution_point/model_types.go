// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// FileShareDistributionPointResourceModel represents the Terraform resource
// model for a Jamf Pro file share distribution point.
//
// The three plaintext passwords are WriteOnly: the framework never persists
// them in state, and Jamf Pro never echoes them on read, so each carries a
// companion `*_wo_version` rotation trigger (a regular Optional Int64 that the
// framework does persist).
type FileShareDistributionPointResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	Name                      types.String `tfsdk:"name"`
	ServerName                types.String `tfsdk:"server_name"`
	FileSharingConnectionType types.String `tfsdk:"file_sharing_connection_type"`

	Principal                 types.Bool   `tfsdk:"principal"`
	BackupDistributionPointID types.String `tfsdk:"backup_distribution_point_id"`
	EnableLoadBalancing       types.Bool   `tfsdk:"enable_load_balancing"`

	ShareName types.String `tfsdk:"share_name"`
	Port      types.Int64  `tfsdk:"port"`
	Workgroup types.String `tfsdk:"workgroup"`

	ReadWriteUsername         types.String `tfsdk:"read_write_username"`
	ReadWritePassword         types.String `tfsdk:"read_write_password"`
	ReadWritePasswordWoVer    types.Int64  `tfsdk:"read_write_password_wo_version"`
	ReadOnlyUsername          types.String `tfsdk:"read_only_username"`
	ReadOnlyPassword          types.String `tfsdk:"read_only_password"`
	ReadOnlyPasswordWoVersion types.Int64  `tfsdk:"read_only_password_wo_version"`

	HTTPSEnabled           types.Bool   `tfsdk:"https_enabled"`
	HTTPSPort              types.Int64  `tfsdk:"https_port"`
	HTTPSContext           types.String `tfsdk:"https_context"`
	HTTPSSecurityType      types.String `tfsdk:"https_security_type"`
	HTTPSUsername          types.String `tfsdk:"https_username"`
	HTTPSPassword          types.String `tfsdk:"https_password"`
	HTTPSPasswordWoVersion types.Int64  `tfsdk:"https_password_wo_version"`

	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// FileShareDistributionPointDataSourceModel represents the Terraform data
// source model. Read-only; the three plaintext passwords and their
// `*_wo_version` triggers are absent because Jamf Pro never echoes them.
type FileShareDistributionPointDataSourceModel struct {
	ID                        types.String `tfsdk:"id"`
	Name                      types.String `tfsdk:"name"`
	ServerName                types.String `tfsdk:"server_name"`
	FileSharingConnectionType types.String `tfsdk:"file_sharing_connection_type"`

	Principal                 types.Bool   `tfsdk:"principal"`
	BackupDistributionPointID types.String `tfsdk:"backup_distribution_point_id"`
	EnableLoadBalancing       types.Bool   `tfsdk:"enable_load_balancing"`

	ShareName types.String `tfsdk:"share_name"`
	Port      types.Int64  `tfsdk:"port"`
	Workgroup types.String `tfsdk:"workgroup"`

	ReadWriteUsername types.String `tfsdk:"read_write_username"`
	ReadOnlyUsername  types.String `tfsdk:"read_only_username"`

	HTTPSEnabled      types.Bool   `tfsdk:"https_enabled"`
	HTTPSPort         types.Int64  `tfsdk:"https_port"`
	HTTPSContext      types.String `tfsdk:"https_context"`
	HTTPSSecurityType types.String `tfsdk:"https_security_type"`
	HTTPSUsername     types.String `tfsdk:"https_username"`

	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// fileShareDistributionPointIdentityModel represents the identity object for
// resources and list results.
type fileShareDistributionPointIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// FileShareDistributionPointListResourceModel represents the config model for
// list queries.
type FileShareDistributionPointListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}
