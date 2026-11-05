// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// ComputerPrestageEnrollmentResourceModel is the Terraform resource model for
// a Jamf Pro Computer PreStage Enrollment.
type ComputerPrestageEnrollmentResourceModel struct {
	ID types.String `tfsdk:"id"`

	DisplayName                       types.String `tfsdk:"display_name"`
	Mandatory                         types.Bool   `tfsdk:"mandatory"`
	MdmRemovable                      types.Bool   `tfsdk:"mdm_removable"`
	DefaultPrestage                   types.Bool   `tfsdk:"default_prestage"`
	SupportPhoneNumber                types.String `tfsdk:"support_phone_number"`
	SupportEmailAddress               types.String `tfsdk:"support_email_address"`
	Department                        types.String `tfsdk:"department"`
	RequireAuthentication             types.Bool   `tfsdk:"require_authentication"`
	AuthenticationPrompt              types.String `tfsdk:"authentication_prompt"`
	DeviceEnrollmentProgramInstanceID types.String `tfsdk:"device_enrollment_program_instance_id"`
	SiteID                            types.String `tfsdk:"site_id"`
	EnrollmentSiteID                  types.String `tfsdk:"enrollment_site_id"`
	KeepExistingLocationInformation   types.Bool   `tfsdk:"keep_existing_location_information"`
	KeepExistingSiteMembership        types.Bool   `tfsdk:"keep_existing_site_membership"`
	EnrollmentCustomizationID         types.String `tfsdk:"enrollment_customization_id"`
	Language                          types.String `tfsdk:"language"`
	Region                            types.String `tfsdk:"region"`
	AutoAdvanceSetup                  types.Bool   `tfsdk:"auto_advance_setup"`
	InstallProfilesDuringSetup        types.Bool   `tfsdk:"install_profiles_during_setup"`

	PrestageInstalledProfileIds      types.Set    `tfsdk:"prestage_installed_profile_ids"`
	CustomPackageIds                 types.Set    `tfsdk:"custom_package_ids"`
	CustomPackageDistributionPointID types.String `tfsdk:"custom_package_distribution_point_id"`
	AnchorCertificates               types.List   `tfsdk:"anchor_certificates"`

	PreventActivationLock           types.Bool `tfsdk:"prevent_activation_lock"`
	EnableDeviceBasedActivationLock types.Bool `tfsdk:"enable_device_based_activation_lock"`

	EnableRecoveryLock            types.Bool   `tfsdk:"enable_recovery_lock"`
	RecoveryLockPasswordType      types.String `tfsdk:"recovery_lock_password_type"`
	RecoveryLockPassword          types.String `tfsdk:"recovery_lock_password"`
	RecoveryLockPasswordWoVersion types.Int64  `tfsdk:"recovery_lock_password_wo_version"`
	RotateRecoveryLockPassword    types.Bool   `tfsdk:"rotate_recovery_lock_password"`

	PrestageMinimumOsTargetVersionType types.String `tfsdk:"prestage_minimum_os_target_version_type"`
	MinimumOsSpecificVersion           types.String `tfsdk:"minimum_os_specific_version"`

	PssoEnabled            types.Bool   `tfsdk:"psso_enabled"`
	PlatformSsoAppBundleID types.String `tfsdk:"platform_sso_app_bundle_id"`
	PssoConfigProfileID    types.String `tfsdk:"psso_config_profile_id"`
	ProfileURL             types.String `tfsdk:"profile_url"`
	ManifestURL            types.String `tfsdk:"manifest_url"`
	ProfileUUID            types.String `tfsdk:"profile_uuid"`

	SkipSetupItems        *SkipSetupItemsModel        `tfsdk:"skip_setup_items"`
	LocationInformation   *LocationInformationModel   `tfsdk:"location_information"`
	PurchasingInformation *PurchasingInformationModel `tfsdk:"purchasing_information"`
	AccountSettings       *AccountSettingsModel       `tfsdk:"account_settings"`

	ScopeSerialNumbers types.Set `tfsdk:"scope_serial_numbers"`

	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// SkipSetupItemsModel mirrors the wire-side `skipSetupItems` map but exposes each Apple Setup
// Assistant pane as a named snake_case attribute, named after the admin UI label. The
// attribute-to-wire-key table is skipItems in skip_items.go. The last fields are deprecated
// aliases that share a wire key with an earlier one; deprecations.go names each.
type SkipSetupItemsModel struct {
	GetStarted                types.Bool `tfsdk:"get_started"`
	Location                  types.Bool `tfsdk:"location"`
	TransferInformation       types.Bool `tfsdk:"transfer_information"`
	AppleID                   types.Bool `tfsdk:"apple_id"`
	TermsAndConditions        types.Bool `tfsdk:"terms_and_conditions"`
	Siri                      types.Bool `tfsdk:"siri"`
	AppAnalytics              types.Bool `tfsdk:"app_analytics"`
	Biometric                 types.Bool `tfsdk:"biometric"`
	EnableLockdownMode        types.Bool `tfsdk:"enable_lockdown_mode"`
	Payment                   types.Bool `tfsdk:"payment"`
	Registration              types.Bool `tfsdk:"registration"`
	Privacy                   types.Bool `tfsdk:"privacy"`
	AdditionalPrivacySettings types.Bool `tfsdk:"additional_privacy_settings"`
	FileVault                 types.Bool `tfsdk:"filevault"`
	ICloudDiagnostics         types.Bool `tfsdk:"icloud_diagnostics"`
	AllYourFilesInICloud      types.Bool `tfsdk:"all_your_files_in_icloud"`
	ChooseYourLook            types.Bool `tfsdk:"choose_your_look"`
	Wallpaper                 types.Bool `tfsdk:"wallpaper"`
	TrueToneDisplay           types.Bool `tfsdk:"true_tone_display"`
	ScreenTime                types.Bool `tfsdk:"screen_time"`
	Accessibility             types.Bool `tfsdk:"accessibility"`
	TermsOfAddress            types.Bool `tfsdk:"terms_of_address"`
	Intelligence              types.Bool `tfsdk:"intelligence"`
	SoftwareUpdate            types.Bool `tfsdk:"software_update"`
	OSShowcase                types.Bool `tfsdk:"os_showcase"`
	LiquidGlass               types.Bool `tfsdk:"liquid_glass"`

	Welcome       types.Bool `tfsdk:"welcome"`
	Restore       types.Bool `tfsdk:"restore"`
	TOS           types.Bool `tfsdk:"tos"`
	Diagnostics   types.Bool `tfsdk:"diagnostics"`
	ICloudStorage types.Bool `tfsdk:"icloud_storage"`
	Appearance    types.Bool `tfsdk:"appearance"`
	DisplayTone   types.Bool `tfsdk:"display_tone"`
}

// LocationInformationModel is the User and Location Information nested block.
// Server-assigned `id` and `versionLock` are managed internally — not exposed
// to users.
type LocationInformationModel struct {
	Username     types.String `tfsdk:"username"`
	Realname     types.String `tfsdk:"realname"`
	Phone        types.String `tfsdk:"phone"`
	Email        types.String `tfsdk:"email"`
	Room         types.String `tfsdk:"room"`
	Position     types.String `tfsdk:"position"`
	BuildingID   types.String `tfsdk:"building_id"`
	DepartmentID types.String `tfsdk:"department_id"`
}

// PurchasingInformationModel is the Purchasing Information nested block.
// Server-assigned `id` and `versionLock` are managed internally — not exposed
// to users.
type PurchasingInformationModel struct {
	Leased            types.Bool   `tfsdk:"leased"`
	Purchased         types.Bool   `tfsdk:"purchased"`
	AppleCareID       types.String `tfsdk:"apple_care_id"`
	PoNumber          types.String `tfsdk:"po_number"`
	Vendor            types.String `tfsdk:"vendor"`
	PurchasePrice     types.String `tfsdk:"purchase_price"`
	LifeExpectancy    types.Int64  `tfsdk:"life_expectancy"`
	PurchasingAccount types.String `tfsdk:"purchasing_account"`
	PurchasingContact types.String `tfsdk:"purchasing_contact"`
	LeaseDate         types.String `tfsdk:"lease_date"`
	PoDate            types.String `tfsdk:"po_date"`
	WarrantyDate      types.String `tfsdk:"warranty_date"`
}

// AccountSettingsModel is the Account Settings nested block. `AdminPassword`
// is WriteOnly + `AdminPasswordWoVersion` is the rotation trigger.
// Server-assigned `id` and `versionLock` are managed internally.
type AccountSettingsModel struct {
	PayloadConfigured                       types.Bool   `tfsdk:"payload_configured"`
	LocalAdminAccountEnabled                types.Bool   `tfsdk:"local_admin_account_enabled"`
	AdminUsername                           types.String `tfsdk:"admin_username"`
	AdminPassword                           types.String `tfsdk:"admin_password"`
	AdminPasswordWoVersion                  types.Int64  `tfsdk:"admin_password_wo_version"`
	HiddenAdminAccount                      types.Bool   `tfsdk:"hidden_admin_account"`
	LocalUserManaged                        types.Bool   `tfsdk:"local_user_managed"`
	UserAccountType                         types.String `tfsdk:"user_account_type"`
	PrefillPrimaryAccountInfoFeatureEnabled types.Bool   `tfsdk:"prefill_primary_account_info_feature_enabled"`
	PrefillType                             types.String `tfsdk:"prefill_type"`
	PrefillAccountFullName                  types.String `tfsdk:"prefill_account_full_name"`
	PrefillAccountUserName                  types.String `tfsdk:"prefill_account_user_name"`
	PreventPrefillInfoFromModification      types.Bool   `tfsdk:"prevent_prefill_info_from_modification"`
}

// ComputerPrestageEnrollmentIdentityModel represents the identity object used
// for import.
type ComputerPrestageEnrollmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// ComputerPrestageEnrollmentDataSourceModel is the data source model. Mirrors
// the resource shape minus the WriteOnly secret fields, which the server
// never echoes back.
type ComputerPrestageEnrollmentDataSourceModel struct {
	ID                                 types.String             `tfsdk:"id"`
	Name                               types.String             `tfsdk:"name"`
	DisplayName                        types.String             `tfsdk:"display_name"`
	Mandatory                          types.Bool               `tfsdk:"mandatory"`
	MdmRemovable                       types.Bool               `tfsdk:"mdm_removable"`
	DefaultPrestage                    types.Bool               `tfsdk:"default_prestage"`
	SupportPhoneNumber                 types.String             `tfsdk:"support_phone_number"`
	SupportEmailAddress                types.String             `tfsdk:"support_email_address"`
	Department                         types.String             `tfsdk:"department"`
	RequireAuthentication              types.Bool               `tfsdk:"require_authentication"`
	AuthenticationPrompt               types.String             `tfsdk:"authentication_prompt"`
	DeviceEnrollmentProgramInstanceID  types.String             `tfsdk:"device_enrollment_program_instance_id"`
	SiteID                             types.String             `tfsdk:"site_id"`
	EnrollmentSiteID                   types.String             `tfsdk:"enrollment_site_id"`
	EnrollmentCustomizationID          types.String             `tfsdk:"enrollment_customization_id"`
	ProfileUUID                        types.String             `tfsdk:"profile_uuid"`
	PrestageMinimumOsTargetVersionType types.String             `tfsdk:"prestage_minimum_os_target_version_type"`
	MinimumOsSpecificVersion           types.String             `tfsdk:"minimum_os_specific_version"`
	PssoEnabled                        types.Bool               `tfsdk:"psso_enabled"`
	Timeouts                           datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// ComputerPrestageEnrollmentListResourceModel is the config model for the
// list resource. The `/v3/computer-prestages` list endpoint accepts no RSQL
// filter — exposes the shared client-side substring matcher.
type ComputerPrestageEnrollmentListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
