// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
)

// DeviceDataSource implements the Terraform data source for Jamf devices.
type DeviceDataSource struct {
	client *devices.Client
}

// DeviceDataSourceModel represents the Terraform data source model for a Jamf device lookup.
type DeviceDataSourceModel struct {
	ID                                      types.String   `tfsdk:"id"`
	SerialNumber                            types.String   `tfsdk:"serial_number"`
	Timeouts                                timeouts.Value `tfsdk:"timeouts"`
	Name                                    types.String   `tfsdk:"name"`
	Model                                   types.String   `tfsdk:"model"`
	ModelIdentifier                         types.String   `tfsdk:"model_identifier"`
	LastInventoryUpdate                     types.String   `tfsdk:"last_inventory_update_time"`
	LastCheckIn                             types.String   `tfsdk:"last_check_in_time"`
	OperatingSystemVersion                  types.String   `tfsdk:"operating_system_version"`
	OperatingSystemName                     types.String   `tfsdk:"operating_system_name"`
	OperatingSystemBuild                    types.String   `tfsdk:"operating_system_build"`
	OperatingSystemSupplementalBuildVersion types.String   `tfsdk:"operating_system_supplemental_build_version"`
	OperatingSystemRapidSecurityResponse    types.String   `tfsdk:"operating_system_rapid_security_response"`
	UserID                                  types.String   `tfsdk:"user_id"`
	EnrollmentType                          types.String   `tfsdk:"enrollment_type"`
	LastEnrollmentTime                      types.String   `tfsdk:"last_enrollment_time"`
	Managed                                 types.Bool     `tfsdk:"managed"`
	MDMCapable                              types.Bool     `tfsdk:"mdm_capable"`
	Supervised                              types.Bool     `tfsdk:"supervised"`
	HardwareMake                            types.String   `tfsdk:"hardware_make"`
	HardwareUDID                            types.String   `tfsdk:"hardware_udid"`
	HardwareBatteryHealth                   types.String   `tfsdk:"hardware_battery_health"`
	HardwareMacAddress                      types.String   `tfsdk:"hardware_mac_address"`
	HardwareStorageCapacity                 types.Int64    `tfsdk:"hardware_storage_capacity"`
	HardwareStorageUsed                     types.Int64    `tfsdk:"hardware_storage_used"`
	NetworkLastIPAddress                    types.String   `tfsdk:"network_last_ip_address"`
	NetworkLastReportedIPv4Address          types.String   `tfsdk:"network_last_reported_ipv4_address"`
	NetworkLastReportedIPv6Address          types.String   `tfsdk:"network_last_reported_ipv6_address"`
	SecurityBootstrapTokenEscrowedStatus    types.String   `tfsdk:"security_bootstrap_token_escrowed_status"`
	SecurityHardwareEncryption              types.Bool     `tfsdk:"security_hardware_encryption"`
	SecurityPasscodePresent                 types.Bool     `tfsdk:"security_passcode_present"`
	SecurityPasscodeCompliant               types.Bool     `tfsdk:"security_passcode_compliant"`
	SecurityLostModeEnabled                 types.Bool     `tfsdk:"security_lost_mode_enabled"`
}
