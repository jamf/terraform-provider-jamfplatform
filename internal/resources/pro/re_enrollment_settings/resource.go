// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package re_enrollment_settings implements the jamfplatform_pro_re_enrollment_settings
// singleton resource and data source. It wraps the Jamf Pro Re-enrollment
// settings page — the five "clear …" toggles and the "Clear Management History"
// dropdown that decide which device data is flushed when a previously-managed
// device re-enrolls.
package re_enrollment_settings

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required.
// Empty: the Re-enrollment settings endpoint ships at the provider's overall
// floor.
const minJamfProVersion = ""

// ReEnrollmentSettingsResource implements the singleton Jamf Pro Re-enrollment
// settings resource.
//
// The resource is backed by an Update-only Jamf Pro API — one Re-enrollment
// settings object per tenant. Create funnels into a full-replace write. Delete
// is state-only by design.
type ReEnrollmentSettingsResource struct {
	client *pro.Client

	// enrollmentMu serializes writes to the shared enrollment-settings backing
	// store. The Re-enrollment settings object and the User-Initiated
	// Enrollment settings object are two views of ONE record, so concurrent
	// applies must not interleave. Obtained by reference from the shared
	// providerdata.Data at Configure; the same *sync.Mutex instance is handed
	// to every enrollment resource.
	enrollmentMu *sync.Mutex
}

var _ resource.Resource = &ReEnrollmentSettingsResource{}
var _ resource.ResourceWithImportState = &ReEnrollmentSettingsResource{}
var _ resource.ResourceWithIdentity = &ReEnrollmentSettingsResource{}

// Default timeouts.
const (
	defaultCreateTimeout = 30 * time.Second
	defaultReadTimeout   = 30 * time.Second
	defaultUpdateTimeout = 30 * time.Second
	defaultDeleteTimeout = 30 * time.Second
)

// NewReEnrollmentSettingsResource constructs a new ReEnrollmentSettingsResource.
func NewReEnrollmentSettingsResource() resource.Resource {
	return &ReEnrollmentSettingsResource{}
}

// Metadata sets the resource type name.
func (r *ReEnrollmentSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_re_enrollment_settings"
}

// IdentitySchema defines the import identifier — singleton id only.
func (r *ReEnrollmentSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\".",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the resource schema.
func (r *ReEnrollmentSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro Re-enrollment settings page (Settings → Global → Re-enrollment). One record per tenant. These options decide which data Jamf Pro clears from a computer or mobile device when it re-enrolls after previously being managed.\n\n" +
			"Each `clear_*` toggle you omit keeps its current Jamf Pro value, including on the first apply: this resource adopts the existing settings and only changes the toggles you declare. Each toggle you set is managed by Terraform and is restored if it is edited in the Jamf Pro UI, so you can manage a subset of the toggles and leave the rest as configured in the admin console. A boolean has no \"unset\": omit to preserve, or set `true`/`false` to change it. `clear_management_history` must always be set, because the dropdown always has a selection.\n\n" +
			"`terraform destroy` removes the resource from Terraform state only. The Re-enrollment settings are left intact on the tenant; they cannot be deleted.\n\n" +
			"Import with `terraform import jamfplatform_pro_re_enrollment_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			// The five clear_* toggles are Optional+Computed with
			// UseStateForUnknown: the /v1/reenrollment PUT is full-replace, but
			// omitting a toggle carries its prior value forward (plan Unknown ->
			// USFU -> prior state -> re-emitted -> preserved), so it is not
			// flipped on an unrelated change. On first create there is no prior
			// state, so Create reads the live settings and merges them in (see
			// crud.go) — the singleton is adopted, not reset. No blank exists for a
			// bool — omit to preserve, set true/false to change. (Full-replace +
			// omit->false reset wire-probed 2026-06-09.)
			"clear_policy_logs": schema.BoolAttribute{
				MarkdownDescription: "Clear policy logs on computers when a device re-enrolls. Matches the \"Clear policy logs on computers\" checkbox. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"clear_location_information": schema.BoolAttribute{
				MarkdownDescription: "Clear user and location information on mobile devices and computers when a device re-enrolls. Matches the \"Clear user and location information on mobile devices and computers\" checkbox. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"clear_location_information_history": schema.BoolAttribute{
				MarkdownDescription: "Clear user and location information history on mobile devices and computers when a device re-enrolls. Matches the \"Clear user and location information history on mobile devices and computers\" checkbox. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"clear_extension_attributes": schema.BoolAttribute{
				MarkdownDescription: "Clear extension attribute values on computers and mobile devices when a device re-enrolls. Matches the \"Clear extension attribute values on computers and mobile devices\" checkbox. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"clear_software_update_plans": schema.BoolAttribute{
				MarkdownDescription: "Clear software update plans on mobile devices and computers when a device re-enrolls. Matches the \"Clear software update plans on mobile devices and computers\" checkbox. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"clear_management_history": schema.StringAttribute{
				MarkdownDescription: "How much of a device's management command history is cleared when it re-enrolls. Matches the \"Clear Management History\" dropdown. Required: the dropdown always has a selection, and this resource overwrites it on every apply, so set the value explicitly. One of:\n" +
					"- `DELETE_NOTHING`: clear nothing.\n" +
					"- `DELETE_ERRORS`: clear failed commands.\n" +
					"- `DELETE_EVERYTHING_EXCEPT_ACKNOWLEDGED`: clear pending and failed commands.\n" +
					"- `DELETE_EVERYTHING`: clear completed, failed and pending commands.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(validClearManagementHistory...),
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

// Configure wires the Jamf Pro client and the shared enrollment write lock into
// the resource.
func (r *ReEnrollmentSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_re_enrollment_settings")
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
func (r *ReEnrollmentSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_re_enrollment_settings")
}
