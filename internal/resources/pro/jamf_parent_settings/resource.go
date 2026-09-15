// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package jamf_parent_settings implements the
// jamfplatform_pro_jamf_parent_settings singleton resource. It wraps the Jamf
// Pro Jamf Parent settings page (Settings → Jamf apps → Jamf Parent): the
// enable toggle, the student device group, the per-day Jamf Parent App
// Restrictions, the time zone, the passcode-clear and revoke-on-wipe toggles,
// and the Safelisted Apps tab.
package jamf_parent_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
// Empty: the parent-app endpoint long predates the provider's overall floor.
const minJamfProVersion = ""

// JamfParentSettingsResource implements the singleton Jamf Parent settings
// resource.
//
// The resource is backed by an Update-only Jamf Pro API — one Jamf Parent
// settings object per tenant. Create funnels into a full-replace write
// (adopting live values for undeclared fields). Delete is state-only by design.
type JamfParentSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &JamfParentSettingsResource{}
var _ resource.ResourceWithImportState = &JamfParentSettingsResource{}
var _ resource.ResourceWithIdentity = &JamfParentSettingsResource{}

// Default timeouts.
const (
	defaultCreateTimeout = 30 * time.Second
	defaultReadTimeout   = 30 * time.Second
	defaultUpdateTimeout = 30 * time.Second
	defaultDeleteTimeout = 30 * time.Second
)

// restrictedTimesDayKeys are the only map keys the server accepts —
// strict-UPPERCASE java.time.DayOfWeek names (lowercase "sunday" and unknown
// names are a 400, wire-probed 2026-06-10).
var restrictedTimesDayKeys = []string{
	"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY",
}

// NewJamfParentSettingsResource constructs a new JamfParentSettingsResource.
func NewJamfParentSettingsResource() resource.Resource {
	return &JamfParentSettingsResource{}
}

// Metadata sets the resource type name.
func (r *JamfParentSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_parent_settings"
}

// IdentitySchema defines the import identifier — singleton id only.
func (r *JamfParentSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
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
func (r *JamfParentSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro **Jamf Parent** settings page (UI: Settings → Jamf apps → Jamf Parent). One record per tenant. These options control limited management of students' devices by parents or guardians using the Jamf Parent app.\n\n" +
			"An optional attribute you omit keeps its current Jamf Pro value, including on the first apply: this resource adopts the existing settings and changes only the attributes you declare. `timezone`, `device_group_id`, and `restricted_times` must always be set, because Jamf Pro requires them on every write.\n\n" +
			"`terraform destroy` removes the resource from Terraform state only. The Jamf Parent settings are left intact on the tenant; they cannot be deleted.\n\n" +
			"Import with `terraform import jamfplatform_pro_jamf_parent_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			// The PUT behind this resource is FULL-REPLACE (wire-probed
			// 2026-06-10): omitted optional fields are reset (safelistedApps →
			// [], the bools → false except allowTemplates → true), and an
			// omitted restrictedTimes or timezoneId is a 500 while an omitted
			// deviceGroupId decodes to 0 and 400s. Every user-settable optional
			// attribute is therefore Optional+Computed with UseStateForUnknown
			// so omit = preserve; on first create the GET-on-create adopt merge
			// in crud.go covers the no-prior-state gap (STYLE_GUIDE
			// §Full-replace endpoints, §Singletons: GET-on-create to adopt).
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Allow limited management of students' devices by parents or guardians using Jamf Parent. Matches the page's enable checkbox. Disabling does not clear the other settings. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
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
				MarkdownDescription: "**\"Time Zone\"** the restricted times are evaluated in (the UI offers a Region + Time Zone picker; this attribute takes the single IANA identifier, e.g. `\"Europe/London\"`).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					validators.IANATimeZone(),
				},
			},
			"device_group_id": schema.Int64Attribute{
				// Required: an omitted deviceGroupId decodes to 0 server-side
				// and is rejected with a clean 400 "No device group was found
				// with id: 0" (wire-probed 2026-06-10) — the §768 API-required
				// carve-out. The server validates the id against existing
				// MOBILE device groups (a computer-group id gets the same 400),
				// so no provider preflight is added: the 4xx is the contract.
				MarkdownDescription: "**\"Student Device Group\"**. ID of the mobile device group (smart or static) whose members Jamf Parent can manage; the group also drives QR-code distribution in Self Service. Jamf Pro rejects ids that do not belong to an existing mobile device group.",
				Required:            true,
			},
			"restricted_times": schema.MapNestedAttribute{
				// Required: omitting restrictedTimes from the PUT is an HTTP
				// 500 (wire-probed 2026-06-10) — the §768 API-required
				// carve-out. An empty map ({}) is valid and means no
				// restrictions. Map keys are server-enforced strict-UPPERCASE
				// java.time.DayOfWeek names (lowercase/unknown → 400), pinned
				// plan-time by the KeysAre validator below.
				MarkdownDescription: "**\"Jamf Parent App Restrictions\"**. Per-day Start/End times during which parents can restrict student devices, keyed by uppercase day name (`MONDAY` … `SUNDAY`). Only the days you declare are stored; Jamf Pro keeps exactly the days present in the map. An empty map (`{}`) means no restrictions.",
				Required:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.OneOf(restrictedTimesDayKeys...)),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"begin_time": schema.StringAttribute{
							// Both times are required per present entry — the
							// server 400s "Begin time and End time are
							// required for <DAY>" when either is missing, and
							// canonicalizes "08:30" to "08:30:00", so the
							// validator pins the canonical HH:MM:SS form.
							MarkdownDescription: "Start of the restricted window for this day, as a 24-hour `HH:MM:SS` time (e.g. `\"08:30:00\"`).",
							Required:            true,
							Validators: []validator.String{
								validators.TimeOfDayHHMMSS(false),
							},
						},
						"end_time": schema.StringAttribute{
							MarkdownDescription: "End of the restricted window for this day, as a 24-hour `HH:MM:SS` time (e.g. `\"15:30:00\"`).",
							Required:            true,
							Validators: []validator.String{
								validators.TimeOfDayHHMMSS(false),
							},
						},
					},
				},
			},
			"allow_clear_passcode": schema.BoolAttribute{
				MarkdownDescription: "**\"Allow Jamf Parent to Clear Student Device Passcode\"**. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"revoke_on_wipe_and_re_enroll": schema.BoolAttribute{
				// UI-aligned rename of the wire field
				// disassociateOnWipeAndReEnroll (STYLE_GUIDE §Attribute names
				// mirror the Jamf Pro admin UI).
				MarkdownDescription: "**\"Revoke Jamf Parent management capabilities when wiping or re-enrolling\"**. Omit to leave the current value untouched (it is not flipped on update); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"safelisted_apps": schema.SetNestedAttribute{
				MarkdownDescription: "Apps students can always use, even while restricted (**Safelisted Apps** tab). Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` to clear them.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				// The server is permissive about safelist entries (same as the
				// teacher endpoint) — these checks are provider-side hygiene
				// mirroring the UI form, which requires both fields and one
				// entry per app.
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
func (r *JamfParentSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_parent_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton.
func (r *JamfParentSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_jamf_parent_settings")
}
