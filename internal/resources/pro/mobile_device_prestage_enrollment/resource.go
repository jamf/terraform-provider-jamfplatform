// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mobile_device_prestage_enrollment implements the
// jamfplatform_pro_mobile_device_prestage_enrollment resource, data source,
// and list resource backed by the Jamf Pro Mobile Device PreStage Enrollment
// API (`pro.*MobileDevicePrestageV3` + scope V2 endpoint family).
package mobile_device_prestage_enrollment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: defer to the provider-wide floor — V3 predates the
// provider's overall minimum (matches the enrollment-domain siblings).
const minJamfProVersion = ""

const (
	defaultCreateTimeout = 5 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 5 * time.Minute
	defaultDeleteTimeout = 60 * time.Second
)

// MobileDevicePrestageEnrollmentResource implements the Terraform resource.
type MobileDevicePrestageEnrollmentResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &MobileDevicePrestageEnrollmentResource{}
	_ resource.ResourceWithImportState = &MobileDevicePrestageEnrollmentResource{}
	_ resource.ResourceWithIdentity    = &MobileDevicePrestageEnrollmentResource{}
	_ resource.ResourceWithModifyPlan  = &MobileDevicePrestageEnrollmentResource{}
)

// NewMobileDevicePrestageEnrollmentResource returns a new resource instance.
func NewMobileDevicePrestageEnrollmentResource() resource.Resource {
	return &MobileDevicePrestageEnrollmentResource{}
}

// Metadata sets the resource type name.
func (r *MobileDevicePrestageEnrollmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_prestage_enrollment"
}

