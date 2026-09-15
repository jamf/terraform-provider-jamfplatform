// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package user_initiated_enrollment_settings implements the
// jamfplatform_pro_user_initiated_enrollment_settings singleton resource and
// data source. It wraps the Jamf Pro User-Initiated Enrollment settings page
// (General, Computers and Devices tabs), the embedded MDM-profile signing and
// QuickAdd developer signing certificates, and the directory-service Access
// Group collection.
package user_initiated_enrollment_settings

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required.
// Empty: the User-Initiated Enrollment settings endpoint ships at the
// provider's overall floor.
const minJamfProVersion = ""

// UserInitiatedEnrollmentSettingsResource implements the singleton Jamf Pro
// User-Initiated Enrollment settings resource.
//
// The resource is backed by an Update-only Jamf Pro API — one User-Initiated
// Enrollment settings object per tenant. Create funnels into a
// read-merge-write. Delete is state-only by design.
type UserInitiatedEnrollmentSettingsResource struct {
	client *pro.Client

	// enrollmentMu serializes writes to the shared enrollment-settings backing
	// store. The User-Initiated Enrollment settings object and the
	// Re-enrollment settings object are two views of ONE record, and this
	// resource's write is a read-merge-write that must round-trip fields it
	// does not own. Obtained by reference from the shared providerdata.Data at
	// Configure; the same *sync.Mutex instance is handed to every enrollment
	// resource.
	enrollmentMu *sync.Mutex
}

var _ resource.Resource = &UserInitiatedEnrollmentSettingsResource{}
var _ resource.ResourceWithImportState = &UserInitiatedEnrollmentSettingsResource{}
var _ resource.ResourceWithIdentity = &UserInitiatedEnrollmentSettingsResource{}
var _ resource.ResourceWithConfigValidators = &UserInitiatedEnrollmentSettingsResource{}
var _ resource.ResourceWithModifyPlan = &UserInitiatedEnrollmentSettingsResource{}

// Default timeouts.
const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 30 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 30 * time.Second
)

// NewUserInitiatedEnrollmentSettingsResource constructs the resource.
func NewUserInitiatedEnrollmentSettingsResource() resource.Resource {
	return &UserInitiatedEnrollmentSettingsResource{}
}

// Metadata sets the resource type name.
func (r *UserInitiatedEnrollmentSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_initiated_enrollment_settings"
}

