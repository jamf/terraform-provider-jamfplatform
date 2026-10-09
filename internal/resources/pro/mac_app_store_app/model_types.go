// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// MacAppResourceModel is the Terraform resource model for a Jamf Pro App Store
// Mac app (the classic /macapplications endpoint). Optional sections are
// pointer-typed so an undeclared block stays null in state — the server echoes
// every section on GET, and Read only refreshes a block the caller manages
// (see assignMacAppResourceModel).
type MacAppResourceModel struct {
	ID          types.String                        `tfsdk:"id"`
	General     *MacAppGeneralModel                 `tfsdk:"general"`
	Scope       *scope.ComputerScopeModelNoIbeacons `tfsdk:"scope"`
	SelfService *MacAppSelfServiceModel             `tfsdk:"self_service"`
	Vpp         *MacAppVppModel                     `tfsdk:"vpp"`
	Timeouts    resourceTimeouts.Value              `tfsdk:"timeouts"`
}

// MacAppGeneralModel models <mac_application><general>. name, version, and
// bundle_id are required on create (the server 409s otherwise) and stored
// verbatim — there is no App Store resolution.
type MacAppGeneralModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Version        types.String `tfsdk:"version"`
	BundleID       types.String `tfsdk:"bundle_id"`
	URL            types.String `tfsdk:"url"`
	IsFree         types.Bool   `tfsdk:"is_free"`
	DeploymentType types.String `tfsdk:"deployment_type"`
	CategoryID     types.String `tfsdk:"category_id"`
	CategoryName   types.String `tfsdk:"category_name"`
	SiteID         types.String `tfsdk:"site_id"`
	SiteName       types.String `tfsdk:"site_name"`
}

// MacAppSelfServiceModel models <mac_application><self_service>.
type MacAppSelfServiceModel struct {
	InstallButtonText           types.String                     `tfsdk:"install_button_text"`
	SelfServiceDescription      types.String                     `tfsdk:"self_service_description"`
	ForceUsersToViewDescription types.Bool                       `tfsdk:"force_users_to_view_description"`
	FeatureOnMainPage           types.Bool                       `tfsdk:"feature_on_main_page"`
	NotificationEnabled         types.Bool                       `tfsdk:"notification_enabled"`
	NotificationMethod          types.String                     `tfsdk:"notification_method"`
	NotificationSubject         types.String                     `tfsdk:"notification_subject"`
	NotificationMessage         types.String                     `tfsdk:"notification_message"`
	SelfServiceIcon             *MacAppSelfServiceIconModel      `tfsdk:"self_service_icon"`
	SelfServiceCategories       []MacAppSelfServiceCategoryModel `tfsdk:"self_service_categories"`
}

// MacAppSelfServiceIconModel models <self_service><self_service_icon>. The icon
// binary is uploaded out-of-band; set id to reference an already-uploaded icon
// and uri is returned by Jamf Pro. Uploading icon bytes inline is not
// supported (Jamf re-encodes PNGs server-side, which would permadiff).
type MacAppSelfServiceIconModel struct {
	ID  types.String `tfsdk:"id"`
	URI types.String `tfsdk:"uri"`
}

// MacAppSelfServiceCategoryModel models a single <self_service_categories><category>.
type MacAppSelfServiceCategoryModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	DisplayIn types.Bool   `tfsdk:"display_in"`
	FeatureIn types.Bool   `tfsdk:"feature_in"`
}

// MacAppVppModel models the top-level <mac_application><vpp> block. The license
// counts are server-computed; only assign_vpp_device_based_licenses and
// vpp_admin_account_id are writable, and only for a genuine VPP-backed title.
type MacAppVppModel struct {
	AssignVppDeviceBasedLicenses types.Bool   `tfsdk:"assign_vpp_device_based_licenses"`
	VppAdminAccountID            types.String `tfsdk:"vpp_admin_account_id"`
	TotalVppLicenses             types.Int64  `tfsdk:"total_vpp_licenses"`
	RemainingVppLicenses         types.Int64  `tfsdk:"remaining_vpp_licenses"`
	UsedVppLicenses              types.Int64  `tfsdk:"used_vpp_licenses"`
}

// MacAppDataSourceModel is the flat data source model. Surfaces a read-only
// projection of the most-frequently looked-up fields so users can resolve IDs
// by name; manage the app as a resource for full detail.
type MacAppDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	Name           types.String             `tfsdk:"name"`
	Version        types.String             `tfsdk:"version"`
	BundleID       types.String             `tfsdk:"bundle_id"`
	IsFree         types.Bool               `tfsdk:"is_free"`
	DeploymentType types.String             `tfsdk:"deployment_type"`
	CategoryID     types.String             `tfsdk:"category_id"`
	CategoryName   types.String             `tfsdk:"category_name"`
	SiteID         types.String             `tfsdk:"site_id"`
	SiteName       types.String             `tfsdk:"site_name"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// macAppIdentityModel represents the identity object for the resource and list results.
type macAppIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// MacAppListResourceModel represents the config model for list queries. Classic
// has no RSQL — the filter shape is the shared client-side substring block.
type MacAppListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
