// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// EbookResourceModel is the Terraform resource model for a Jamf Pro ebook (the
// classic /ebooks endpoint). Optional sections are pointer-typed so an
// undeclared block stays null in state — the server echoes every section on
// GET, and Read only refreshes a block the caller manages (see
// assignEbookResourceModel).
type EbookResourceModel struct {
	ID          types.String           `tfsdk:"id"`
	General     *EbookGeneralModel     `tfsdk:"general"`
	Scope       *EbookScopeModel       `tfsdk:"scope"`
	SelfService *EbookSelfServiceModel `tfsdk:"self_service"`
	Timeouts    resourceTimeouts.Value `tfsdk:"timeouts"`
}

// EbookGeneralModel models <ebook><general>. name and url are required on
// create. file_type and version are user-set for in-house ebooks but
// server-derived for App-Store ebooks (the server resolves them from the Apple
// Books URL), so both are Optional+Computed.
type EbookGeneralModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Author          types.String `tfsdk:"author"`
	URL             types.String `tfsdk:"url"`
	DeploymentType  types.String `tfsdk:"deployment_type"`
	DeployAsManaged types.Bool   `tfsdk:"deploy_as_managed"`
	Free            types.Bool   `tfsdk:"free"`
	FileType        types.String `tfsdk:"file_type"`
	Version         types.String `tfsdk:"version"`
	CategoryID      types.String `tfsdk:"category_id"`
	CategoryName    types.String `tfsdk:"category_name"`
	SiteID          types.String `tfsdk:"site_id"`
	SiteName        types.String `tfsdk:"site_name"`
}

// EbookScopeModel is the Terraform model for an ebook <scope> block. Ebook
// scope is the classic dual-target union: computer targets AND mobile-device
// targets AND user targets, plus the ebook-specific `class_ids` target. It is
// hand-composed from the shared scope sub-block primitives rather than reusing
// the single-target computer/mobile factories, because the union+classes shape
// is ebook's own (see STYLE_GUIDE.md §Scope helper and the
// project_scope_ebook_dual_target design note). There are NO iBeacon targets
// anywhere in ebook scope. The all-flags and per-category target ID sets nest
// under `targets`, mirroring the admin UI's Targets / Limitations / Exclusions
// tabs.
type EbookScopeModel struct {
	Targets     *EbookScopeTargetsModel     `tfsdk:"targets"`
	Limitations *EbookScopeLimitationsModel `tfsdk:"limitations"`
	Exclusions  *EbookScopeExclusionsModel  `tfsdk:"exclusions"`
}

// EbookScopeTargetsModel models <scope> targets — the three all-flags plus the
// per-category ID sets (computer + mobile + user targets and the ebook-specific
// `class_ids`). Mirrors the admin UI's Targets tab.
type EbookScopeTargetsModel struct {
	AllComputers         types.Bool `tfsdk:"all_computers"`
	AllMobileDevices     types.Bool `tfsdk:"all_mobile_devices"`
	AllJssUsers          types.Bool `tfsdk:"all_jss_users"`
	ComputerIDs          types.Set  `tfsdk:"computer_ids"`
	ComputerGroupIDs     types.Set  `tfsdk:"computer_group_ids"`
	MobileDeviceIDs      types.Set  `tfsdk:"mobile_device_ids"`
	MobileDeviceGroupIDs types.Set  `tfsdk:"mobile_device_group_ids"`
	BuildingIDs          types.Set  `tfsdk:"building_ids"`
	DepartmentIDs        types.Set  `tfsdk:"department_ids"`
	UserIDs              types.Set  `tfsdk:"user_ids"`
	UserGroupIDs         types.Set  `tfsdk:"user_group_ids"`
	ClassIDs             types.Set  `tfsdk:"class_ids"`
}

// TargetsOrZero returns the targets sub-model, or a zero value with null flags
// and null sets when the block was omitted, so input-builders can read target
// fields without a nil-guard. The omission semantics in BuildIDSlice treat null
// sets as absent.
func (m EbookScopeModel) TargetsOrZero() EbookScopeTargetsModel {
	if m.Targets != nil {
		return *m.Targets
	}
	return EbookScopeTargetsModel{
		AllComputers:         types.BoolNull(),
		AllMobileDevices:     types.BoolNull(),
		AllJssUsers:          types.BoolNull(),
		ComputerIDs:          types.SetNull(types.StringType),
		ComputerGroupIDs:     types.SetNull(types.StringType),
		MobileDeviceIDs:      types.SetNull(types.StringType),
		MobileDeviceGroupIDs: types.SetNull(types.StringType),
		BuildingIDs:          types.SetNull(types.StringType),
		DepartmentIDs:        types.SetNull(types.StringType),
		UserIDs:              types.SetNull(types.StringType),
		UserGroupIDs:         types.SetNull(types.StringType),
		ClassIDs:             types.SetNull(types.StringType),
	}
}

