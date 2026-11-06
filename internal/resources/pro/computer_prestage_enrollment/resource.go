// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package computer_prestage_enrollment implements the
// jamfplatform_pro_computer_prestage_enrollment resource, data source, and
// list resource backed by the Jamf Pro Computer PreStage Enrollment API
// (`pro.*ComputerPrestageV3` + scope V2 endpoint family).
package computer_prestage_enrollment

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/prestagealias"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/prestageskip"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: defer to the provider-wide floor — V3 has been available
// since the SDK's first cut and predates the provider's overall minimum.
const minJamfProVersion = ""

const (
	// Defaults span the post-Create / post-Update readiness wait — Jamf
	// Pro generates the prestage's profile_uuid asynchronously and any
	// scope or PUT operation against the prestage returns
	// `500 + empty errors[]` until that field populates. The wait is
	// usually under a few seconds but can stretch to 30s under load.
	defaultCreateTimeout = 5 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 5 * time.Minute
	defaultDeleteTimeout = 60 * time.Second

	// prestageReadyPollInterval is how often the provider polls the
	// per-prestage GET while waiting for `profile_uuid` to populate.
	prestageReadyPollInterval = 2 * time.Second
)

// ComputerPrestageEnrollmentResource implements the Terraform resource.
type ComputerPrestageEnrollmentResource struct {
	client     *pro.Client
	collisions *collision.Checker
}

var (
	_ resource.Resource                = &ComputerPrestageEnrollmentResource{}
	_ resource.ResourceWithImportState = &ComputerPrestageEnrollmentResource{}
	_ resource.ResourceWithIdentity    = &ComputerPrestageEnrollmentResource{}
	_ resource.ResourceWithModifyPlan  = &ComputerPrestageEnrollmentResource{}
)

// NewComputerPrestageEnrollmentResource returns a new resource instance.
func NewComputerPrestageEnrollmentResource() resource.Resource {
	return &ComputerPrestageEnrollmentResource{}
}

// Metadata sets the resource type name.
func (r *ComputerPrestageEnrollmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_prestage_enrollment"
}

