// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// MobileAppResourceModel is the Terraform resource model for a Jamf Pro mobile
// device app (the classic /mobiledeviceapplications endpoint). Optional sections
// are pointer-typed so an undeclared block stays null in state — the server
// echoes every section on GET, and Read only refreshes a block the caller
// manages (see assignMobileAppResourceModel).
type MobileAppResourceModel struct {
	ID               types.String                      `tfsdk:"id"`
	General          *MobileAppGeneralModel            `tfsdk:"general"`
	Scope            *scope.MobileScopeModelNoIbeacons `tfsdk:"scope"`
	SelfService      *MobileAppSelfServiceModel        `tfsdk:"self_service"`
	Vpp              *MobileAppVppModel                `tfsdk:"vpp"`
	AppConfiguration *MobileAppAppConfigurationModel   `tfsdk:"app_configuration"`
	Timeouts         resourceTimeouts.Value            `tfsdk:"timeouts"`
}

// MobileAppGeneralModel models <mobile_device_application><general>. name,
// version, and bundle_id are required on create; os_type is optional (only
// in-house apps need it). description is server-managed and surfaced read-only.
// Not modeled: the app's display name (always == name); url (a deprecated mirror
// of itunes_store_url); and internal_app — a read-only flag the server flips
// based on the store/hosting inputs, so modeling it would trip a plan/apply
// inconsistency when those change (in-house status is derivable from whether a
// store/external URL is set).
type MobileAppGeneralModel struct {
	ID                               types.String `tfsdk:"id"`
	Name                             types.String `tfsdk:"name"`
	Version                          types.String `tfsdk:"version"`
	BundleID                         types.String `tfsdk:"bundle_id"`
	OsType                           types.String `tfsdk:"os_type"`
	Description                      types.String `tfsdk:"description"`
	IsFree                           types.Bool   `tfsdk:"is_free"`
	DeploymentType                   types.String `tfsdk:"deployment_type"`
	ExternalURL                      types.String `tfsdk:"external_url"`
	ItunesStoreURL                   types.String `tfsdk:"itunes_store_url"`
	ItunesCountryRegion              types.String `tfsdk:"itunes_country_region"`
	ItunesSyncTime                   types.Int64  `tfsdk:"itunes_sync_time"`
	CategoryID                       types.String `tfsdk:"category_id"`
	CategoryName                     types.String `tfsdk:"category_name"`
	SiteID                           types.String `tfsdk:"site_id"`
	SiteName                         types.String `tfsdk:"site_name"`
	MakeAvailableAfterInstall        types.Bool   `tfsdk:"make_available_after_install"`
	KeepDescriptionAndIconUpToDate   types.Bool   `tfsdk:"keep_description_and_icon_up_to_date"`
	KeepAppUpdatedOnDevices          types.Bool   `tfsdk:"keep_app_updated_on_devices"`
	DeployAsManagedApp               types.Bool   `tfsdk:"deploy_as_managed_app"`
	TakeOverManagement               types.Bool   `tfsdk:"take_over_management"`
	DeployAutomatically              types.Bool   `tfsdk:"deploy_automatically"`
	RemoveAppWhenMDMProfileIsRemoved types.Bool   `tfsdk:"remove_app_when_mdm_profile_is_removed"`
	PreventBackupOfAppData           types.Bool   `tfsdk:"prevent_backup_of_app_data"`
	AllowUserToDelete                types.Bool   `tfsdk:"allow_user_to_delete"`
	RequireNetworkTethered           types.Bool   `tfsdk:"require_network_tethered"`
	HostExternally                   types.Bool   `tfsdk:"host_externally"`
}

// MobileAppSelfServiceModel models <mobile_device_application><self_service>.
// Mobile carries the bool form of <notification> only (no method, unlike mac apps).
type MobileAppSelfServiceModel struct {
	InstallButtonText      types.String                        `tfsdk:"install_button_text"`
	AfterInstallButtonText types.String                        `tfsdk:"after_install_button_text"`
	SelfServiceDescription types.String                        `tfsdk:"self_service_description"`
	FeatureOnMainPage      types.Bool                          `tfsdk:"feature_on_main_page"`
	NotificationEnabled    types.Bool                          `tfsdk:"notification_enabled"`
	NotificationSubject    types.String                        `tfsdk:"notification_subject"`
	NotificationMessage    types.String                        `tfsdk:"notification_message"`
	SelfServiceIcon        *MobileAppSelfServiceIconModel      `tfsdk:"self_service_icon"`
	SelfServiceCategories  []MobileAppSelfServiceCategoryModel `tfsdk:"self_service_categories"`
}

// MobileAppSelfServiceIconModel models <self_service><self_service_icon>. Set id
// to reference an already-uploaded icon; uri is returned by Jamf Pro. Uploading
// icon bytes inline is not supported.
type MobileAppSelfServiceIconModel struct {
	ID  types.String `tfsdk:"id"`
	URI types.String `tfsdk:"uri"`
}

// MobileAppSelfServiceCategoryModel models a single <self_service_categories><category>.
// Mobile carries display_in but no feature_in (unlike mac apps).
type MobileAppSelfServiceCategoryModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	DisplayIn types.Bool   `tfsdk:"display_in"`
}

// MobileAppVppModel models the top-level <mobile_device_application><vpp> block.
// Mobile carries only assign_vpp_device_based_licenses and vpp_admin_account_id
// (no license-count echo fields, unlike mac apps). Writable only for a genuinely
// VPP-backed title — assigning device-based licenses to a non-VPP app 409s.
type MobileAppVppModel struct {
	AssignVppDeviceBasedLicenses types.Bool   `tfsdk:"assign_vpp_device_based_licenses"`
	VppAdminAccountID            types.String `tfsdk:"vpp_admin_account_id"`
}

// MobileAppAppConfigurationModel models <mobile_device_application><app_configuration>.
// preferences is a managed-app configuration plist; CRLF and LF newlines are
// treated as semantically equal (see the preferences plan modifier).
type MobileAppAppConfigurationModel struct {
	Preferences types.String `tfsdk:"preferences"`
}

// MobileAppDataSourceModel is the flat data source model. Surfaces a read-only
// projection of the most-frequently looked-up fields so users can resolve IDs
// by name; manage the app as a resource for full detail.
type MobileAppDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	Name           types.String             `tfsdk:"name"`
	Version        types.String             `tfsdk:"version"`
	BundleID       types.String             `tfsdk:"bundle_id"`
	OsType         types.String             `tfsdk:"os_type"`
	IsFree         types.Bool               `tfsdk:"is_free"`
	DeploymentType types.String             `tfsdk:"deployment_type"`
	CategoryID     types.String             `tfsdk:"category_id"`
	CategoryName   types.String             `tfsdk:"category_name"`
	SiteID         types.String             `tfsdk:"site_id"`
	SiteName       types.String             `tfsdk:"site_name"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// mobileAppIdentityModel represents the identity object for the resource and list results.
type mobileAppIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// MobileAppListResourceModel represents the config model for list queries. Classic
// has no RSQL — the filter shape is the shared client-side substring block.
type MobileAppListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