// EbookScopeLimitationsModel models <scope><limitations>. The directory-service
// categories carry names (not IDs) because that is how Jamf Pro identifies
// these objects.
type EbookScopeLimitationsModel struct {
	NetworkSegmentIDs                types.Set `tfsdk:"network_segment_ids"`
	DirectoryServiceOrLocalUserNames types.Set `tfsdk:"directory_service_or_local_user_names"`
	DirectoryServiceUserGroupNames   types.Set `tfsdk:"directory_service_user_group_names"`
}

// EbookScopeExclusionsModel models <scope><exclusions> — the full target union
// (computer + mobile + user) plus network segments and directory-service names.
type EbookScopeExclusionsModel struct {
	ComputerIDs                      types.Set `tfsdk:"computer_ids"`
	ComputerGroupIDs                 types.Set `tfsdk:"computer_group_ids"`
	MobileDeviceIDs                  types.Set `tfsdk:"mobile_device_ids"`
	MobileDeviceGroupIDs             types.Set `tfsdk:"mobile_device_group_ids"`
	BuildingIDs                      types.Set `tfsdk:"building_ids"`
	DepartmentIDs                    types.Set `tfsdk:"department_ids"`
	UserIDs                          types.Set `tfsdk:"user_ids"`
	UserGroupIDs                     types.Set `tfsdk:"user_group_ids"`
	NetworkSegmentIDs                types.Set `tfsdk:"network_segment_ids"`
	DirectoryServiceOrLocalUserNames types.Set `tfsdk:"directory_service_or_local_user_names"`
	DirectoryServiceUserGroupNames   types.Set `tfsdk:"directory_service_user_group_names"`
}

// EbookSelfServiceModel models <ebook><self_service>.
type EbookSelfServiceModel struct {
	DisplayName                 types.String                    `tfsdk:"display_name"`
	InstallButtonText           types.String                    `tfsdk:"install_button_text"`
	SelfServiceDescription      types.String                    `tfsdk:"self_service_description"`
	ForceUsersToViewDescription types.Bool                      `tfsdk:"force_users_to_view_description"`
	FeatureOnMainPage           types.Bool                      `tfsdk:"feature_on_main_page"`
	NotificationEnabled         types.Bool                      `tfsdk:"notification_enabled"`
	NotificationMethod          types.String                    `tfsdk:"notification_method"`
	NotificationSubject         types.String                    `tfsdk:"notification_subject"`
	NotificationMessage         types.String                    `tfsdk:"notification_message"`
	IconID                      types.String                    `tfsdk:"icon_id"`
	IconURI                     types.String                    `tfsdk:"icon_uri"`
	Categories                  []EbookSelfServiceCategoryModel `tfsdk:"categories"`
}

// EbookSelfServiceCategoryModel models a single
// <self_service_categories><category>. The set is keyed by category ID.
type EbookSelfServiceCategoryModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	DisplayIn types.Bool   `tfsdk:"display_in"`
	FeatureIn types.Bool   `tfsdk:"feature_in"`
}

// EbookDataSourceModel is the flat data source model. Surfaces a read-only
// projection of the most-frequently looked-up fields so users can resolve IDs
// by name; manage the ebook as a resource for full detail.
type EbookDataSourceModel struct {
	ID              types.String             `tfsdk:"id"`
	Name            types.String             `tfsdk:"name"`
	Author          types.String             `tfsdk:"author"`
	URL             types.String             `tfsdk:"url"`
	DeploymentType  types.String             `tfsdk:"deployment_type"`
	FileType        types.String             `tfsdk:"file_type"`
	Version         types.String             `tfsdk:"version"`
	Free            types.Bool               `tfsdk:"free"`
	DeployAsManaged types.Bool               `tfsdk:"deploy_as_managed"`
	CategoryID      types.String             `tfsdk:"category_id"`
	CategoryName    types.String             `tfsdk:"category_name"`
	SiteID          types.String             `tfsdk:"site_id"`
	SiteName        types.String             `tfsdk:"site_name"`
	Timeouts        datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// ebookIdentityModel represents the identity object for the resource and list results.
type ebookIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// EbookListResourceModel represents the config model for list queries. Classic
// /ebooks has no RSQL — the filter shape is the shared client-side substring block.
type EbookListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