// IdentitySchema defines the import identifier — singleton id only.
func (r *UserInitiatedEnrollmentSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\".",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the resource schema. Per the style guide it is kept inline and
// as flat as possible — every attribute is declared in place rather than via
// attribute-returning helpers.
func (r *UserInitiatedEnrollmentSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro **User-Initiated Enrollment** settings page (UI: Settings → Global → User-initiated enrollment): the General, Computers and Devices tabs, the third-party MDM-profile signing certificate, the QuickAdd package signing certificate, and the directory-service Access Groups. One record per tenant.\n\n" +
			"### Shared backing record\n\n" +
			"The User-Initiated Enrollment settings and the Re-enrollment settings (`jamfplatform_pro_re_enrollment_settings`) are two views of one tenant record. This resource preserves the Re-enrollment options untouched on every apply; manage those with the dedicated re-enrollment resource. Within a single Terraform run the provider serializes writes to the shared record, but two separate `terraform apply` processes against the same tenant can still race.\n\n" +
			"### Third-party MDM signing certificate\n\n" +
			"Set `signing_mdm_profile_enabled = true` and supply the `mdm_signing_certificate` block to upload a keystore. Leaving the block absent on a later apply preserves the existing certificate. Setting `signing_mdm_profile_enabled = false` removes the stored certificate. When `signing_mdm_profile_enabled = true`, the `mdm_signing_certificate` block is required, and that is checked at plan time.\n\n" +
			"### Destroy\n\n" +
			"`terraform destroy` removes the resource from Terraform state only. The settings, certificates and Access Groups are left intact on the tenant. To reset options, set them explicitly and apply before destroy.\n\n" +
			"Import with `terraform import jamfplatform_pro_user_initiated_enrollment_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			// ===== General tab =====
			"skip_certificate_installation": schema.BoolAttribute{
				MarkdownDescription: "Skip certificate installation during enrollment. Matches the \"Skip certificate installation during enrollment\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"restrict_reenrollment": schema.BoolAttribute{
				MarkdownDescription: "Restrict re-enrollment to authorized users only. Matches the \"Restrict re-enrollment to authorized users only\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"signing_mdm_profile_enabled": schema.BoolAttribute{
				MarkdownDescription: "Use a third-party signing certificate to sign the MDM profile. Matches the \"Use a third-party signing certificate\" checkbox. When `true`, supply the `mdm_signing_certificate` block (or rely on a previously-uploaded certificate). Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},

			// ===== Computers tab =====
			"enable_computer_enrollment": schema.BoolAttribute{
				MarkdownDescription: "Enable user-initiated enrollment for computers. Matches the computers \"Enable user-initiated enrollment\" toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"create_management_account": schema.BoolAttribute{
				MarkdownDescription: "Create a managed local administrator account on enrolled computers. Matches the \"Create managed local administrator account\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"management_username": schema.StringAttribute{
				MarkdownDescription: "Username for the managed local administrator account created on enrolled computers. Matches the \"Management Account\" username field. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"hide_management_account": schema.BoolAttribute{
				MarkdownDescription: "Hide the managed local administrator account on enrolled computers. Matches the \"Hide managed local administrator account\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"allow_ssh_only_management_account": schema.BoolAttribute{
				MarkdownDescription: "Allow the managed local administrator account SSH access only. Matches the \"Allow SSH access for the managed local administrator account only\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"ensure_ssh_running": schema.BoolAttribute{
				MarkdownDescription: "Ensure SSH (Remote Login) is enabled on enrolled computers. Matches the \"Ensure SSH is enabled\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"launch_self_service": schema.BoolAttribute{
				MarkdownDescription: "Launch Self Service after a computer completes enrollment. Matches the \"Launch Self Service when done\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"sign_quickadd_package": schema.BoolAttribute{
				MarkdownDescription: "Sign the QuickAdd package with a developer certificate. Matches the \"Sign QuickAdd Package\" checkbox. Supply the `developer_certificate` block to upload a signing identity. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"account_driven_device_enrollment_macos": schema.BoolAttribute{
				MarkdownDescription: "Enable Account-Driven Device Enrollment for institutionally owned computers. Matches the computers Account-Driven Device Enrollment toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},

			// ===== Devices tab =====
			"profile_driven_enrollment_via_url_institutional": schema.BoolAttribute{
				MarkdownDescription: "Enable Profile-Driven Enrollment via URL for institutionally owned mobile devices. Matches the institutional Profile-Driven Enrollment via URL toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"profile_driven_enrollment_via_url_personal": schema.BoolAttribute{
				MarkdownDescription: "Enable Profile-Driven Enrollment via URL for personally owned mobile devices. Matches the personal Profile-Driven Enrollment via URL toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"account_driven_user_enrollment": schema.BoolAttribute{
				MarkdownDescription: "Enable Account-Driven User Enrollment for mobile devices. Matches the Account-Driven User Enrollment toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"account_driven_user_enrollment_visionos": schema.BoolAttribute{
				MarkdownDescription: "Enable Account-Driven User Enrollment for Apple Vision Pro. Matches the Account-Driven User Enrollment (Apple Vision Pro) toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"merge_managed_apple_account_usernames": schema.BoolAttribute{
				MarkdownDescription: "Merge matching Managed Apple Account usernames during enrollment. Matches the \"Merge matching Managed Apple Account usernames\" checkbox. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"account_driven_device_enrollment_ios": schema.BoolAttribute{
				MarkdownDescription: "Enable Account-Driven Device Enrollment for institutionally owned mobile devices. Matches the device Account-Driven Device Enrollment toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"account_driven_device_enrollment_visionos": schema.BoolAttribute{
				MarkdownDescription: "Enable Account-Driven Device Enrollment for Apple Vision Pro. Matches the Account-Driven Device Enrollment (Apple Vision Pro) toggle. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},

			// ===== Deprecated =====
			"personal_device_enrollment_type": schema.StringAttribute{
				MarkdownDescription: "Personal-device enrollment type. Deprecated as of Jamf Pro 11.25. Jamf Pro ignores any supplied value and always reports `USERENROLLMENT`. Read-only.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			// ===== Certificates =====
			"mdm_signing_certificate": schema.SingleNestedAttribute{
				MarkdownDescription: "Third-party signing certificate used to sign the MDM enrollment profile. Required when `signing_mdm_profile_enabled = true`. Supply `keystore_file` (raw base64 of a `.p12`) and `keystore_password`; both are `WriteOnly`. Removing the block while `signing_mdm_profile_enabled` stays `true` preserves the existing certificate; setting `signing_mdm_profile_enabled = false` removes it.\n\n`keystore_password` is `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state. Bump `keystore_password_wo_version` to force the next apply to re-send the keystore and password.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"keystore_file": schema.StringAttribute{
						MarkdownDescription: "Raw base64 of the keystore file (`.p12`). Idiomatic usage: `filebase64(\"signing.p12\")`. `WriteOnly`: never persisted in Terraform state.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"keystore_file_name": schema.StringAttribute{
						MarkdownDescription: "Optional display filename for the uploaded keystore, for your own reference. Jamf Pro does not return a filename, so it round-trips from configuration only.",
						Optional:            true,
					},
					"keystore_password": schema.StringAttribute{
						MarkdownDescription: "Keystore password. `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state. Pair with `keystore_password_wo_version`, the rotation companion; bump that integer to re-send.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
						Validators: []validator.String{
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("keystore_password_wo_version")),
						},
					},
					"keystore_password_wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `keystore_password`. Bump this integer to force re-sending the keystore and password on the next apply.",
						Optional:            true,
					},
					"subject": schema.StringAttribute{
						MarkdownDescription: "Certificate subject DN. Populated by Jamf Pro while the certificate is in use; may be empty when the matching toggle is disabled.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"serial_number": schema.StringAttribute{
						MarkdownDescription: "Certificate serial number. Populated by Jamf Pro while the certificate is in use; may be empty when the matching toggle is disabled.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
				},
			},
			"developer_certificate": schema.SingleNestedAttribute{
				MarkdownDescription: "Developer signing identity used to sign the QuickAdd package when `sign_quickadd_package = true`. Supply `keystore_file` (raw base64 of a `.p12`) and `keystore_password`; both are `WriteOnly`. This path expects an Apple Developer ID signing certificate.\n\n`keystore_password` is `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state. Bump `keystore_password_wo_version` to force the next apply to re-send the keystore and password.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"keystore_file": schema.StringAttribute{
						MarkdownDescription: "Raw base64 of the keystore file (`.p12`). Idiomatic usage: `filebase64(\"signing.p12\")`. `WriteOnly`: never persisted in Terraform state.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"keystore_file_name": schema.StringAttribute{
						MarkdownDescription: "Optional display filename for the uploaded keystore, for your own reference. Jamf Pro does not return a filename, so it round-trips from configuration only.",
						Optional:            true,
					},
					"keystore_password": schema.StringAttribute{
						MarkdownDescription: "Keystore password. `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state. Pair with `keystore_password_wo_version`, the rotation companion; bump that integer to re-send.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
						Validators: []validator.String{
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("keystore_password_wo_version")),
						},
					},
					"keystore_password_wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `keystore_password`. Bump this integer to force re-sending the keystore and password on the next apply.",
						Optional:            true,
					},
					"subject": schema.StringAttribute{
						MarkdownDescription: "Certificate subject DN. Populated by Jamf Pro while the certificate is in use; may be empty when the matching toggle is disabled.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"serial_number": schema.StringAttribute{
						MarkdownDescription: "Certificate serial number. Populated by Jamf Pro while the certificate is in use; may be empty when the matching toggle is disabled.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
				},
			},

			// ===== Access Groups =====
			"access_group": schema.SetNestedAttribute{
				MarkdownDescription: "Directory-service Access Groups permitted to perform user-initiated enrollment (UI: Access tab). Each group is identified by its `name` and `ldap_server_id`; the provider resolves the directory's canonical group id for you (like the UI's \"Resolve\" action). Omit the block entirely to leave the tenant's Access Groups unmanaged. The built-in \"All Directory Service Users\" group always exists and cannot be created or removed. Declare it (with `ldap_server_id = \"-1\"`) to edit its toggles, or leave it out to keep it untouched.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Access Group identifier assigned by Jamf Pro.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Directory-service group name, exactly as it appears in the directory. Resolved against `ldap_server_id` to the directory's group id. For the built-in group use `All Directory Service Users`.",
							Required:            true,
						},
						"ldap_server_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the LDAP / directory-service server hosting the group. Use `-1` for the built-in \"All Directory Service Users\" group (no directory lookup is performed).",
							Required:            true,
						},
						"directory_service_group_id": schema.StringAttribute{
							MarkdownDescription: "Directory's canonical identifier for the group, resolved by the provider from `name` and `ldap_server_id`. Computed; do not set. For the built-in \"All Directory Service Users\" group this is `-1`.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						},
						"site_id": schema.StringAttribute{
							MarkdownDescription: "Site assigned to devices enrolled through this group. Omit to leave the current value untouched; there is no blank-clear, so set `-1` to remove the site.",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						},
						"enterprise_enrollment_enabled": schema.BoolAttribute{
							MarkdownDescription: "Allow institutional (enterprise) enrollment for members of this group. Omit to leave the current value untouched; set `true`/`false` to change it.",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
						},
						"personal_enrollment_enabled": schema.BoolAttribute{
							MarkdownDescription: "Allow personal-device enrollment for members of this group. Omit to leave the current value untouched; set `true`/`false` to change it.",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
						},
						"account_driven_user_enrollment_enabled": schema.BoolAttribute{
							MarkdownDescription: "Allow Account-Driven User Enrollment for members of this group. Omit to leave the current value untouched; set `true`/`false` to change it.",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
						},
						"require_eula": schema.BoolAttribute{
							MarkdownDescription: "Require members of this group to accept the EULA during enrollment. Jamf Pro may override the requested value depending on the other enrollment toggles, and has been observed to force `true`. When it overrides an explicitly-set value, Terraform shows a perpetual diff for this attribute. Omit to leave the current value untouched; set `true`/`false` to change it. Where Jamf Pro enforces a value, align the configuration with it.",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
						},
					},
				},
			},

			"messaging_languages": schema.MapNestedAttribute{
				MarkdownDescription: "Per-language enrollment messaging (UI: Messaging tab), keyed by ISO 639-1 language code (e.g. `fr`, `de`, `en`; a few locale variants such as `en-gb` and `zh-Hant` are also accepted). Each entry configures the text shown during user-initiated enrollment for that language. All text is displayed to the user exactly as entered. Omit the attribute entirely to leave the tenant's languages unmanaged. Only the fields you set are overridden; unset fields are seeded from the current English messaging when a language is first added, and otherwise left at their current Jamf Pro value. The built-in English language always exists, is the default shown when no language matches a device's locale, and cannot be removed. Set the `en` key to edit its messaging, or leave it out to keep it untouched. Map keys are validated at plan time against the language codes Jamf Pro recognises.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Display name of the language (e.g. `English`), resolved by the provider from the language-code key. Computed; do not set.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						},
						"page_title":                                  messagingLanguageStringAttribute("Title to display on all enrollment pages (UI: Page Title for Enrollment)."),
						"login_page_text":                             messagingLanguageStringAttribute("Text to display below the title on the login page during enrollment (UI: Login → Login Page Text)."),
						"username_text":                               messagingLanguageStringAttribute("Text to display for the username field on the login page during enrollment (UI: Login → Username Text)."),
						"password_text":                               messagingLanguageStringAttribute("Text to display for the password field on the login page during enrollment (UI: Login → Password Text)."),
						"login_button_text":                           messagingLanguageStringAttribute("Name for the button that users tap/click to log in (UI: Login → Login Button Text)."),
						"device_ownership_page_text":                  messagingLanguageStringAttribute("Text to display during enrollment that prompts the user to specify the device ownership type (UI: Device ownership → Device Ownership Page Text)."),
						"personal_device_button_name":                 messagingLanguageStringAttribute("Name for the button that users tap to enroll a personally owned device (UI: Device ownership → Personal Device Button Name)."),
						"institutional_ownership_button_name":         messagingLanguageStringAttribute("Name for the button that users tap to enroll an institutionally owned device (UI: Device ownership → Institutional Ownership Button Name)."),
						"personal_device_management_description":      messagingLanguageStringAttribute("Description to display for personal device management when users enroll a personally owned device (UI: Device ownership → Personal Device Management Description)."),
						"institutional_device_management_description": messagingLanguageStringAttribute("Description to display for institutional device management when users enroll an institutionally owned device (UI: Device ownership → Institutional Device Management Description)."),
						"enroll_device_button_name":                   messagingLanguageStringAttribute("Name for the button that users tap to start enrollment (UI: Device ownership → Enroll Device Button Name)."),
						"personal_eula":                               messagingLanguageStringAttribute("End User License Agreement to display during enrollment of personally owned devices (UI: EULA → For Personally Owned Devices)."),
						"institutional_eula":                          messagingLanguageStringAttribute("End User License Agreement to display during enrollment of institutionally owned devices and computers (UI: EULA → For Institutionally Owned Devices And Computers)."),
						"eula_accept_button_text":                     messagingLanguageStringAttribute("Name for the button that users tap/click to accept the End User License Agreement (UI: EULA → Accept Button Text)."),
						"site_selection_text":                         messagingLanguageStringAttribute("Text to display that prompts the user to select a site if the user has more than one site to choose from during enrollment (UI: Sites → Site Selection Text)."),
						"ca_certificate_installation_text":            messagingLanguageStringAttribute("Text to display when installing the CA certificate during enrollment (UI: Certificate → CA Certificate Installation Text)."),
						"ca_certificate_name":                         messagingLanguageStringAttribute("Name to display for the CA certificate during enrollment (UI: Certificate → CA Certificate Name)."),
						"ca_certificate_description":                  messagingLanguageStringAttribute("Description to display for the CA certificate during enrollment (UI: Certificate → CA Certificate Description)."),
						"ca_certificate_install_button_name":          messagingLanguageStringAttribute("Name for the button that users tap to install the CA certificate (UI: Certificate → CA Certificate Install Button Name)."),
						"institutional_mdm_installation_text":         messagingLanguageStringAttribute("Text to display when installing the MDM profile during enrollment of an institutionally owned device (UI: Institutional MDM → MDM Profile Installation Text)."),
						"institutional_mdm_profile_name":              messagingLanguageStringAttribute("Name to display for the MDM profile during enrollment of an institutionally owned device (UI: Institutional MDM → MDM Profile Name)."),
						"institutional_mdm_profile_description":       messagingLanguageStringAttribute("Description to display for the MDM profile during enrollment of an institutionally owned device (UI: Institutional MDM → MDM Profile Description)."),
						"institutional_mdm_pending_text":              messagingLanguageStringAttribute("Text to display when the user is installing the MDM profile on their computer (UI: Institutional MDM → MDM Profile Pending Page Text)."),
						"institutional_mdm_install_button_name":       messagingLanguageStringAttribute("Name for the button that users tap to install the MDM profile (UI: Institutional MDM → MDM Profile Install Button Name)."),
						"user_enrollment_mdm_installation_text":       messagingLanguageStringAttribute("Text to display when prompting to install the MDM profile (UI: User Enrollment MDM → MDM Profile Installation Text)."),
						"user_enrollment_mdm_profile_name":            messagingLanguageStringAttribute("Name to display for the MDM profile (UI: User Enrollment MDM → MDM Profile Name)."),
						"user_enrollment_mdm_profile_description":     messagingLanguageStringAttribute("Description to display for the MDM profile (UI: User Enrollment MDM → MDM Profile Description)."),
						"user_enrollment_mdm_install_button_name":     messagingLanguageStringAttribute("Name for the button that users tap to install the MDM profile (UI: User Enrollment MDM → MDM Profile Install Button Name)."),
						"quickadd_installation_text":                  messagingLanguageStringAttribute("Text to display when installing the QuickAdd package during enrollment (UI: QuickAdd → QuickAdd Package Installation Text)."),
						"quickadd_name":                               messagingLanguageStringAttribute("Name to display for the QuickAdd package during enrollment (UI: QuickAdd → QuickAdd Package Name)."),
						"quickadd_progress_text":                      messagingLanguageStringAttribute("Text to display when the QuickAdd package is downloading (UI: QuickAdd → QuickAdd Package Progress Text)."),
						"quickadd_install_button_name":                messagingLanguageStringAttribute("Name for the button that users tap to install the QuickAdd package (UI: QuickAdd → QuickAdd Package Install Button Name)."),
						"enrollment_complete_text":                    messagingLanguageStringAttribute("Text to display when enrollment is complete (UI: Complete → Enrollment Complete Text)."),
						"enrollment_failed_text":                      messagingLanguageStringAttribute("Text to display when enrollment fails (UI: Complete → Enrollment Failed Text)."),
						"try_again_button_name":                       messagingLanguageStringAttribute("Name for the button that users tap/click to try enrolling again (UI: Complete → Try Again Button Name)."),
						"view_enrollment_status_button_name":          messagingLanguageStringAttribute("Name for the button that users tap to view the enrollment status for the device (UI: Complete → View Enrollment Status Button Name)."),
						"view_enrollment_status_text":                 messagingLanguageStringAttribute("Text to display during enrollment that prompts the user to view the enrollment status for the device (UI: Complete → View Enrollment Status Text)."),
						"log_out_button_name":                         messagingLanguageStringAttribute("Name for the button that users tap/click to log out (UI: Complete → Log Out Button Name)."),
					},
				},
			},

			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// messagingLanguageStringAttribute builds an Optional+Computed string attribute