// IdentitySchema defines the identifier used for import.
func (r *ComputerPrestageEnrollmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro computer PreStage enrollment ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *ComputerPrestageEnrollmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro Computer PreStage Enrollment: the macOS Automated Device Enrollment (ADE) record exposed at *Settings → Computer Management → PreStage Enrollments* in the Jamf Pro admin UI. " +
			"Device scope (`scope_serial_numbers`) is folded into this resource. Serial numbers must exist on the underlying ADE token, or Jamf Pro rejects the assignment." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Computer PreStage enrollment ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Must not be blank.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			// Wire field: mandatory. Replaces `mandatory`, which stays for the deprecation window.
			"make_mdm_profile_mandatory": renamedBool("**\"Make MDM Profile Mandatory\"** in the Jamf Pro admin UI. Require the user to apply the MDM profile. Replaces `mandatory`; set one or the other.", "mandatory", true, topLevelBoolModifier),
			"mandatory":                  deprecatedBool("**\"Make MDM Profile Mandatory\"** in the Jamf Pro admin UI.", deprecationMandatory, "make_mdm_profile_mandatory", topLevelBoolModifier),
			// Wire field: mdmRemovable. Replaces `mdm_removable`, which stays for the deprecation window.
			"allow_mdm_profile_removal": renamedBool("**\"Allow MDM Profile Removal\"** in the Jamf Pro admin UI. Allow the user to remove the MDM profile. Replaces `mdm_removable`; set one or the other.", "mdm_removable", true, topLevelBoolModifier),
			"mdm_removable":             deprecatedBool("**\"Allow MDM Profile Removal\"** in the Jamf Pro admin UI.", deprecationMdmRemovable, "allow_mdm_profile_removal", topLevelBoolModifier),
			"default_prestage": schema.BoolAttribute{
				MarkdownDescription: "When true, this PreStage becomes the tenant default. Jamf Pro enforces at most one default PreStage; setting this to `true` may cause another PreStage to be silently demoted.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"support_phone_number": schema.StringAttribute{
				MarkdownDescription: "**\"Support Phone Number\"** in the Jamf Pro admin UI.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"support_email_address": schema.StringAttribute{
				MarkdownDescription: "**\"Support Email Address\"** in the Jamf Pro admin UI.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"department": schema.StringAttribute{
				MarkdownDescription: "**\"Department\"** label shown during Setup Assistant. Free-form text, *not* the department ID (`location_information.department_id`).",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"require_authentication": schema.BoolAttribute{
				MarkdownDescription: "**\"Require Authentication\"** in the Jamf Pro admin UI. When `true`, users must authenticate before completing Setup Assistant.",
				Required:            true,
			},
			// Wire field: authenticationPrompt. Replaces `authentication_prompt`, which stays for the deprecation window.
			"authentication_message": schema.StringAttribute{
				MarkdownDescription: "**\"Authentication Message\"** shown to the user when `require_authentication = true`. Replaces `authentication_prompt`; set one or the other.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"authentication_prompt": schema.StringAttribute{
				MarkdownDescription: "~> " + deprecationAuthenticationPrompt + "\n\n**\"Authentication Message\"** shown to the user when `require_authentication = true`.",
				DeprecationMessage:  deprecationAuthenticationPrompt,
				Optional:            true,
				Computed:            true,
				Validators:          prestagealias.StringConflicts("authentication_message"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"device_enrollment_program_instance_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Automated Device Enrollment (ADE) instance that backs this PreStage. Required.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			// `site_id` is read-only on this resource because Jamf Pro
			// does not accept `siteId` in the Pro V3 PreStage create or
			// update payloads. Use `enrollment_site_id` to drive site
			// assignment for devices enrolled through this PreStage.
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site ID that owns this PreStage. Returned by Jamf Pro; not user-settable on this resource. Use `enrollment_site_id` to drive site assignment for devices enrolled through this PreStage. Jamf Pro reports `\"-1\"` when no site is set.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enrollment_site_id": schema.StringAttribute{
				MarkdownDescription: "Site ID applied to devices enrolled through this PreStage. Sentinel `\"-1\"` = no site.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// Wire field: keepExistingLocationInformation. Replaces `keep_existing_location_information`, which stays for the deprecation window.
			"use_existing_location_information":  renamedBool("**\"Use existing location information, if applicable\"** in the Jamf Pro admin UI. Replaces `keep_existing_location_information`; set one or the other.", "keep_existing_location_information", true, topLevelBoolModifier),
			"keep_existing_location_information": deprecatedBool("**\"Use existing location information, if applicable\"** in the Jamf Pro admin UI.", deprecationKeepExistingLocationInformation, "use_existing_location_information", topLevelBoolModifier),
			// Wire field: keepExistingSiteMembership. Replaces `keep_existing_site_membership`, which stays for the deprecation window.
			"use_existing_site_membership":  renamedBool("**\"Use existing site membership, if applicable\"** in the Jamf Pro admin UI. Replaces `keep_existing_site_membership`; set one or the other.", "keep_existing_site_membership", true, topLevelBoolModifier),
			"keep_existing_site_membership": deprecatedBool("**\"Use existing site membership, if applicable\"** in the Jamf Pro admin UI.", deprecationKeepExistingSiteMembership, "use_existing_site_membership", topLevelBoolModifier),
			"enrollment_customization_id": schema.StringAttribute{
				MarkdownDescription: "Enrollment customization ID to apply during Setup Assistant. Sentinel `\"0\"` = no customization; it is `\"0\"`, not `\"-1\"`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"language": schema.StringAttribute{
				MarkdownDescription: "Default Setup Assistant language (ISO-639 code, e.g. `\"en\"`).",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Default Setup Assistant region (ISO-3166 code, e.g. `\"US\"`).",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// Wire field: autoAdvanceSetup. Replaces `auto_advance_setup`, which stays for the deprecation window.
			"automatically_advance_through_setup_assistant": renamedBool("**\"Automatically advance through Setup Assistant (macOS 11 or later only)\"** in the Jamf Pro admin UI. Automatically skips every Setup Assistant screen that can be skipped during enrollment. Replaces `auto_advance_setup`; set one or the other.", "auto_advance_setup", true, topLevelBoolModifier),
			"auto_advance_setup":                            deprecatedBool("**\"Automatically advance through Setup Assistant (macOS 11 or later only)\"** in the Jamf Pro admin UI.", deprecationAutoAdvanceSetup, "automatically_advance_through_setup_assistant", topLevelBoolModifier),
			// Wire field: installProfilesDuringSetup. The admin UI has no control for it: Jamf Pro refuses profiles unless it is true,
			// so the provider derives it from the profile set. A configured value is sent as written.
			"install_profiles_during_setup": schema.BoolAttribute{
				MarkdownDescription: "~> " + deprecationInstallProfilesDuringSetup + "\n\nThe provider sends `true` whenever `configuration_profile_ids` holds a profile. Setting `false` alongside profiles is a configuration error.",
				DeprecationMessage:  deprecationInstallProfilesDuringSetup,
				Optional:            true,
				Validators: []validator.Bool{
					installProfilesFalseWithProfiles(),
				},
			},
			// Wire field: prestageInstalledProfileIds. Replaces `prestage_installed_profile_ids`, which stays for the deprecation window.
			"configuration_profile_ids": schema.SetAttribute{
				MarkdownDescription: "**\"Configuration Profiles\"** pane of the Jamf Pro admin UI: the IDs of the configuration profiles to install during Setup Assistant. Replaces `prestage_installed_profile_ids`; set one or the other. Leave it empty to install none.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"prestage_installed_profile_ids": schema.SetAttribute{
				MarkdownDescription: "~> " + deprecationPrestageInstalledProfileIDs + "\n\n**\"Configuration Profiles\"** pane of the Jamf Pro admin UI: the IDs of the configuration profiles to install during Setup Assistant.",
				DeprecationMessage:  deprecationPrestageInstalledProfileIDs,
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators:          prestagealias.SetConflicts("configuration_profile_ids"),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"custom_package_ids": schema.SetAttribute{
				MarkdownDescription: "Set of custom package IDs to install during Setup Assistant. Paired with `custom_package_distribution_point_id`.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"custom_package_distribution_point_id": schema.StringAttribute{
				MarkdownDescription: "Distribution point ID used to serve `custom_package_ids`. Sentinels: `\"-1\"` = none, `\"-2\"` = cloud distribution point, positive integer = specific DP ID.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// Jamf Pro validates certificate content on save; supplying
			// invalid PEM bytes silently rolls the whole save back
			// (HTTP 500 with empty errors[] on the wire, no per-field
			// rejection). The provider's update path detects this via a
			// post-write diff and surfaces a hard error.
			"anchor_certificates": schema.ListAttribute{
				MarkdownDescription: "Ordered list of base64-encoded PEM certificates to embed in the PreStage. Each entry must be a valid X.509 certificate in PEM format. Jamf Pro rejects malformed entries by silently discarding the whole change; the provider catches that and surfaces a hard error.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listUseStateForUnknown(),
				},
			},
			"prevent_activation_lock": schema.BoolAttribute{
				MarkdownDescription: "**\"Prevent user from enabling Activation Lock\"** in the Jamf Pro admin UI.",
				Required:            true,
			},
			"enable_device_based_activation_lock": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable Activation Lock on the device (Apple School Manager, Apple Business Manager)\"** in the Jamf Pro admin UI.",
				Required:            true,
			},
			"enable_recovery_lock": schema.BoolAttribute{
				MarkdownDescription: "**\"Set Recovery Lock Password\"** in the Jamf Pro admin UI. Turns Recovery Lock on; `recovery_lock_password_type` chooses how the password is provisioned.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"recovery_lock_password_type": schema.StringAttribute{
				MarkdownDescription: "How the Recovery Lock password is provisioned. `\"MANUAL\"` = supply `recovery_lock_password`; `\"RANDOM\"` = Jamf Pro generates per-device passwords.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(recoveryLockPasswordTypeValues...),
					recoveryLockPasswordTypeRandomConflictsWithPassword(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"recovery_lock_password": schema.StringAttribute{
				MarkdownDescription: "Recovery Lock plaintext password. Only meaningful when `recovery_lock_password_type = \"MANUAL\"` AND `enable_recovery_lock = true`. WriteOnly: the value is sent to Jamf Pro but never persisted in Terraform state. Bump `recovery_lock_password_wo_version` to force the provider to re-send the current value.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Validators: []validator.String{
					recoveryLockPasswordRequiresManualAndEnabled(),
				},
			},
			"recovery_lock_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the WriteOnly `recovery_lock_password`. Bump to force the provider to re-send the current password value.",
				Optional:            true,
			},
			"rotate_recovery_lock_password": schema.BoolAttribute{
				MarkdownDescription: "**\"Rotate Recovery Lock Password\"** in the Jamf Pro admin UI.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"prestage_minimum_os_target_version_type": schema.StringAttribute{
				MarkdownDescription: "**\"Minimum required macOS version\"** in the Jamf Pro admin UI: the enforcement mode. One of `NO_ENFORCEMENT`, `MINIMUM_OS_LATEST_VERSION`, `MINIMUM_OS_LATEST_MAJOR_VERSION`, `MINIMUM_OS_LATEST_MINOR_VERSION`, `MINIMUM_OS_SPECIFIC_VERSION`. Pair `MINIMUM_OS_SPECIFIC_VERSION` with `minimum_os_specific_version`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(prestageMinimumOsTargetVersionValues...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"minimum_os_specific_version": schema.StringAttribute{
				MarkdownDescription: "**\"Minimum required macOS version\"** in the Jamf Pro admin UI: the specific version (e.g. `\"14.5\"`). Used only when `prestage_minimum_os_target_version_type = \"MINIMUM_OS_SPECIFIC_VERSION\"`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"psso_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable Simplified Setup for Platform Single Sign-on (macOS 26 or later)\"** in the Jamf Pro admin UI. Whether Platform SSO is enabled for this PreStage.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"platform_sso_app_bundle_id": schema.StringAttribute{
				MarkdownDescription: "Bundle identifier of the unattended Platform SSO application, for example `\"com.okta.mobile\"`. Mutually exclusive with `psso_config_profile_id`, which configures attended Platform SSO.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"psso_config_profile_id": schema.StringAttribute{
				MarkdownDescription: "Configuration profile ID for attended Platform SSO. Mutually exclusive with `platform_sso_app_bundle_id`, which configures unattended Platform SSO. Incompatible with an enrollment customization, so `enrollment_customization_id` must be `\"0\"`. Sentinel `\"-1\"` means none.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					pssoConfigProfileConflictsWithBundle(),
					pssoAttendedConflictsWithEnrollmentCustomization(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"profile_url": schema.StringAttribute{
				MarkdownDescription: "Returned by Jamf Pro for the Platform SSO 403 workflow; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"manifest_url": schema.StringAttribute{
				MarkdownDescription: "Returned by Jamf Pro for the Platform SSO 403 workflow; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"profile_uuid": schema.StringAttribute{
				MarkdownDescription: "MDM profile UUID assigned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"skip_setup_items":       skipSetupItemsSchema(),
			"location_information":   locationInformationSchema(),
			"purchasing_information": purchasingInformationSchema(),
			"account_settings":       accountSettingsSchema(),
			// Scope is managed via the Pro V2 scope endpoint
			// (/v2/computer-prestages/{id}/scope). The provider always
			// rewrites the full set; partial add/remove is not
			// supported by the underlying SDK call used here.
			"scope_serial_numbers": schema.SetAttribute{
				MarkdownDescription: "Set of device serial numbers assigned to this PreStage. Each serial must exist on the underlying ADE token. The full set is rewritten on every change. " +
					"Jamf Pro enforces single-PreStage-per-serial: assigning a serial that is currently scoped to a different PreStage is rejected with a scope-conflict error and there is no transparent reassignment. To move a serial between PreStages, first remove it from the holding PreStage (e.g. in the same `terraform apply` with an explicit `depends_on` ordering or in two separate applies).",
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
func (r *ComputerPrestageEnrollmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_prestage_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.collisions = providerdata.ConfigureCollision(req.ProviderData)
}

// ModifyPlan keeps the two Platform SSO mode inputs from being written together.
//
// platform_sso_app_bundle_id (unattended) and psso_config_profile_id (attended)
// are both Optional+Computed with UseStateForUnknown, so the field the user did
// not author in HCL carries the prior server value forward into the plan. If an
// admin switches the PreStage from one mode to the other in the Jamf Pro UI, a
// later apply would otherwise reassert the HCL-configured field AND write the
// stale server value of the other field back — putting both on the wire. When
// the config selects exactly one mode, force the other field's plan to its wire
// "none" form so only the configured mode is sent.
func (r *ComputerPrestageEnrollmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	r.reportNameCollision(ctx, req, resp)
	prestageskip.ReconcilePlan(ctx, req, resp, skipAliases())
	prestagealias.ReconcilePlan(ctx, req, resp, topLevelPairs)
	var planAccount types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("account_settings"), &planAccount)...)
	if !resp.Diagnostics.HasError() && !planAccount.IsNull() && !planAccount.IsUnknown() {
		prestagealias.ReconcilePlan(ctx, req, resp, accountSettingsPairs)
	}

	var cfgBundle, cfgProfile, cfgCustomization types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("platform_sso_app_bundle_id"), &cfgBundle)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("psso_config_profile_id"), &cfgProfile)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("enrollment_customization_id"), &cfgCustomization)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bundleSet := platformSsoBundleConfigured(cfgBundle)
	profileSet := pssoConfigProfileConfigured(cfgProfile)

	// Only override a field when the user left it out of HCL entirely (config
	// null). If they explicitly wrote its "none" form, the plan already equals
	// config — overriding a known config value trips the framework's "planned
	// value does not match config value" check.
	switch {
	case bundleSet && cfgProfile.IsNull():
		// Unattended mode selected: clear the attended profile id to its "none"
		// sentinel so a UI switch to attended can't ride along on the next PUT.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("psso_config_profile_id"), types.StringValue(sentinelNoneIDDash1))...)
	case profileSet:
		// Attended mode selected.
		if cfgBundle.IsNull() {
			// Clear the unattended bundle id to its "none" (empty) form.
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("platform_sso_app_bundle_id"), types.StringValue(""))...)
		}
		if cfgCustomization.IsNull() {
			// Attended Platform SSO is incompatible with an enrollment
			// customization; disable it (sentinel "0") so a UI-set customization
			// can't ride along on the next PUT and trip the server constraint.
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("enrollment_customization_id"), types.StringValue("0"))...)
		}
	}
}