// IdentitySchema defines the identifier used for import.
func (r *MobileDevicePrestageEnrollmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro mobile device PreStage enrollment ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *MobileDevicePrestageEnrollmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro mobile device PreStage enrollment, the iOS/iPadOS/tvOS Automated Device Enrollment (ADE) record at *Devices → PreStage Enrollments* in the Jamf Pro admin UI. " +
			"Device scope (`scope_serial_numbers`) is folded into this resource; serial numbers must exist on the underlying ADE token or Jamf Pro rejects the assignment." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Mobile device PreStage enrollment ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Required, and must not be blank.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"device_enrollment_program_instance_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Automated Device Enrollment (ADE) instance that backs this PreStage. Required.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},

			"mandatory":              optBool("**\"Make MDM Profile Mandatory\"** in the Jamf Pro admin UI."),
			"mdm_removable":          optBool("**\"Allow MDM Profile Removal\"** in the Jamf Pro admin UI."),
			"require_authentication": optBool("**\"Require Authentication\"** in the Jamf Pro admin UI."),
			"supervised":             optBool("**\"Supervised\"** in the Jamf Pro admin UI."),
			"allow_pairing":          optBool("**\"Allow Pairing\"** in the Jamf Pro admin UI."),
			"auto_advance_setup":     optBool("**\"Auto Advance Setup\"** (tvOS) in the Jamf Pro admin UI."),
			"configure_device_before_setup_assistant": optBool("**\"Configure device before Setup Assistant\"** in the Jamf Pro admin UI."),
			// Jamf Pro allows at most one default PreStage per device type.
			// Setting this to true is honored only when no other PreStage is
			// currently the default; if another already holds it the write is
			// REJECTED with 400 ALREADY_DEFAULT and nothing is stored — it is NOT
			// silently kept false. diagnoseAlreadyDefault turns that into an
			// actionable diagnostic, and ModifyPlan deliberately leaves this
			// attribute alone (see the note there).
			"default_prestage": schema.BoolAttribute{
				MarkdownDescription: "When true, this PreStage becomes the tenant default for new devices. Jamf Pro allows at most one default PreStage and will not move the default automatically: if another PreStage already holds it, the apply fails. Set `default_prestage = false` on the current holder first. Where both are managed by Terraform, make the release land before the claim: apply it on its own, or add a `depends_on` from this resource to the one giving up the default.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"send_timezone":                       optBool("**\"Send Time Zone\"** in the Jamf Pro admin UI."),
			"prevent_activation_lock":             optBool("**\"Prevent user from enabling Activation Lock\"** in the Jamf Pro admin UI."),
			"enable_device_based_activation_lock": optBool("**\"Enable Device-Based Activation Lock\"** in the Jamf Pro admin UI."),
			"keep_existing_site_membership":       optBool("**\"Keep Existing Site Membership\"** in the Jamf Pro admin UI."),
			"keep_existing_location_information":  optBool("**\"Keep Existing Location Information\"** in the Jamf Pro admin UI."),
			"multi_user": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable Shared iPad\"** in the Jamf Pro admin UI. Requires both `supervised = true` and `prevent_activation_lock = true`. Jamf Pro rejects Shared iPad otherwise: `prevent_activation_lock` fails with a hard error, and `supervised` silently disables Shared iPad.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Bool{
					multiUserRequiresPreventActivationLock(),
					multiUserRequiresSupervised(),
				},
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"use_storage_quota_size": schema.BoolAttribute{
				MarkdownDescription: "**\"Use Storage Quota Size\"** shared-iPad storage mode. Mutually exclusive with `temporary_session_only`: Jamf Pro forces this to `false` when `temporary_session_only = true`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Bool{
					storageQuotaConflictsWithTemporarySession(),
				},
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"temporary_session_only":            optBool("**\"Temporary Session Only\"** shared-iPad storage mode. Mutually exclusive with `use_storage_quota_size`: Jamf Pro forces `use_storage_quota_size` to `false` when this is `true`."),
			"enforce_temporary_session_timeout": optBool("**\"Enforce Temporary Session Timeout\"** in the Jamf Pro admin UI."),
			"enforce_user_session_timeout":      optBool("**\"Enforce User Session Timeout\"** in the Jamf Pro admin UI."),
			"preserve_managed_apps":             optBool("**\"Preserve Managed Apps\"** in the Jamf Pro admin UI."),
			"do_not_use_profile_from_backup":    optBool("**\"Do not use profile from backup\"** in the Jamf Pro admin UI."),
			"install_apps_during_enrollment":    optBool("**\"Install managed apps before Setup Assistant\"** in the Jamf Pro admin UI."),
			"rts_enabled":                       optBool("**\"Return to Service\"** enabled toggle."),

			"authentication_prompt": optString("**\"Authentication Prompt\"** message shown when `require_authentication = true`."),
			"support_phone_number":  optString("**\"Support Phone Number\"** in the Jamf Pro admin UI."),
			"support_email_address": optString("**\"Support Email Address\"** in the Jamf Pro admin UI."),
			"department":            optString("**\"Department\"** label shown during Setup Assistant. Free-form text; *not* the department ID (`location_information.department_id`)."),
			"timezone": schema.StringAttribute{
				// Required + non-empty: the Jamf Pro API always validates this
				// field (the SDK serialises it with no omitempty) and rejects an
				// empty string with `[INVALID_CONTENT] timezone: Not a valid
				// timezone`. Required blocks null; LengthAtLeast(1) blocks "".
				// IANA validity is checked plan-time by the shared
				// validators.IANATimeZone() (Go's embedded tzdata — see its doc
				// comment for the wire-probe evidence on why tzdata, not the
				// curated /v1/time-zones list, is the gate).
				MarkdownDescription: "**\"Time Zone\"** in the Jamf Pro admin UI (e.g. `\"America/Chicago\"`). Required. Must be a valid IANA time-zone identifier, and may not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					validators.IANATimeZone(),
				},
			},
			"language": optString("Default Setup Assistant language (ISO-639 code, e.g. `\"en\"`). Invalid values are silently coerced to empty by Jamf Pro."),
			"region":   optString("Default Setup Assistant region (ISO-3166 code, e.g. `\"US\"`). Invalid values are silently coerced to empty by Jamf Pro."),

			"enrollment_site_id":          optString("Site ID applied to devices enrolled through this PreStage. Sentinel `\"-1\"` = no site."),
			"enrollment_customization_id": optString("Enrollment customization ID to apply during Setup Assistant. Sentinel `\"0\"` = no customization (note: `\"0\"`, not `\"-1\"`). Invalid IDs are silently coerced to `\"0\"`."),
			"rts_config_profile_id":       optString("Return to Service configuration profile ID. Sentinel `\"-1\"` = none."),

			"maximum_shared_accounts": optInt64("**\"Number of users\"** for Shared iPad."),
			"temporary_session_timeout": schema.Int64Attribute{
				MarkdownDescription: "**\"Temporary Session Timeout\"** (minutes). With enforcement on, Jamf Pro silently discards a value below the UI minimum of 30.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
					temporarySessionTimeoutMinimum(),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"user_session_timeout": optInt64("**\"User Session Timeout\"** (minutes)."),

			// storage_quota_size_megabytes is READ-ONLY (Computed). The Jamf Pro
			// REST API recalculates it to a device-capacity floor on every PUT,
			// ignoring any submitted value (spike §F8), so a settable attribute
			// would perpetually diff. Set it in the Jamf Pro admin UI (which
			// uses a separate legacy endpoint). The provider sends the §F3 POST
			// floor on create and otherwise reflects the server value.
			"storage_quota_size_megabytes": schema.Int64Attribute{
				MarkdownDescription: "**\"Storage Quota Size\"** (megabytes) for Shared iPad. Read-only, because Jamf Pro recalculates it on every change. Set it in the Jamf Pro admin UI; this attribute reflects the value Jamf Pro reports.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},

			"prestage_minimum_os_target_version_type_ios": schema.StringAttribute{
				MarkdownDescription: "Minimum-iOS enforcement mode. One of `NO_ENFORCEMENT`, `MINIMUM_OS_LATEST_VERSION`, `MINIMUM_OS_LATEST_MAJOR_VERSION`, `MINIMUM_OS_LATEST_MINOR_VERSION`, `MINIMUM_OS_SPECIFIC_VERSION`. Pair `MINIMUM_OS_SPECIFIC_VERSION` with `minimum_os_specific_version_ios`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(prestageMinimumOsTargetVersionValues...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"prestage_minimum_os_target_version_type_ipad": schema.StringAttribute{
				MarkdownDescription: "Minimum-iPadOS enforcement mode. One of `NO_ENFORCEMENT`, `MINIMUM_OS_LATEST_VERSION`, `MINIMUM_OS_LATEST_MAJOR_VERSION`, `MINIMUM_OS_LATEST_MINOR_VERSION`, `MINIMUM_OS_SPECIFIC_VERSION`. Pair `MINIMUM_OS_SPECIFIC_VERSION` with `minimum_os_specific_version_ipad`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(prestageMinimumOsTargetVersionValues...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"minimum_os_specific_version_ios":  optString("Specific minimum iOS version (e.g. `\"17.1\"`). Used only when `prestage_minimum_os_target_version_type_ios = \"MINIMUM_OS_SPECIFIC_VERSION\"`."),
			"minimum_os_specific_version_ipad": optString("Specific minimum iPadOS version (e.g. `\"17.1\"`). Used only when `prestage_minimum_os_target_version_type_ipad = \"MINIMUM_OS_SPECIFIC_VERSION\"`."),

			// Jamf Pro validates certificate content on save; supplying
			// invalid PEM bytes silently rolls the whole save back (HTTP 500
			// with empty errors[] on the wire). The update path detects this
			// via a post-write diff and surfaces a hard error.
			"anchor_certificates": schema.ListAttribute{
				MarkdownDescription: "Ordered list of base64-encoded PEM certificates to embed in the PreStage. Each entry must be a valid X.509 certificate in PEM format. Jamf Pro rejects a malformed entry by silently discarding the whole change; the provider catches that and raises a hard error.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listUseStateForUnknown(),
				},
			},

			"profile_uuid": schema.StringAttribute{
				MarkdownDescription: "MDM profile UUID assigned by Jamf Pro; not user-settable. Populates asynchronously after create.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site ID that owns this PreStage. Returned by Jamf Pro; not user-settable on this resource. Use `enrollment_site_id` to drive site assignment for devices enrolled through this PreStage. Jamf Pro reports `\"-1\"` when no site is set.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"skip_setup_items":       skipSetupItemsSchema(),
			"names":                  namesSchema(),
			"location_information":   locationInformationSchema(),
			"purchasing_information": purchasingInformationSchema(),

			// Scope is managed via the Pro V2 scope endpoint
			// (/v2/mobile-device-prestages/{id}/scope). The provider always
			// rewrites the full set; partial add/remove is not supported by
			// the underlying SDK call used here.
			"scope_serial_numbers": schema.SetAttribute{
				MarkdownDescription: "Set of device serial numbers assigned to this PreStage. Each serial must exist on the underlying ADE token. The full set is rewritten on every change. " +
					"Jamf Pro enforces single-PreStage-per-serial: assigning a serial that is currently scoped to a different PreStage is rejected with `ALREADY_SCOPED` and there is no transparent reassignment. To move a serial between PreStages, first remove it from the holding PreStage. A serial that does not exist on the ADE token is rejected with `DEVICE_DOES_NOT_EXIST_ON_TOKEN`.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
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

// Configure wires the Jamf Pro client into the resource.
func (r *MobileDevicePrestageEnrollmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_prestage_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro PreStage enrollment ID.
func (r *MobileDevicePrestageEnrollmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// optBool / optString / optInt64 are local helpers for the repetitive
// Optional+Computed scalar shape used across the schema.
func optBool(md string) schema.Attribute {
	return schema.BoolAttribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

func optString(md string) schema.Attribute {
	return schema.StringAttribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

func optInt64(md string) schema.Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Int64{
			int64planmodifier.UseStateForUnknown(),
		},
		Validators: []validator.Int64{
			int64validator.AtLeast(0),
		},
	}
}

// skipSetupItemsSchema returns the SingleNestedAttribute for the
// Setup-Assistant pane-skip checklist (45 keys, §F12). Per STYLE_GUIDE:
// typed-pointer model ⇒ block is Optional-only; inner fields are
// Optional+Computed.
func skipSetupItemsSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Setup Assistant panes to skip during enrolment. Each attribute corresponds to a Setup Assistant pane; `true` skips the pane. Supply the block, even empty (`skip_setup_items = {}`), to manage this section. Omitting it produces drift on the next refresh.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"action_button":           optBool("**\"Action Button\"** pane."),
			"android":                 optBool("**\"Move from Android\"** pane."),
			"appearance":              optBool("**\"Appearance\"** pane."),
			"apple_id":                optBool("**\"Apple ID\"** pane."),
			"biometric":               optBool("**\"Biometric\"** pane (Face ID / Touch ID)."),
			"camera_button":           optBool("**\"Camera Button\"** pane."),
			"cloud_storage":           optBool("**\"iCloud Storage\"** pane."),
			"diagnostics":             optBool("**\"Diagnostics\"** pane."),
			"display_tone":            optBool("**\"True Tone\"** pane."),
			"enable_lockdown_mode":    optBool("**\"Enable Lockdown Mode\"** pane."),
			"express_language":        optBool("**\"Express Language\"** pane."),
			"home_button_sensitivity": optBool("**\"Home Button Sensitivity\"** pane."),
			"intelligence":            optBool("**\"Apple Intelligence\"** pane."),
			"keyboard":                optBool("**\"Keyboard\"** pane."),
			"location":                optBool("**\"Location Services\"** pane."),
			"multitasking":            optBool("**\"Multitasking\"** pane."),
			"os_showcase":             optBool("**\"OS Showcase\"** pane."),
			"onboarding":              optBool("**\"Onboarding\"** pane."),
			"passcode":                optBool("**\"Passcode\"** pane."),
			"payment":                 optBool("**\"Apple Pay\"** pane."),
			"preferred_language":      optBool("**\"Preferred Language\"** pane."),
			"privacy":                 optBool("**\"Privacy\"** pane."),
			"restore":                 optBool("**\"Restore from backup\"** pane."),
			"restore_completed":       optBool("**\"Restore Completed\"** pane."),
			"sim_setup":               optBool("**\"SIM Setup\"** pane."),
			"safety":                  optBool("**\"Safety\"** pane."),
			"safety_and_handling":     optBool("**\"Safety and Handling\"** pane."),
			"screen_saver":            optBool("**\"Screen Saver\"** pane."),
			"screen_time":             optBool("**\"Screen Time\"** pane."),
			"siri":                    optBool("**\"Siri\"** pane."),
			"software_update":         optBool("**\"Software Update\"** pane."),
			"spoken_language":         optBool("**\"Spoken Language\"** pane."),
			"tos":                     optBool("**\"Terms of Service\"** pane."),
			"tv_home_screen_sync":     optBool("**\"TV Home Screen Sync\"** pane."),
			"tv_provider_sign_in":     optBool("**\"TV Provider Sign In\"** pane."),
			"tv_room":                 optBool("**\"TV Room\"** pane."),
			"tap_to_setup":            optBool("**\"Tap to Setup\"** pane."),
			"terms_of_address":        optBool("**\"Terms of Address\"** pane."),
			"transfer_data":           optBool("**\"Transfer Data\"** pane."),
			"update_completed":        optBool("**\"Update Completed\"** pane."),
			"voice_selection":         optBool("**\"Voice Selection\"** pane."),
			"watch_migration":         optBool("**\"Watch Migration\"** pane."),
			"welcome":                 optBool("**\"Welcome\"** pane."),
			"zoom":                    optBool("**\"Zoom\"** pane."),
			"imessage_and_facetime":   optBool("**\"iMessage & FaceTime\"** pane."),
		},
	}
}

// namesSchema returns the device-naming block (spike §4.2). The novel surface
// with no computer analog.
func namesSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"Mobile device names\"** in the Jamf Pro admin UI. Supply the block, even empty (`names = {}`), to manage device naming. Omitting it produces drift on the next refresh, because Jamf Pro always returns a populated block. A bare `names = {}` leaves device naming *unconfigured*, which is how the Jamf Pro admin UI decides not to show naming at all. Set at least one field below for naming to take effect.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"assign_names_using": schema.StringAttribute{
				MarkdownDescription: "How device names are assigned. One of `\"Default Names\"`, `\"List of Names\"`, `\"Serial Numbers\"`, `\"Single Name\"`. These UI-label strings are the literal values Jamf Pro stores.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(assignNamesUsingValues...),
					singleNameRequiresSingleDeviceName(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"manage_names":       optBool("**\"Enforce Mobile Device Names\"** in the Jamf Pro admin UI."),
			"device_name_prefix": optString("Device name prefix (used in `\"Serial Numbers\"` mode)."),
			"device_name_suffix": optString("Device name suffix (used in `\"Serial Numbers\"` mode)."),
			"single_device_name": optString("Single device name (used in `\"Single Name\"` mode). Required when `assign_names_using = \"Single Name\"`."),
			// deviceNamingConfigured is not exposed: it adds nothing the sibling
			// attributes don't already say. It is still WRITTEN — buildNames
			// derives it via namingIntended. Do not drop it from the wire body:
			// the Jamf Pro admin UI hides the whole "Mobile device names" payload
			// when it is false, and the server stores exactly what the write
			// sends rather than deriving it (wire-probed 2026-08-07, Jamf Pro
			// 11.30). Omitting it is what made naming invisible in the UI.
			// Deliberately plain Optional, NOT Optional+Computed — an explicit
			// carve-out from the provider-wide "full-replace ⇒ omit=preserve"
			// standard (STYLE_GUIDE §Full-replace endpoints). prestage_device_names
			// is authoritative, mode-gated content (only valid when
			// assign_names_using = "List of Names"): Terraform owns the list when it
			// is declared, and an empty/omitted list in another naming mode is the
			// correct state, so drift-revert (not omit=preserve) is the desired
			// behaviour. Converting it would also entangle the server-assigned
			// id/used + positional reconcile + the PUT-500 serializer workaround
			// (diffPlanAgainstGet) with a discriminator-aware plan modifier for no
			// real co-management benefit. Audited 2026-06-09; keep as-is.
			"prestage_device_names": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered list of device names (used in `\"List of Names\"` mode). Jamf Pro assigns each entry an `id` and consumes them in order as devices enrol. The framework reconciles entries by list position; append new names to the end to avoid churn.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"device_name": schema.StringAttribute{
							MarkdownDescription: "The device name.",
							Required:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "Name ID assigned by Jamf Pro. Supply `\"-1\"` for a new entry.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"used": schema.BoolAttribute{
							MarkdownDescription: "Whether this name has been consumed by an enrolled device. Returned by Jamf Pro; not user-settable.",
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseNonNullStateForUnknown(),
							},
						},
					},
				},
			},
		},
	}
}

