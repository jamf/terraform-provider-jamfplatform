// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package devices

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	devSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// DevicesDataSource implements the Terraform data source for Jamf devices.
type DevicesDataSource struct {
	client *devSDK.Client
}

// DevicesDataSourceModel represents the Terraform data source model for Jamf devices lookups.
type DevicesDataSourceModel struct {
	ID       types.String          `tfsdk:"id"`
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts timeouts.Value        `tfsdk:"timeouts"`
	Devices  []DevicesListEntry    `tfsdk:"devices"`
}

// DeviceListEntry models a single device entry in the devices data source.
type DevicesListEntry struct {
	ID                     types.String `tfsdk:"id"`
	SerialNumber           types.String `tfsdk:"serial_number"`
	Name                   types.String `tfsdk:"name"`
	Model                  types.String `tfsdk:"model"`
	ModelIdentifier        types.String `tfsdk:"model_identifier"`
	LastInventoryUpdate    types.String `tfsdk:"last_inventory_update_time"`
	LastCheckIn            types.String `tfsdk:"last_check_in_time"`
	OperatingSystemVersion types.String `tfsdk:"operating_system_version"`
	UserID                 types.String `tfsdk:"user_id"`
	EnrollmentType         types.String `tfsdk:"enrollment_type"`
	LastEnrollmentTime     types.String `tfsdk:"last_enrollment_time"`
}