// ImportState handles import by the Jamf Pro PreStage enrollment ID.
func (r *ComputerPrestageEnrollmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// listUseStateForUnknown is a List plan modifier with the same semantics as
// stringplanmodifier.UseStateForUnknown — the framework only exposes
// list-level modifiers via the listplanmodifier subpackage which doesn't yet
// ship UseStateForUnknown for List<String>.
func listUseStateForUnknown() planmodifier.List {
	return listUseStateForUnknownModifier{}
}

type listUseStateForUnknownModifier struct{}

func (listUseStateForUnknownModifier) Description(_ context.Context) string {
	return "Copies the prior state value into the plan when the plan is Unknown."
}

func (listUseStateForUnknownModifier) MarkdownDescription(ctx context.Context) string {
	return "Copies the prior state value into the plan when the plan is Unknown."
}

func (listUseStateForUnknownModifier) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.StateValue.IsNull() {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

// optBool / optString / optInt64 are local helpers for the repetitive
// Optional+Computed scalar shape used inside every SingleNested block. They carry
// the prior value with planmodifiers.UseStateForUnknownInExistingBlock*.
//
// Plain UseStateForUnknown is wrong here. When location_information,
// purchasing_information, account_settings or skip_setup_items goes from omitted
// to declared, every leaf in it holds null in state, the plan copies that null,
// and apply returns the server's default ("", -1, false, 1970-01-01): "Provider
// produced inconsistent result after apply" on every leaf the configuration
// leaves out, reproduced live 2026-10-14. UseNonNullStateForUnknown would avoid
// that too, but ModifyPlan writes the plan, so a plan can collapse to a no-op and
// the non-null variant would leave each null-prior leaf Unknown in it
// (STYLE_GUIDE rule 1a). The in-existing-block modifier carries a null prior only
// when the block is already in state.
func optBool(md string) schema.Attribute {
	return schema.BoolAttribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			planmodifiers.UseStateForUnknownInExistingBlockBool(),
		},
	}
}

