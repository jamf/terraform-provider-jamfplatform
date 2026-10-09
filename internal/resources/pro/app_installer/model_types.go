// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// AppInstallerResourceModel is the Terraform resource model for an App Installer
// deployment. notification_settings and self_service_settings are pointer-typed
// so an undeclared block stays null in state — the server echoes both blocks
// populated on read, and Read only refreshes a block the caller manages (see
// assignAppInstallerResourceModel).
type AppInstallerResourceModel struct {
	ID                              types.String               `tfsdk:"id"`
	Name                            types.String               `tfsdk:"name"`
	Enabled                         types.Bool                 `tfsdk:"enabled"`
	AppTitleName                    types.String               `tfsdk:"app_title_name"`
	AppTitleID                      types.String               `tfsdk:"app_title_id"`
	DeploymentType                  types.String               `tfsdk:"deployment_type"`
	UpdateBehavior                  types.String               `tfsdk:"update_behavior"`
	SelectedVersion                 types.String               `tfsdk:"selected_version"`
	LatestAvailableVersion          types.String               `tfsdk:"latest_available_version"`
	TitleAvailableInAis             types.Bool                 `tfsdk:"title_available_in_ais"`
	VersionRemoved                  types.Bool                 `tfsdk:"version_removed"`
	CategoryID                      types.String               `tfsdk:"category_id"`
	SiteID                          types.String               `tfsdk:"site_id"`
	SmartGroupID                    types.String               `tfsdk:"smart_group_id"`
	InstallPredefinedConfigProfiles types.Bool                 `tfsdk:"install_predefined_config_profiles"`
	TriggerAdminNotifications       types.Bool                 `tfsdk:"trigger_admin_notifications"`
	NotificationSettings            *NotificationSettingsModel `tfsdk:"notification_settings"`
	SelfServiceSettings             *SelfServiceSettingsModel  `tfsdk:"self_service_settings"`
	Timeouts                        resourceTimeouts.Value     `tfsdk:"timeouts"`
}

// NotificationSettingsModel models the end-user notification block. Every field
// is round-tripped on write — the block is full-replace, so a managed block must
// emit a complete payload.
type NotificationSettingsModel struct {
	NotificationMessage  types.String `tfsdk:"notification_message"`
	NotificationInterval types.Int64  `tfsdk:"notification_interval"`
	DeadlineMessage      types.String `tfsdk:"deadline_message"`
	Deadline             types.Int64  `tfsdk:"deadline"`
	QuitDelay            types.Int64  `tfsdk:"quit_delay"`
	CompleteMessage      types.String `tfsdk:"complete_message"`
	Relaunch             types.Bool   `tfsdk:"relaunch"`
	Suppress             types.Bool   `tfsdk:"suppress"`
}

// SelfServiceSettingsModel models the Self Service presentation block. Like the
// notification block it is full-replace; categories is keyed by category id.
type SelfServiceSettingsModel struct {
	Description                 types.String               `tfsdk:"description"`
	ForceViewDescription        types.Bool                 `tfsdk:"force_view_description"`
	IncludeInComplianceCategory types.Bool                 `tfsdk:"include_in_compliance_category"`
	IncludeInFeaturedCategory   types.Bool                 `tfsdk:"include_in_featured_category"`
	Categories                  []SelfServiceCategoryModel `tfsdk:"categories"`
}

// SelfServiceCategoryModel models one Self Service category membership.
type SelfServiceCategoryModel struct {
	CategoryID types.String `tfsdk:"category_id"`
	Featured   types.Bool   `tfsdk:"featured"`
}

