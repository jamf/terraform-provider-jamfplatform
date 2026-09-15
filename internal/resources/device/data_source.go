// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultReadTimeout = 30 * time.Second

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &DeviceDataSource{}

// NewDeviceDataSource returns a new instance of DeviceDataSource.
func NewDeviceDataSource() datasource.DataSource {
	return &DeviceDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DeviceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device"
}

// Schema defines the data source schema.
func (d *DeviceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Jamf device by ID. Requires **Device Inventory API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Device UUID (Jamf Pro Management ID) to query.",
				Required:            true,
			},
			"serial_number": schema.StringAttribute{
				MarkdownDescription: "Device serial number reported by inventory (case-sensitive).",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Device name reported by inventory.",
				Computed:            true,
			},
			"model": schema.StringAttribute{
				MarkdownDescription: "Marketing model string.",
				Computed:            true,
			},
			"model_identifier": schema.StringAttribute{
				MarkdownDescription: "Model identifier (e.g., Mac14,6).",
				Computed:            true,
			},
			"last_inventory_update_time": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last inventory update in ISO 8601 format.",
				Computed:            true,
			},
			"last_check_in_time": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last check-in in ISO 8601 format.",
				Computed:            true,
			},
			"operating_system_version": schema.StringAttribute{
				MarkdownDescription: "Operating system version string.",
				Computed:            true,
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "User ID associated with the device, if any.",
				Computed:            true,
			},
			"enrollment_type": schema.StringAttribute{
				MarkdownDescription: "Enrollment type reported by the platform.",
				Computed:            true,
			},
			"last_enrollment_time": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last enrollment in ISO 8601 format.",
				Computed:            true,
			},
			"managed": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the device is managed.",
				Computed:            true,
			},
			"mdm_capable": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the device is MDM capable.",
				Computed:            true,
			},
			"supervised": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the device is supervised.",
				Computed:            true,
			},
			"hardware_make": schema.StringAttribute{
				MarkdownDescription: "Hardware make from the detailed device record.",
				Computed:            true,
			},
			"hardware_udid": schema.StringAttribute{
				MarkdownDescription: "Hardware UDID reported by inventory.",
				Computed:            true,
			},
			"hardware_battery_health": schema.StringAttribute{
				MarkdownDescription: "Battery health reported by inventory.",
				Computed:            true,
			},
			"hardware_mac_address": schema.StringAttribute{
				MarkdownDescription: "Primary hardware MAC address.",
				Computed:            true,
			},
			"hardware_storage_capacity": schema.Int64Attribute{
				MarkdownDescription: "Total storage capacity in bytes.",
				Computed:            true,
			},
			"hardware_storage_used": schema.Int64Attribute{
				MarkdownDescription: "Used storage in bytes.",
				Computed:            true,
			},
			"network_last_ip_address": schema.StringAttribute{
				MarkdownDescription: "Last IP address reported for the device.",
				Computed:            true,
			},
			"network_last_reported_ipv4_address": schema.StringAttribute{
				MarkdownDescription: "Last reported IPv4 address.",
				Computed:            true,
			},
			"network_last_reported_ipv6_address": schema.StringAttribute{
				MarkdownDescription: "Last reported IPv6 address.",
				Computed:            true,
			},
			"operating_system_name": schema.StringAttribute{
				MarkdownDescription: "Operating system name.",
				Computed:            true,
			},
			"operating_system_build": schema.StringAttribute{
				MarkdownDescription: "Operating system build string.",
				Computed:            true,
			},
			"operating_system_supplemental_build_version": schema.StringAttribute{
				MarkdownDescription: "Supplemental build version, if provided.",
				Computed:            true,
			},
			"operating_system_rapid_security_response": schema.StringAttribute{
				MarkdownDescription: "Rapid Security Response build identifier, if provided.",
				Computed:            true,
			},
			"security_bootstrap_token_escrowed_status": schema.StringAttribute{
				MarkdownDescription: "Bootstrap token escrow status.",
				Computed:            true,
			},
			"security_hardware_encryption": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether hardware encryption is enabled.",
				Computed:            true,
			},
			"security_passcode_present": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether a passcode is present.",
				Computed:            true,
			},
			"security_passcode_compliant": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the passcode complies with policy.",
				Computed:            true,
			},
			"security_lost_mode_enabled": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether lost mode is enabled.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *DeviceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_device", providerdata.DevicesScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = devices.New(pd.Client)
}

