// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// ZtnaAppResourceModel represents the Terraform resource model for a Jamf
// Security Cloud ZTNA access policy application.
type ZtnaAppResourceModel struct {
	ID                  types.String           `tfsdk:"id"`
	Name                types.String           `tfsdk:"name"`
	PredefinedAppID     types.String           `tfsdk:"predefined_app_id"`
	AppType             types.String           `tfsdk:"app_type"`
	Category            types.String           `tfsdk:"category"`
	Hostnames           types.Set              `tfsdk:"hostnames"`
	DirectIPsAndSubnets types.Set              `tfsdk:"direct_ips_and_subnets"`
	AllDeviceGroups     types.Bool             `tfsdk:"all_device_groups"`
	DeviceGroupIDs      types.Set              `tfsdk:"device_group_ids"`
	Routing             *RoutingModel          `tfsdk:"routing"`
	RoutingOverrides    types.List             `tfsdk:"routing_overrides"`
	Security            *SecurityModel         `tfsdk:"security"`
	Timeouts            resourceTimeouts.Value `tfsdk:"timeouts"`
}

// RoutingModel represents a routing block, used both for the app's default
// routing and inside each per-group override.
type RoutingModel struct {
	TrafficRouting types.String `tfsdk:"traffic_routing"`
	GatewayID      types.String `tfsdk:"gateway_id"`
	RoutingMode    types.String `tfsdk:"routing_mode"`
}

// RoutingOverrideModel represents one per-group routing override on the resource.
type RoutingOverrideModel struct {
	DeviceGroupIDs types.Set     `tfsdk:"device_group_ids"`
	Routing        *RoutingModel `tfsdk:"routing"`
}

// SecurityModel represents the security block, one member per card in the admin
// UI's Security tab.
type SecurityModel struct {
	ManagedDevice *SecurityControlModel `tfsdk:"managed_device"`
	DeviceRisk    *DeviceRiskModel      `tfsdk:"device_risk"`
	JamfTrust     *SecurityControlModel `tfsdk:"jamf_trust"`
}

// SecurityControlModel represents a security card carrying only a toggle and its
// notification setting.
type SecurityControlModel struct {
	Enabled                 types.Bool `tfsdk:"enabled"`
	DevicePushNotifications types.Bool `tfsdk:"device_push_notifications"`
}

// DeviceRiskModel represents the device risk card, which adds the risk level at
// which denial begins.
type DeviceRiskModel struct {
	Enabled                 types.Bool   `tfsdk:"enabled"`
	DenyAtRiskLevel         types.String `tfsdk:"deny_at_risk_level"`
	DevicePushNotifications types.Bool   `tfsdk:"device_push_notifications"`
}

// ztnaAppIdentityModel represents the identity object for ZTNA app resources and
// list results.
type ztnaAppIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// ZtnaAppDataSourceModel represents the Terraform data source model for a single
// Jamf Security Cloud ZTNA access policy application. Collections are lists
// rather than sets because data source attributes returning API data are always
// read-only.
type ZtnaAppDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	Name                types.String             `tfsdk:"name"`
	PredefinedAppID     types.String             `tfsdk:"predefined_app_id"`
	AppType             types.String             `tfsdk:"app_type"`
	Category            types.String             `tfsdk:"category"`
	Hostnames           types.List               `tfsdk:"hostnames"`
	DirectIPsAndSubnets types.List               `tfsdk:"direct_ips_and_subnets"`
	AllDeviceGroups     types.Bool               `tfsdk:"all_device_groups"`
	DeviceGroupIDs      types.List               `tfsdk:"device_group_ids"`
	Routing             types.Object             `tfsdk:"routing"`
	RoutingOverrides    types.List               `tfsdk:"routing_overrides"`
	Security            types.Object             `tfsdk:"security"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// ZtnaAppsDataSourceModel represents the Terraform data source model for the
// plural ZTNA app lookup.
type ZtnaAppsDataSourceModel struct {
	ID       types.String                    `tfsdk:"id"`
	ZtnaApps []ZtnaAppsDataSourceResultModel `tfsdk:"ztna_apps"`
	Timeouts datasourceTimeouts.Value        `tfsdk:"timeouts"`
}

// ZtnaAppsDataSourceResultModel represents a single app in the plural data source
// results.
type ZtnaAppsDataSourceResultModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	PredefinedAppID     types.String `tfsdk:"predefined_app_id"`
	AppType             types.String `tfsdk:"app_type"`
	Category            types.String `tfsdk:"category"`
	Hostnames           types.List   `tfsdk:"hostnames"`
	DirectIPsAndSubnets types.List   `tfsdk:"direct_ips_and_subnets"`
	AllDeviceGroups     types.Bool   `tfsdk:"all_device_groups"`
	DeviceGroupIDs      types.List   `tfsdk:"device_group_ids"`
	Routing             types.Object `tfsdk:"routing"`
	RoutingOverrides    types.List   `tfsdk:"routing_overrides"`
	Security            types.Object `tfsdk:"security"`
}

// ZtnaAppListResourceModel represents the config model for ZTNA app list queries.
// Jamf Security Cloud exposes no filter parameters on the app list endpoint, so
// the model carries only the `timeouts` attribute.
type ZtnaAppListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
