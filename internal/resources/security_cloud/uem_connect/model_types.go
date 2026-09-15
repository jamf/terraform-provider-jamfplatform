// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// UEMConnectResource implements the Terraform resource for the Jamf Security
// Cloud UEM Connect integration.
type UEMConnectResource struct {
	client *securitycloud.Client
}

// UEMConnectDataSource implements the Terraform data source for the Jamf Security
// Cloud UEM Connect integration.
type UEMConnectDataSource struct {
	client *securitycloud.Client
}

// UEMConnectResourceModel represents the Terraform resource model for the UEM
// Connect integration.
//
// The nested blocks are typed pointers rather than types.Object so that
// "block absent" is expressible as nil, which is what selects the authentication
// form — see the package doc comment.
type UEMConnectResourceModel struct {
	ID                            types.String           `tfsdk:"id"`
	UEMVendor                     types.String           `tfsdk:"uem_vendor"`
	UEMServerURL                  types.String           `tfsdk:"uem_server_url"`
	PlatformTenant                *PlatformTenantModel   `tfsdk:"platform_tenant"`
	OAuth                         *OAuthModel            `tfsdk:"oauth"`
	Enabled                       types.Bool             `tfsdk:"enabled"`
	ScheduledSyncEnabled          types.Bool             `tfsdk:"scheduled_sync_enabled"`
	SyncRefreshIntervalMinutes    types.Int64            `tfsdk:"sync_refresh_interval_minutes"`
	UEMAutoDeleteBehaviour        types.String           `tfsdk:"uem_auto_delete_behavior"`
	UnmanagedSyncThreshold        types.Int64            `tfsdk:"unmanaged_sync_threshold"`
	DeviceRiskUEMSignalingEnabled types.Bool             `tfsdk:"device_risk_uem_signaling_enabled"`
	DisableSyncOnAuthError        types.Bool             `tfsdk:"disable_sync_on_auth_error"`
	ConcurrentDeviceSyncEnabled   types.Bool             `tfsdk:"concurrent_device_sync_enabled"`
	UserDataFieldMapping          *DataFieldMappingModel `tfsdk:"user_data_field_mapping"`
	GroupMembershipMapping        *GroupMappingModel     `tfsdk:"group_membership_mapping"`
	Timeouts                      resourceTimeouts.Value `tfsdk:"timeouts"`
}

// PlatformTenantModel names the Jamf Pro tenant Jamf Security Cloud provisions
// its own credentials on.
type PlatformTenantModel struct {
	TenantID types.String `tfsdk:"tenant_id"`
}

// OAuthModel carries credentials for an API integration the operator created on
// the target Jamf Pro instance themselves.
type OAuthModel struct {
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecret          types.String `tfsdk:"client_secret"`
	ClientSecretWOVersion types.Int64  `tfsdk:"client_secret_wo_version"`
}

// DataFieldMappingModel controls which Jamf Pro attribute each Jamf Security
// Cloud device field is populated from.
type DataFieldMappingModel struct {
	DeviceName  types.String       `tfsdk:"device_name"`
	UserName    types.String       `tfsdk:"user_name"`
	UserID      types.String       `tfsdk:"user_id"`
	PhoneNumber types.String       `tfsdk:"phone_number"`
	Email       *EmailMappingModel `tfsdk:"email"`
}

// EmailMappingModel describes how a device's email address is derived.
type EmailMappingModel struct {
	Source             types.String `tfsdk:"source"`
	Prefix             types.String `tfsdk:"prefix"`
	Suffix             types.String `tfsdk:"suffix"`
	OnlyIfEmailMissing types.Bool   `tfsdk:"only_if_email_missing"`
}

// GroupMappingModel links Jamf Pro groups to Jamf Security Cloud device groups.
type GroupMappingModel struct {
	Enabled                     types.Bool               `tfsdk:"enabled"`
	DefaultSecurityCloudGroupID types.String             `tfsdk:"default_security_cloud_group_id"`
	Mappings                    []GroupMappingEntryModel `tfsdk:"mappings"`
}

// GroupMappingEntryModel is one Jamf Pro group to Jamf Security Cloud group
// assignment. Order is significant: membership is evaluated top to bottom.
type GroupMappingEntryModel struct {
	UEMGroupID           types.String `tfsdk:"uem_group_id"`
	SecurityCloudGroupID types.String `tfsdk:"security_cloud_group_id"`
}

// uemConnectIdentityModel represents the identity object for UEM Connect
// resources.
type uemConnectIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// UEMConnectDataSourceModel represents the Terraform data source model for the
// UEM Connect integration.
//
// It carries the observed state the resource deliberately leaves out — whether
// the integration is currently connected, the Jamf Pro version behind it, and the
// most recent sync — because those move without any configuration change and a
// Computed attribute over them would report drift on every refresh. Collections
// are lists rather than sets: data source attributes are always read-only.
type UEMConnectDataSourceModel struct {
	ID                            types.String             `tfsdk:"id"`
	UEMVendor                     types.String             `tfsdk:"uem_vendor"`
	UEMServerURL                  types.String             `tfsdk:"uem_server_url"`
	PlatformTenantID              types.String             `tfsdk:"platform_tenant_id"`
	ClientID                      types.String             `tfsdk:"client_id"`
	Enabled                       types.Bool               `tfsdk:"enabled"`
	Connected                     types.Bool               `tfsdk:"connected"`
	JamfProVersion                types.String             `tfsdk:"jamf_pro_version"`
	ScheduledSyncEnabled          types.Bool               `tfsdk:"scheduled_sync_enabled"`
	SyncRefreshIntervalMinutes    types.Int64              `tfsdk:"sync_refresh_interval_minutes"`
	UEMAutoDeleteBehaviour        types.String             `tfsdk:"uem_auto_delete_behavior"`
	UnmanagedSyncThreshold        types.Int64              `tfsdk:"unmanaged_sync_threshold"`
	DeviceRiskUEMSignalingEnabled types.Bool               `tfsdk:"device_risk_uem_signaling_enabled"`
	DisableSyncOnAuthError        types.Bool               `tfsdk:"disable_sync_on_auth_error"`
	ConcurrentDeviceSyncEnabled   types.Bool               `tfsdk:"concurrent_device_sync_enabled"`
	UserDataFieldMapping          types.Object             `tfsdk:"user_data_field_mapping"`
	GroupMembershipMapping        types.Object             `tfsdk:"group_membership_mapping"`
	LatestSync                    types.Object             `tfsdk:"latest_sync"`
	Timeouts                      datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// UEMConnectListResourceModel represents the config model for UEM Connect list
// queries. A tenant holds at most one integration and the list endpoint exposes no
// parameters, so the model carries no fields.
type UEMConnectListResourceModel struct{}