// Read fetches a device by ID and populates the Terraform state.
func (d *DeviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DeviceDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	lookupID := ""
	if helpers.IsConfiguredValue(data.ID) {
		lookupID = data.ID.ValueString()
	}
	if lookupID == "" {
		resp.Diagnostics.AddError(
			"Missing device id",
			"The id attribute must be provided to read a device.",
		)
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	deviceDetail, err := d.client.GetDevice(readCtx, lookupID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read device", fmt.Sprintf("Failed to get device %s: %s", lookupID, helpers.APIErrorDetail(err)))
		return
	}

	serialValue := ""
	if deviceDetail.Hardware != nil && deviceDetail.Hardware.SerialNumber != "" {
		serialValue = deviceDetail.Hardware.SerialNumber
	}

	modelValue := ""
	if deviceDetail.Hardware != nil && deviceDetail.Hardware.Model != "" {
		modelValue = deviceDetail.Hardware.Model
	}

	modelIdentifierValue := ""
	if deviceDetail.Hardware != nil && deviceDetail.Hardware.ModelIdentifier != "" {
		modelIdentifierValue = deviceDetail.Hardware.ModelIdentifier
	}

	operatingSystemVersion := ""
	if deviceDetail.OperatingSystem != nil && deviceDetail.OperatingSystem.Version != "" {
		operatingSystemVersion = deviceDetail.OperatingSystem.Version
	}

	data.ID = types.StringValue(deviceDetail.ID)
	data.SerialNumber = helpers.StringValueOrNull(serialValue)
	data.Name = helpers.StringValueOrNull(deviceDetail.Name)
	data.Model = helpers.StringValueOrNull(modelValue)
	data.ModelIdentifier = helpers.StringValueOrNull(modelIdentifierValue)
	if deviceDetail.LastInventoryUpdateTime != nil {
		data.LastInventoryUpdate = types.StringValue(deviceDetail.LastInventoryUpdateTime.Format(time.RFC3339))
	} else {
		data.LastInventoryUpdate = types.StringNull()
	}
	if deviceDetail.LastCheckInTime != nil {
		data.LastCheckIn = types.StringValue(deviceDetail.LastCheckInTime.Format(time.RFC3339))
	} else {
		data.LastCheckIn = types.StringNull()
	}
	data.OperatingSystemVersion = helpers.StringValueOrNull(operatingSystemVersion)
	if deviceDetail.OperatingSystem != nil {
		data.OperatingSystemName = helpers.StringValueOrNull(deviceDetail.OperatingSystem.Name)
		data.OperatingSystemBuild = helpers.StringValueOrNull(deviceDetail.OperatingSystem.Build)
		data.OperatingSystemSupplementalBuildVersion = helpers.StringPointerValueOrNull(deviceDetail.OperatingSystem.SupplementalBuildVersion)
		data.OperatingSystemRapidSecurityResponse = helpers.StringPointerValueOrNull(deviceDetail.OperatingSystem.RapidSecurityResponse)
	} else {
		data.OperatingSystemName = types.StringNull()
		data.OperatingSystemBuild = types.StringNull()
		data.OperatingSystemSupplementalBuildVersion = types.StringNull()
		data.OperatingSystemRapidSecurityResponse = types.StringNull()
	}
	data.UserID = helpers.StringPointerValueOrNull(deviceDetail.UserID)
	data.EnrollmentType = helpers.StringValueOrNull(deviceDetail.EnrollmentType)
	if deviceDetail.LastEnrollmentTime != nil {
		data.LastEnrollmentTime = types.StringValue(deviceDetail.LastEnrollmentTime.Format(time.RFC3339))
	} else {
		data.LastEnrollmentTime = types.StringNull()
	}
	data.Managed = types.BoolValue(deviceDetail.Managed)
	data.MDMCapable = types.BoolValue(deviceDetail.MDMCapable)
	data.Supervised = types.BoolValue(deviceDetail.Supervised)

	if deviceDetail.Hardware != nil {
		data.HardwareMake = helpers.StringValueOrNull(deviceDetail.Hardware.Make)
		data.HardwareUDID = helpers.StringValueOrNull(deviceDetail.Hardware.UDID)
		data.HardwareBatteryHealth = helpers.StringValueOrNull(deviceDetail.Hardware.BatteryHealth)
		data.HardwareMacAddress = helpers.StringValueOrNull(deviceDetail.Hardware.MacAddress)
		data.HardwareStorageCapacity = types.Int64Value(int64(deviceDetail.Hardware.StorageCapacity))
		data.HardwareStorageUsed = types.Int64Value(int64(deviceDetail.Hardware.StorageUsed))
	} else {
		data.HardwareMake = types.StringNull()
		data.HardwareUDID = types.StringNull()
		data.HardwareBatteryHealth = types.StringNull()
		data.HardwareMacAddress = types.StringNull()
		data.HardwareStorageCapacity = types.Int64Null()
		data.HardwareStorageUsed = types.Int64Null()
	}

	if deviceDetail.Network != nil {
		data.NetworkLastIPAddress = helpers.StringPointerValueOrNull(deviceDetail.Network.LastIPAddress)
		data.NetworkLastReportedIPv4Address = helpers.StringPointerValueOrNull(deviceDetail.Network.LastReportedIPV4Address)
		data.NetworkLastReportedIPv6Address = helpers.StringPointerValueOrNull(deviceDetail.Network.LastReportedIPV6Address)
	} else {
		data.NetworkLastIPAddress = types.StringNull()
		data.NetworkLastReportedIPv4Address = types.StringNull()
		data.NetworkLastReportedIPv6Address = types.StringNull()
	}

	if deviceDetail.Security != nil {
		data.SecurityBootstrapTokenEscrowedStatus = helpers.StringValueOrNull(deviceDetail.Security.BootstrapTokenEscrowedStatus)
		data.SecurityHardwareEncryption = helpers.BoolPointerValueOrNull(deviceDetail.Security.HardwareEncryption)
		data.SecurityPasscodePresent = helpers.BoolPointerValueOrNull(deviceDetail.Security.PasscodePresent)
		data.SecurityPasscodeCompliant = helpers.BoolPointerValueOrNull(deviceDetail.Security.PasscodeCompliant)
		data.SecurityLostModeEnabled = helpers.BoolPointerValueOrNull(deviceDetail.Security.LostModeEnabled)
	} else {
		data.SecurityBootstrapTokenEscrowedStatus = types.StringNull()
		data.SecurityHardwareEncryption = types.BoolNull()
		data.SecurityPasscodePresent = types.BoolNull()
		data.SecurityPasscodeCompliant = types.BoolNull()
		data.SecurityLostModeEnabled = types.BoolNull()
	}

	tflog.Trace(ctx, "read device data source", map[string]any{
		"id": deviceDetail.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
