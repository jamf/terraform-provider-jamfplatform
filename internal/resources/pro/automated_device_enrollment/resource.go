// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package automated_device_enrollment implements the
// jamfplatform_pro_automated_device_enrollment resource backed by the Jamf Pro
// /api/v1/device-enrollments API.
package automated_device_enrollment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: defer to the provider-wide floor via
// providerdata.ConfigurePro — the device-enrollments endpoint predates the
// provider's overall minimum.
const minJamfProVersion = ""

// AutomatedDeviceEnrollmentResource implements the Terraform resource for a
// Jamf Pro Automated Device Enrollment (ADE) instance.
type AutomatedDeviceEnrollmentResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &AutomatedDeviceEnrollmentResource{}
	_ resource.ResourceWithImportState = &AutomatedDeviceEnrollmentResource{}
	_ resource.ResourceWithIdentity    = &AutomatedDeviceEnrollmentResource{}
)

const (
	// defaultCreateTimeout bounds the whole Create operation including the
	// post-upload sync wait. Apple's ADE round-trip is the slow leg —
	// uploading the .p7m token is sub-second but Jamf then has to fetch
	// the device list from Apple, which can take a few minutes on cold
	// tokens.
	defaultCreateTimeout = 5 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	// defaultUpdateTimeout also covers the post-rotation sync wait when
	// `server_token_wo_version` is bumped.
	defaultUpdateTimeout = 5 * time.Minute
	defaultDeleteTimeout = 60 * time.Second

	// syncPollInterval is how often the provider polls
	// pro.GetLatestDeviceEnrollmentSyncV1 while waiting for the Apple ADE
	// sync to finish. Conservative — Apple's side is the bottleneck, not
	// the polling cadence.
	syncPollInterval = 5 * time.Second

	// adeSyncStateSuccessful is the terminal-success value of
	// DeviceEnrollmentInstanceSyncStatus.SyncState (verified via live
	// wire-probe against a synced tenant ADE instance, 2026-05-28).
	adeSyncStateSuccessful = "SUCCESSFUL"
)

// NewAutomatedDeviceEnrollmentResource returns a new instance of
// AutomatedDeviceEnrollmentResource.
func NewAutomatedDeviceEnrollmentResource() resource.Resource {
	return &AutomatedDeviceEnrollmentResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AutomatedDeviceEnrollmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_automated_device_enrollment"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *AutomatedDeviceEnrollmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Automated Device Enrollment (ADE) instance ID used to uniquely reference the instance.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the automated device enrollment
// resource.
func (r *AutomatedDeviceEnrollmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro Automated Device Enrollment (ADE) instance. " +
			"ADE binds a Jamf Pro tenant to an Apple School Manager / Apple Business Manager MDM " +
			"server using a `.p7m` server token downloaded from Apple. The provider performs the " +
			"two-step upload-and-rename flow internally: it first uploads the decoded token bytes " +
			"to allocate the instance, then sets the user-visible `name` and any optional " +
			"`site_id` / `supervision_identity_id` associations. If the rename step fails the " +
			"provider deletes the partially-created instance so Terraform's create either fully " +
			"succeeds or leaves no resource behind. " +
			"After every token write (initial create, and any update that bumps " +
			"`server_token_wo_version`), the provider blocks until Jamf Pro reports the " +
			"Apple ADE sync as `SUCCESSFUL`. Until that completes the device list is not known " +
			"to Jamf Pro, and downstream resources such as `jamfplatform_pro_computer_prestage_enrollment` " +
			"scope assignments will fail. Default create and update timeout is 5 minutes; override " +
			"it with the `timeouts` block when Apple's round-trip is slower on a particular tenant." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Automated Device Enrollment instance ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name for the ADE instance in the Jamf Pro admin UI. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"server_token": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded contents of the `.p7m` server token downloaded from Apple " +
					"Business Manager / Apple School Manager for this MDM server. `WriteOnly`: the value is " +
					"sent to Jamf Pro on create and on token-rotating updates, and **never persisted in " +
					"Terraform state**. Jamf Pro also never returns the token on reads, so the only signal " +
					"Terraform can use to rotate the stored token is the companion `server_token_wo_version` " +
					"integer. The provider trims surrounding whitespace and then base64-decodes the supplied " +
					"string to raw bytes before sending; a decode failure surfaces as a plan-time diagnostic.",
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"server_token_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `server_token`. Bump this integer " +
					"(any change) to force an update that re-sends the current `server_token` to Jamf Pro. " +
					"Initial create should set `server_token_wo_version = 1`. Required because `server_token` " +
					"is Required; keeping the companion Required keeps the rotation signal explicit in " +
					"config. " +
					"Bumping the value triggers a fresh Apple ADE sync, and the update blocks until " +
					"Jamf Pro reports the sync as `SUCCESSFUL` (subject to `timeouts.update`, default 5 minutes).",
				Required: true,
			},
			"token_file_name": schema.StringAttribute{
				MarkdownDescription: "Optional file name to send alongside the uploaded token (e.g. " +
					"`\"my-org-ade-token.p7m\"`). Jamf Pro does not return this field on reads, so the " +
					"attribute is plain `Optional` rather than `Optional+Computed`: it is used only at upload time " +
					"and is not refreshed on read.",
				Optional: true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Optional Jamf Pro site ID to associate with this ADE instance. Jamf Pro " +
					"reports the sentinel `\"-1\"` when no site is set. The provider mirrors whatever Jamf Pro " +
					"reports into state and applies no default. Omit to leave the current value untouched; " +
					"set `-1` to clear it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"supervision_identity_id": schema.StringAttribute{
				MarkdownDescription: "Optional Jamf Pro supervision identity ID to associate with this ADE " +
					"instance. Jamf Pro reports the sentinel `\"-1\"` when no supervision identity is set; the " +
					"provider mirrors whatever Jamf Pro reports into state and does not apply a default. Omit " +
					"to leave the current value untouched; set `-1` to clear it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"admin_id": schema.StringAttribute{
				MarkdownDescription: "Apple administrator ID parsed from the uploaded server token.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_name": schema.StringAttribute{
				MarkdownDescription: "Organization name parsed from the uploaded server token.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_email": schema.StringAttribute{
				MarkdownDescription: "Organization email address parsed from the uploaded server token.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_phone": schema.StringAttribute{
				MarkdownDescription: "Organization phone number parsed from the uploaded server token. Apple " +
					"may return values containing trailing whitespace; the provider preserves whatever Jamf " +
					"Pro reports without trimming.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_address": schema.StringAttribute{
				MarkdownDescription: "Organization mailing address parsed from the uploaded server token. " +
					"Apple may return values containing trailing whitespace; the provider preserves whatever " +
					"Jamf Pro reports without trimming.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_name": schema.StringAttribute{
				MarkdownDescription: "MDM server hostname recorded by Apple for this ADE instance.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_uuid": schema.StringAttribute{
				MarkdownDescription: "MDM server UUID recorded by Apple for this ADE instance.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"token_expiration_date": schema.StringAttribute{
				MarkdownDescription: "Expiration date of the uploaded ADE server token, in `YYYY-MM-DD` format.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *AutomatedDeviceEnrollmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_automated_device_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro ADE instance ID.
func (r *AutomatedDeviceEnrollmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
