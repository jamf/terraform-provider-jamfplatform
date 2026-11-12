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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
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
	client     *pro.Client
	collisions *collision.Checker
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

			"allow_pairing":                       optBool("**\"Pairing\"** in the Jamf Pro admin UI, under *Settings for Supervised Devices*. Allows devices to connect to Mac computers."),
			"prevent_activation_lock":             optBool("**\"Prevent user from enabling Activation Lock\"** in the Jamf Pro admin UI."),
			"enable_device_based_activation_lock": optBool("**\"Enable Activation Lock on the device (Apple School Manager, Apple Business Manager)\"** in the Jamf Pro admin UI."),
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
			"preserve_managed_apps":             optBool("**\"Preserve managed apps\"** in the Jamf Pro admin UI. The admin UI shows it only while Return to Service is allowed (`allow_return_to_service`), and the provider rejects `true` when that is `false`.", preserveManagedAppsRequiresReturnToService()),
			"do_not_use_profile_from_backup":    optBool("Has no control in the Jamf Pro admin UI."),

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

			"enrollment_site_id":          optStringRef("Site ID applied to devices enrolled through this PreStage. Set `-1` for no site. `\"\"` and `0` fail at plan time. Remove the attribute to keep the current site.", "site"),
			"enrollment_customization_id": optStringValidated("Enrollment customization ID to apply during Setup Assistant. Set `0` for no customization. `-1` and `\"\"` fail at plan time. Remove the attribute to keep the current customization.", validators.PositiveReferenceIDOr("enrollment customization", "Set \"0\" for no customization.", "0")),
			"rts_config_profile_id":       optStringRef("Return to Service configuration profile ID. Set `-1` for none. `\"\"` and `0` fail at plan time. Remove the attribute to keep the current profile.", "configuration profile"),

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
			// REST API recalculates it on every PUT, ignoring any submitted
			// value (spike §F8), so a settable attribute would perpetually
			// diff. The API minimum is 1024 MB, but the admin UI stores 0
			// while Shared iPad / Use Storage Quota Size are off, so an imported
			// UI-created record reads 0 and its first write rewrites it to 1024;
			// ModifyPlan renders that as Unknown (storageQuotaWillBeRepaired). Set it in the Jamf Pro admin UI (which
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
	addRenamedAttributes(resp.Schema.Attributes)
}

// Configure wires the Jamf Pro client into the resource.
func (r *MobileDevicePrestageEnrollmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_prestage_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.collisions = providerdata.ConfigureCollision(req.ProviderData)
}

// ImportState handles import by the Jamf Pro PreStage enrollment ID.
func (r *MobileDevicePrestageEnrollmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// optBool / optString / optInt64 are local helpers for the repetitive
// Optional+Computed scalar shape used across the schema. They carry the prior
// value with planmodifiers.UseStateForUnknownInExistingBlock*.
//
// Plain UseStateForUnknown is wrong here. When location_information,
// purchasing_information, names or skip_setup_items goes from omitted to
// declared, every leaf in it holds null in state, the plan copies that null, and
// apply returns the server's default ("", -1, false, 1970-01-01, "Serial
// Numbers"): "Provider produced inconsistent result after apply" on every leaf
// the configuration leaves out, reproduced live 2026-10-14.
// UseNonNullStateForUnknown would avoid that too, but ModifyPlan writes the plan,
// so a plan can collapse to a no-op and the non-null variant would leave each
// null-prior leaf Unknown in it (STYLE_GUIDE rule 1a). The in-existing-block
// modifier carries a null prior only when the block is already in state.
func optBool(md string, vals ...validator.Bool) schema.Attribute {
	return schema.BoolAttribute{
		MarkdownDescription: md,
		Optional:            true,
		Computed:            true,
		Validators:          vals,
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

// optStringRef is optString for a reference ID whose clear value is "-1"
// (wire-probed 2026-11-12, issue #573).
// optStringValidated is optString with validators.
func optStringValidated(md string, v ...validator.String) schema.Attribute {
	a := optString(md).(schema.StringAttribute)
	a.Validators = v
	return a
}

func optStringRef(md, noun string) schema.Attribute {
	a := optString(md).(schema.StringAttribute)
	a.Validators = []validator.String{validators.ClearableReferenceID(noun)}
	return a
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
					planmodifiers.UseStateForUnknownInExistingBlockString(),
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
			"building_id":   optStringRef("Building ID for the device record. Set `-1` for no building. `\"\"` and `0` fail at plan time. Remove the attribute to keep the current building.", "building"),
			"department_id": optStringRef("Department ID for the device record. Set `-1` for no department. `\"\"` and `0` fail at plan time. Remove the attribute to keep the current department.", "department"),
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