// for a messaging-language text field. Optional+Computed because the write path
// is read-merge: a field the user does not set is seeded from English (on first
// add) or preserved from the current server value, so the server resolves it.
// UseNonNullStateForUnknown is mandatory for Optional+Computed leaves inside a
// nested set (a null prior state on set growth would otherwise trip Terraform's
// "was null, now …" consistency check).
func messagingLanguageStringAttribute(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
	}
}

// ConfigValidators registers the plan-time cross-field invariants.
func (r *UserInitiatedEnrollmentSettingsResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		mdmSigningCertificateRequiredValidator{},
	}
}

// ModifyPlan validates each planned messaging_languages key (a language code)
// against the live language-codes endpoint at plan time. Because the collection
// is a map keyed by the always-known language code, Terraform correlates
// elements by key and tolerates unknown Computed leaves (resolved at apply) — so
// no plan-time leaf resolution is needed, only key validation.
//
// This needs the configured client (so it cannot be a pure schema/config
// validator) and is skipped during bare `terraform validate` (client nil) and on
// destroy (null plan).
func (r *UserInitiatedEnrollmentSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}
	var languages types.Map
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("messaging_languages"), &languages)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.validateMessagingLanguageKeys(ctx, languages, &resp.Diagnostics)
}

// Configure wires the Jamf Pro client and the shared enrollment write lock.
func (r *UserInitiatedEnrollmentSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_initiated_enrollment_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client

	// The shared enrollment write lock comes from the same providerdata.Data
	// value. The comma-ok type assertion is nil-safe: during the early-lifecycle
	// Configure call ProviderData is nil, so ok is false and the mutex stays nil
	// alongside the (also nil) client.
	if pd, ok := req.ProviderData.(*providerdata.Data); ok {
		r.enrollmentMu = pd.EnrollmentWriteLock()
	}
}

// ImportState handles import for the singleton.
func (r *UserInitiatedEnrollmentSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_user_initiated_enrollment_settings")
}