// locationInformationSchema returns the User and Location Information block.
func locationInformationSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"User and Location Information\"** in the Jamf Pro admin UI. Supply the block, even empty (`location_information = {}`), to manage this section. Omitting it produces drift on the next refresh, because Jamf Pro always returns a populated block.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"username":      optString("**\"Username\"** for the device record."),
			"realname":      optString("**\"Full Name\"** for the device record."),
			"phone":         optString("**\"Phone Number\"** for the device record."),
			"email":         optString("**\"Email Address\"** for the device record."),
			"room":          optString("**\"Room\"** for the device record."),
			"position":      optString("**\"Position\"** for the device record."),
			"building_id":   optString("Building ID for the device record. Sentinel `\"-1\"` = no building."),
			"department_id": optString("Department ID for the device record. Sentinel `\"-1\"` = no department."),
		},
	}
}

// purchasingInformationSchema returns the Purchasing Information block.
func purchasingInformationSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"Purchasing Information\"** in the Jamf Pro admin UI. Supply the block, even empty (`purchasing_information = {}`), to manage this section. Omitting it produces drift on the next refresh, because Jamf Pro always returns a populated block.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"leased":             optBool("**\"Leased\"** in the Jamf Pro admin UI."),
			"purchased":          optBool("**\"Purchased\"** in the Jamf Pro admin UI."),
			"apple_care_id":      optString("**\"AppleCare ID\"** in the Jamf Pro admin UI."),
			"po_number":          optString("**\"PO Number\"** in the Jamf Pro admin UI."),
			"vendor":             optString("**\"Vendor\"** in the Jamf Pro admin UI."),
			"purchase_price":     optString("**\"Purchase Price\"** in the Jamf Pro admin UI."),
			"life_expectancy":    optInt64("**\"Life Expectancy (years)\"** in the Jamf Pro admin UI."),
			"purchasing_account": optString("**\"Purchasing Account\"** in the Jamf Pro admin UI."),
			"purchasing_contact": optString("**\"Purchasing Contact\"** in the Jamf Pro admin UI."),
			"lease_date":         optString("**\"Lease Date\"** in `YYYY-MM-DD` format. Jamf Pro returns `\"1970-01-01\"` when unset."),
			"po_date":            optString("**\"PO Date\"** in `YYYY-MM-DD` format. Jamf Pro returns `\"1970-01-01\"` when unset."),
			"warranty_date":      optString("**\"Warranty Date\"** in `YYYY-MM-DD` format. Jamf Pro returns `\"1970-01-01\"` when unset."),
		},
	}
}