func optString(md string) schema.Attribute {
	return schema.StringAttribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			planmodifiers.UseStateForUnknownInExistingBlockString(),
		},
	}
}

func optInt64(md string) schema.Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Int64{
			planmodifiers.UseStateForUnknownInExistingBlockInt64(),
		},
		Validators: []validator.Int64{
			int64validator.AtLeast(0),
		},
	}
}

// locationInformationSchema returns the User and Location Information block.
func locationInformationSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"User and Location Information\"** in the Jamf Pro admin UI. Supply the block to manage this section, even empty as `location_information = {}`. Omitting it produces drift on the next refresh, because Jamf Pro always returns a populated block.",
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
		MarkdownDescription: "**\"Purchasing Information\"** in the Jamf Pro admin UI. Supply the block to manage this section, even empty as `purchasing_information = {}`. Omitting it produces drift on the next refresh, because Jamf Pro always returns a populated block.",
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

// accountSettingsSchema returns the Account Settings block. Contains the
// WriteOnly `admin_password` + `admin_password_wo_version` rotation pair.
// Jamf Pro rejects any non-default value unless its payloadConfigured flag is
// true, so the provider derives that flag; see buildAccountSettingsRequest.
func accountSettingsSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"Account Settings\"** in the Jamf Pro admin UI. Supply the block to manage this section, even empty as `account_settings = {}`. Omitting it produces drift on the next refresh, because Jamf Pro always returns a populated block.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			// Wire field: payloadConfigured. The admin UI has no control for it. Jamf Pro refuses any
			// other non-default account setting unless it is true, so the provider derives it. State
			// holds what the configuration wrote and nothing else.
			"payload_configured": schema.BoolAttribute{
				MarkdownDescription: "~> " + deprecationPayloadConfigured + "\n\nThe provider sends `true` whenever the block sets any other account setting.",
				DeprecationMessage:  deprecationPayloadConfigured,
				Optional:            true,
			},
			// Wire field: localAdminAccountEnabled. Replaces `local_admin_account_enabled`, which stays for the deprecation window.
			"create_managed_local_administrator_account": renamedBool("**\"Create a managed local administrator account during macOS Setup Assistant\"** in the Jamf Pro admin UI. Replaces `local_admin_account_enabled`; set one or the other.", "local_admin_account_enabled", false, nestedBoolModifier),
			"local_admin_account_enabled":                deprecatedBool("**\"Create a managed local administrator account during macOS Setup Assistant\"** in the Jamf Pro admin UI.", deprecationLocalAdminAccountEnabled, "create_managed_local_administrator_account", nestedBoolModifier),
			"admin_username":                             optString("**\"Username\"** for the managed local administrator account."),
			"admin_password": schema.StringAttribute{
				MarkdownDescription: "**\"Password\"** for the managed local administrator account. WriteOnly: the value is sent to Jamf Pro but never persisted in Terraform state. Bump `admin_password_wo_version` to force the provider to re-send the current value.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"admin_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the WriteOnly `admin_password`. Bump to force the provider to re-send the current password value.",
				Optional:            true,
			},
			// Wire field: hiddenAdminAccount. Replaces `hidden_admin_account`, which stays for the deprecation window.
			"hide_managed_local_administrator_account": renamedBool("**\"Hide managed local administrator account in Users & Groups\"** in the Jamf Pro admin UI. Replaces `hidden_admin_account`; set one or the other.", "hidden_admin_account", false, nestedBoolModifier),
			"hidden_admin_account":                     deprecatedBool("**\"Hide managed local administrator account in Users & Groups\"** in the Jamf Pro admin UI.", deprecationHiddenAdminAccount, "hide_managed_local_administrator_account", nestedBoolModifier),
			// Wire field: localUserManaged. Replaces `local_user_managed`, which stays for the deprecation window.
			"make_managed_local_administrator_account_mdm_enabled": renamedBool("**\"Make the managed local administrator account MDM-enabled\"** in the Jamf Pro admin UI. Replaces `local_user_managed`; set one or the other.", "local_user_managed", false, nestedBoolModifier),
			"local_user_managed": deprecatedBool("**\"Make the managed local administrator account MDM-enabled\"** in the Jamf Pro admin UI.", deprecationLocalUserManaged, "make_managed_local_administrator_account_mdm_enabled", nestedBoolModifier),
			"user_account_type": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("**\"Local User Account Type\"** in the Jamf Pro admin UI: the type of account the user creates during enrollment. One of %v. Default `\"STANDARD\"`.", userAccountTypeValues),
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(userAccountTypeValues...),
				},
				PlanModifiers: []planmodifier.String{
					planmodifiers.UseStateForUnknownInExistingBlockString(),
				},
			},
			"prefill_primary_account_info_feature_enabled": optBool("**\"Pre-fill primary account information\"** in the Jamf Pro admin UI."),
			"prefill_type": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("**\"Information Type\"** in the Jamf Pro admin UI: how prefill values are populated. One of %v. `\"CUSTOM\"` requires `prefill_account_full_name` and `prefill_account_user_name`; `\"DEVICE_OWNER\"` populates from the assigning user.", prefillTypeValues),
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(prefillTypeValues...),
					prefillTypeCustomRequiresFullAndUserNames(),
				},
				PlanModifiers: []planmodifier.String{
					planmodifiers.UseStateForUnknownInExistingBlockString(),
				},
			},
			"prefill_account_full_name": optString("**\"Account Full Name\"** prefill value."),
			"prefill_account_user_name": optString("**\"Account User Name\"** prefill value."),
			// Wire field: preventPrefillInfoFromModification. Replaces `prevent_prefill_info_from_modification`, which stays for the deprecation window.
			"lock_primary_account_information":       renamedBool("**\"Lock primary account information\"** in the Jamf Pro admin UI: prevent the user changing the prefilled values during Setup Assistant. Replaces `prevent_prefill_info_from_modification`; set one or the other.", "prevent_prefill_info_from_modification", false, nestedBoolModifier),
			"prevent_prefill_info_from_modification": deprecatedBool("**\"Lock primary account information\"** in the Jamf Pro admin UI.", deprecationPreventPrefillInfoFromModification, "lock_primary_account_information", nestedBoolModifier),
		},
	}
}

var (
	topLevelBoolModifier = []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	nestedBoolModifier   = []planmodifier.Bool{planmodifiers.UseStateForUnknownInExistingBlockBool()}
)

// renamedBool returns the replacement attribute of a renamed bool. When the deprecated attribute
// was Required, one of the two names must still be set.
func renamedBool(md, deprecatedName string, wasRequired bool, mods []planmodifier.Bool) schema.BoolAttribute {
	a := schema.BoolAttribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       mods,
	}
	if wasRequired {
		a.Validators = prestagealias.BoolAtLeastOne(deprecatedName)
	}
	return a
}

// deprecatedBool returns the deprecated twin of a renamed bool.
func deprecatedBool(md, message, replacement string, mods []planmodifier.Bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: "~> " + message + "\n\n" + md,
		DeprecationMessage:  message,
		Optional:            true,
		Computed:            true,
		Validators:          prestagealias.BoolConflicts(replacement),
		PlanModifiers:       mods,
	}
}