// AppInstallerDataSourceModel is the singular data source model. It projects the
// flat scalar fields of a single deployment; the managed nested blocks
// (notification_settings, self_service_settings) are intentionally omitted —
// manage the deployment as a resource for full detail.
type AppInstallerDataSourceModel struct {
	ID                              types.String   `tfsdk:"id"`
	Name                            types.String   `tfsdk:"name"`
	Enabled                         types.Bool     `tfsdk:"enabled"`
	AppTitleName                    types.String   `tfsdk:"app_title_name"`
	AppTitleID                      types.String   `tfsdk:"app_title_id"`
	DeploymentType                  types.String   `tfsdk:"deployment_type"`
	UpdateBehavior                  types.String   `tfsdk:"update_behavior"`
	SelectedVersion                 types.String   `tfsdk:"selected_version"`
	LatestAvailableVersion          types.String   `tfsdk:"latest_available_version"`
	TitleAvailableInAis             types.Bool     `tfsdk:"title_available_in_ais"`
	VersionRemoved                  types.Bool     `tfsdk:"version_removed"`
	CategoryID                      types.String   `tfsdk:"category_id"`
	SiteID                          types.String   `tfsdk:"site_id"`
	SmartGroupID                    types.String   `tfsdk:"smart_group_id"`
	InstallPredefinedConfigProfiles types.Bool     `tfsdk:"install_predefined_config_profiles"`
	TriggerAdminNotifications       types.Bool     `tfsdk:"trigger_admin_notifications"`
	Timeouts                        timeouts.Value `tfsdk:"timeouts"`
}

// appInstallerIdentityModel represents the identity object for the resource and
// list results.
type appInstallerIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// AppInstallerListResourceModel represents the config model for list queries.
// The deployments endpoint has no server-side filter, so the shared client-side
// substring block is applied locally.
type AppInstallerListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}

// AppInstallersDataSourceModel is the plural data source model. `deployments`
// carries the expanded list shape; `name_substring` narrows it client-side (the
// deployments endpoint has no server-side filter).
type AppInstallersDataSourceModel struct {
	ID            types.String      `tfsdk:"id"`
	NameSubstring types.String      `tfsdk:"name_substring"`
	Deployments   []DeploymentModel `tfsdk:"deployments"`
	Timeouts      timeouts.Value    `tfsdk:"timeouts"`
}

// DeploymentModel is one expanded deployment list entry.
type DeploymentModel struct {
	ID               types.String           `tfsdk:"id"`
	Name             types.String           `tfsdk:"name"`
	Enabled          types.Bool             `tfsdk:"enabled"`
	DeploymentType   types.String           `tfsdk:"deployment_type"`
	UpdateBehavior   types.String           `tfsdk:"update_behavior"`
	App              *AppModel              `tfsdk:"app"`
	Site             *NamedRefModel         `tfsdk:"site"`
	Category         *NamedRefModel         `tfsdk:"category"`
	SmartGroup       *NamedRefModel         `tfsdk:"smart_group"`
	ComputerStatuses *ComputerStatusesModel `tfsdk:"computer_statuses"`
}

// AppModel is the resolved catalog app reference on a list entry.
type AppModel struct {
	ID                  types.String `tfsdk:"id"`
	BundleID            types.String `tfsdk:"bundle_id"`
	IconURL             types.String `tfsdk:"icon_url"`
	LatestVersion       types.String `tfsdk:"latest_version"`
	SelectedVersion     types.String `tfsdk:"selected_version"`
	DeployedVersion     types.String `tfsdk:"deployed_version"`
	MediaSourceType     types.String `tfsdk:"media_source_type"`
	TitleAvailableInAis types.Bool   `tfsdk:"title_available_in_ais"`
	VersionRemoved      types.Bool   `tfsdk:"version_removed"`
}

// NamedRefModel is a resolved {id, name} reference (site / category / smart group).
type NamedRefModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

// ComputerStatusesModel carries the per-deployment computer status counts.
type ComputerStatusesModel struct {
	Available   types.Int64 `tfsdk:"available"`
	Failed      types.Int64 `tfsdk:"failed"`
	InProgress  types.Int64 `tfsdk:"in_progress"`
	Installed   types.Int64 `tfsdk:"installed"`
	Unqualified types.Int64 `tfsdk:"unqualified"`
}
