// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// MobileDevicePrestageEnrollmentResourceModel is the Terraform resource model
// for a Jamf Pro Mobile Device PreStage Enrollment. There are no WriteOnly
// secrets and no _wo_version companions — mobile prestages carry no admin
// password / account settings block (spike §6).
type MobileDevicePrestageEnrollmentResourceModel struct {
	ID types.String `tfsdk:"id"`

	DisplayName                       types.String `tfsdk:"display_name"`
	DeviceEnrollmentProgramInstanceID types.String `tfsdk:"device_enrollment_program_instance_id"`

	Mandatory                           types.Bool `tfsdk:"mandatory"`
	MdmRemovable                        types.Bool `tfsdk:"mdm_removable"`
	RequireAuthentication               types.Bool `tfsdk:"require_authentication"`
	Supervised                          types.Bool `tfsdk:"supervised"`
	AllowPairing                        types.Bool `tfsdk:"allow_pairing"`
	AutoAdvanceSetup                    types.Bool `tfsdk:"auto_advance_setup"`
	ConfigureDeviceBeforeSetupAssistant types.Bool `tfsdk:"configure_device_before_setup_assistant"`
	DefaultPrestage                     types.Bool `tfsdk:"default_prestage"`
	SendTimezone                        types.Bool `tfsdk:"send_timezone"`
	PreventActivationLock               types.Bool `tfsdk:"prevent_activation_lock"`
	EnableDeviceBasedActivationLock     types.Bool `tfsdk:"enable_device_based_activation_lock"`
	KeepExistingSiteMembership          types.Bool `tfsdk:"keep_existing_site_membership"`
	KeepExistingLocationInformation     types.Bool `tfsdk:"keep_existing_location_information"`
	MultiUser                           types.Bool `tfsdk:"multi_user"`
	UseStorageQuotaSize                 types.Bool `tfsdk:"use_storage_quota_size"`
	TemporarySessionOnly                types.Bool `tfsdk:"temporary_session_only"`
	EnforceTemporarySessionTimeout      types.Bool `tfsdk:"enforce_temporary_session_timeout"`
	EnforceUserSessionTimeout           types.Bool `tfsdk:"enforce_user_session_timeout"`
	PreserveManagedApps                 types.Bool `tfsdk:"preserve_managed_apps"`
	DoNotUseProfileFromBackup           types.Bool `tfsdk:"do_not_use_profile_from_backup"`
	InstallAppsDuringEnrollment         types.Bool `tfsdk:"install_apps_during_enrollment"`

	AuthenticationPrompt      types.String `tfsdk:"authentication_prompt"`
	SupportPhoneNumber        types.String `tfsdk:"support_phone_number"`
	SupportEmailAddress       types.String `tfsdk:"support_email_address"`
	Department                types.String `tfsdk:"department"`
	Timezone                  types.String `tfsdk:"timezone"`
	Language                  types.String `tfsdk:"language"`
	Region                    types.String `tfsdk:"region"`
	EnrollmentSiteID          types.String `tfsdk:"enrollment_site_id"`
	EnrollmentCustomizationID types.String `tfsdk:"enrollment_customization_id"`
	RtsConfigProfileID        types.String `tfsdk:"rts_config_profile_id"`
	RtsEnabled                types.Bool   `tfsdk:"rts_enabled"`

	MaximumSharedAccounts     types.Int64 `tfsdk:"maximum_shared_accounts"`
	TemporarySessionTimeout   types.Int64 `tfsdk:"temporary_session_timeout"`
	UserSessionTimeout        types.Int64 `tfsdk:"user_session_timeout"`
	StorageQuotaSizeMegabytes types.Int64 `tfsdk:"storage_quota_size_megabytes"`

	PrestageMinimumOsTargetVersionTypeIos  types.String `tfsdk:"prestage_minimum_os_target_version_type_ios"`
	PrestageMinimumOsTargetVersionTypeIpad types.String `tfsdk:"prestage_minimum_os_target_version_type_ipad"`
	MinimumOsSpecificVersionIos            types.String `tfsdk:"minimum_os_specific_version_ios"`
	MinimumOsSpecificVersionIpad           types.String `tfsdk:"minimum_os_specific_version_ipad"`

	AnchorCertificates types.List `tfsdk:"anchor_certificates"`

	ProfileUUID types.String `tfsdk:"profile_uuid"`
	SiteID      types.String `tfsdk:"site_id"`

	SkipSetupItems        *SkipSetupItemsModel        `tfsdk:"skip_setup_items"`
	Names                 *NamesModel                 `tfsdk:"names"`
	LocationInformation   *LocationInformationModel   `tfsdk:"location_information"`
	PurchasingInformation *PurchasingInformationModel `tfsdk:"purchasing_information"`

	ScopeSerialNumbers types.Set `tfsdk:"scope_serial_numbers"`

	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// NamesModel is the device-naming nested block (spike §4.2). The
// `prestage_device_names` list element `id`/`used` are server-managed but echoed
// on the wire so the PUT round-trips. The wire block's deviceNamingConfigured is
// unmodelled and derived at write time — see namingIntended.
type NamesModel struct {
	AssignNamesUsing    types.String              `tfsdk:"assign_names_using"`
	ManageNames         types.Bool                `tfsdk:"manage_names"`
	DeviceNamePrefix    types.String              `tfsdk:"device_name_prefix"`
	DeviceNameSuffix    types.String              `tfsdk:"device_name_suffix"`
	SingleDeviceName    types.String              `tfsdk:"single_device_name"`
	PrestageDeviceNames []PrestageDeviceNameModel `tfsdk:"prestage_device_names"`
}

// PrestageDeviceNameModel is one element of the `prestage_device_names` list.
// `id` and `used` are Computed but MUST serialise on the wire (§F4b).
type PrestageDeviceNameModel struct {
	DeviceName types.String `tfsdk:"device_name"`
	ID         types.String `tfsdk:"id"`
	Used       types.Bool   `tfsdk:"used"`
}

// SkipSetupItemsModel mirrors the wire-side `skipSetupItems` map but exposes
// each Apple Setup Assistant pane as a named snake_case attribute. Boundary
// translation (wire-case ⇔ snake_case) lives in the build / flatten / diff
// functions. 45 keys (§F12).
type SkipSetupItemsModel struct {
	ActionButton          types.Bool `tfsdk:"action_button"`
	Android               types.Bool `tfsdk:"android"`
	Appearance            types.Bool `tfsdk:"appearance"`
	AppleID               types.Bool `tfsdk:"apple_id"`
	Biometric             types.Bool `tfsdk:"biometric"`
	CameraButton          types.Bool `tfsdk:"camera_button"`
	CloudStorage          types.Bool `tfsdk:"cloud_storage"`
	Diagnostics           types.Bool `tfsdk:"diagnostics"`
	DisplayTone           types.Bool `tfsdk:"display_tone"`
	EnableLockdownMode    types.Bool `tfsdk:"enable_lockdown_mode"`
	ExpressLanguage       types.Bool `tfsdk:"express_language"`
	HomeButtonSensitivity types.Bool `tfsdk:"home_button_sensitivity"`
	Intelligence          types.Bool `tfsdk:"intelligence"`
	Keyboard              types.Bool `tfsdk:"keyboard"`
	Location              types.Bool `tfsdk:"location"`
	Multitasking          types.Bool `tfsdk:"multitasking"`
	OSShowcase            types.Bool `tfsdk:"os_showcase"`
	OnBoarding            types.Bool `tfsdk:"onboarding"`
	Passcode              types.Bool `tfsdk:"passcode"`
	Payment               types.Bool `tfsdk:"payment"`
	PreferredLanguage     types.Bool `tfsdk:"preferred_language"`
	Privacy               types.Bool `tfsdk:"privacy"`
	Restore               types.Bool `tfsdk:"restore"`
	RestoreCompleted      types.Bool `tfsdk:"restore_completed"`
	SIMSetup              types.Bool `tfsdk:"sim_setup"`
	Safety                types.Bool `tfsdk:"safety"`
	SafetyAndHandling     types.Bool `tfsdk:"safety_and_handling"`
	ScreenSaver           types.Bool `tfsdk:"screen_saver"`
	ScreenTime            types.Bool `tfsdk:"screen_time"`
	Siri                  types.Bool `tfsdk:"siri"`
	SoftwareUpdate        types.Bool `tfsdk:"software_update"`
	SpokenLanguage        types.Bool `tfsdk:"spoken_language"`
	TOS                   types.Bool `tfsdk:"tos"`
	TVHomeScreenSync      types.Bool `tfsdk:"tv_home_screen_sync"`
	TVProviderSignIn      types.Bool `tfsdk:"tv_provider_sign_in"`
	TVRoom                types.Bool `tfsdk:"tv_room"`
	TapToSetup            types.Bool `tfsdk:"tap_to_setup"`
	TermsOfAddress        types.Bool `tfsdk:"terms_of_address"`
	TransferData          types.Bool `tfsdk:"transfer_data"`
	UpdateCompleted       types.Bool `tfsdk:"update_completed"`
	VoiceSelection        types.Bool `tfsdk:"voice_selection"`
	WatchMigration        types.Bool `tfsdk:"watch_migration"`
	Welcome               types.Bool `tfsdk:"welcome"`
	Zoom                  types.Bool `tfsdk:"zoom"`
	IMessageAndFaceTime   types.Bool `tfsdk:"imessage_and_facetime"`
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

// MobileDevicePrestageEnrollmentIdentityModel represents the identity object
// used for import.
type MobileDevicePrestageEnrollmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// MobileDevicePrestageEnrollmentDataSourceModel is the data source model.
// Mirrors a scalar subset of the resource shape.
type MobileDevicePrestageEnrollmentDataSourceModel struct {
	ID                                     types.String             `tfsdk:"id"`
	Name                                   types.String             `tfsdk:"name"`
	DisplayName                            types.String             `tfsdk:"display_name"`
	DeviceEnrollmentProgramInstanceID      types.String             `tfsdk:"device_enrollment_program_instance_id"`
	Mandatory                              types.Bool               `tfsdk:"mandatory"`
	MdmRemovable                           types.Bool               `tfsdk:"mdm_removable"`
	DefaultPrestage                        types.Bool               `tfsdk:"default_prestage"`
	RequireAuthentication                  types.Bool               `tfsdk:"require_authentication"`
	Supervised                             types.Bool               `tfsdk:"supervised"`
	SupportPhoneNumber                     types.String             `tfsdk:"support_phone_number"`
	SupportEmailAddress                    types.String             `tfsdk:"support_email_address"`
	Department                             types.String             `tfsdk:"department"`
	AuthenticationPrompt                   types.String             `tfsdk:"authentication_prompt"`
	SiteID                                 types.String             `tfsdk:"site_id"`
	EnrollmentSiteID                       types.String             `tfsdk:"enrollment_site_id"`
	EnrollmentCustomizationID              types.String             `tfsdk:"enrollment_customization_id"`
	ProfileUUID                            types.String             `tfsdk:"profile_uuid"`
	Timezone                               types.String             `tfsdk:"timezone"`
	MultiUser                              types.Bool               `tfsdk:"multi_user"`
	PrestageMinimumOsTargetVersionTypeIos  types.String             `tfsdk:"prestage_minimum_os_target_version_type_ios"`
	PrestageMinimumOsTargetVersionTypeIpad types.String             `tfsdk:"prestage_minimum_os_target_version_type_ipad"`
	Timeouts                               datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// MobileDevicePrestageEnrollmentListResourceModel is the config model for the
// list resource. The `/v3/mobile-device-prestages` list endpoint accepts no
// RSQL filter — exposes the shared client-side substring matcher.
type MobileDevicePrestageEnrollmentListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
