// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package jamf_teacher_settings implements the
// jamfplatform_pro_jamf_teacher_settings singleton resource. It wraps the Jamf
// Pro Jamf Teacher settings page (Settings → Jamf apps → Jamf Teacher): the
// enable toggle, the time zone, the restriction end-time / maximum-length
// limits, and the Safelisted Apps tab.
package jamf_teacher_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required.
// Empty: the teacher-app endpoint long predates the provider's overall floor.
const minJamfProVersion = ""

// JamfTeacherSettingsResource implements the singleton Jamf Teacher settings
// resource.
//
// The resource is backed by an Update-only Jamf Pro API — one Jamf Teacher
// settings object per tenant. Create funnels into a full-replace write
// (adopting live values for undeclared fields). Delete is state-only by design.
type JamfTeacherSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &JamfTeacherSettingsResource{}
var _ resource.ResourceWithImportState = &JamfTeacherSettingsResource{}
var _ resource.ResourceWithIdentity = &JamfTeacherSettingsResource{}

// Default timeouts.
const (
	defaultCreateTimeout = 30 * time.Second
	defaultReadTimeout   = 30 * time.Second
	defaultUpdateTimeout = 30 * time.Second
	defaultDeleteTimeout = 30 * time.Second
)

// NewJamfTeacherSettingsResource constructs a new JamfTeacherSettingsResource.
func NewJamfTeacherSettingsResource() resource.Resource {
	return &JamfTeacherSettingsResource{}
}

// Metadata sets the resource type name.
func (r *JamfTeacherSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_teacher_settings"
}

// IdentitySchema defines the import identifier — singleton id only.
func (r *JamfTeacherSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
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
func (r *JamfTeacherSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro **Jamf Teacher** settings page (UI: Settings → Jamf apps → Jamf Teacher). One record per tenant. These options control limited management of students' devices by the Jamf Teacher app.\n\n" +
			"An optional attribute you omit keeps its current Jamf Pro value, including on the first apply: this resource adopts the existing settings and changes only the attributes you declare. `timezone` must always be set, because Jamf Pro requires it on every write.\n\n" +
			"`terraform destroy` removes the resource from Terraform state only. The Jamf Teacher settings are left intact on the tenant; they cannot be deleted.\n\n" +
			"Import with `terraform import jamfplatform_pro_jamf_teacher_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			// The PUT behind this resource is FULL-REPLACE (wire-probed
			// 2026-06-10): omitted optional fields are reset (autoClear /
			// maxRestrictionLengthSeconds → null, safelistedApps → [],
			// isEnabled → false). Every user-settable optional attribute is
			// therefore Optional+Computed with UseStateForUnknown so omit =
			// preserve; on first create the GET-on-create adopt merge in
			// crud.go covers the no-prior-state gap (STYLE_GUIDE §Full-replace
			// endpoints, §Singletons: GET-on-create to adopt).
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Allow limited management of students' devices by Jamf Teacher. Matches the page's enable checkbox. Disabling does not clear the other settings. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"timezone": schema.StringAttribute{
				// Required: the API rejects every PUT that omits timezoneId
				// (HTTP 500, wire-probed 2026-06-10) — the §768 API-required
				// carve-out. IANA validity is checked plan-time by the shared
				// validators.IANATimeZone() (Go's embedded tzdata; see its doc
				// comment for why tzdata, not the curated /v1/time-zones list,
				// is the gate).
				MarkdownDescription: "**\"Time Zone\"** the restriction times are evaluated in (the UI offers a Region + Time Zone picker; this attribute takes the single IANA identifier, e.g. `\"Europe/London\"`).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					validators.IANATimeZone(),
				},
			},
			"restrictions_end_time": schema.StringAttribute{
				MarkdownDescription: "**\"Restrictions End Time\"**. The time at which all restrictions set by Jamf Teacher are cleared from student devices, as a 24-hour `HH:MM:SS` time (e.g. `\"17:30:00\"`). Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					validators.TimeOfDayHHMMSS(true),
				},
			},
			"maximum_restriction_time_seconds": schema.Int64Attribute{
				// No bounds validator: the server enforces none (accepts -1, 0
				// and values beyond the UI maximum — wire-probed 2026-06-10),
				// so the UI range is documentation only.
				MarkdownDescription: "**\"Maximum Restriction Time\"**. The longest a teacher can restrict student devices for (the page captures hours and minutes; this attribute takes the total in seconds). The page exposes 0 to 28740 (7 h 59 min), but Jamf Pro accepts any integer, so no bounds are enforced here. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"safelisted_apps": schema.SetNestedAttribute{
				MarkdownDescription: "Apps students can always use, even while restricted (**Safelisted Apps** tab). Safelisting more than one app prevents teachers from enabling Single App Mode; safelisting exactly one app lets teachers lock student devices to it. Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` to clear them.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				// The server accepts missing names/bundle ids and duplicate
				// bundle ids (all 200, wire-probed 2026-06-10) — these checks
				// are provider-side hygiene mirroring the UI form, which
				// requires both fields and one entry per app.
				Validators: []validator.Set{
					validators.UniqueStringFieldSet("bundle_id"),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Display name of the safelisted app.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"bundle_id": schema.StringAttribute{
							MarkdownDescription: "Bundle identifier of the safelisted app (e.g. `com.example.app`).",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
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

// Configure wires the Jamf Pro client into the resource.
func (r *JamfTeacherSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_teacher_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton.
func (r *JamfTeacherSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_jamf_teacher_settings")
}
