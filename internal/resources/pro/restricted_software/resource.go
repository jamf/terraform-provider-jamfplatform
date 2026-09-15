// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package restricted_software implements the jamfplatform_pro_restricted_software
// resource, data source, and list resource backed by the Jamf ProClassic
// /restrictedsoftware API. The construct name mirrors the Jamf Pro admin UI
// ("Restricted software" under the Computers sidebar). Scope is a LIMITED
// computer-scope subset — targets + exclusions only, no limitations block and
// no target users — hand-composed from the shared scope primitives because the
// category set differs from the full computer-scope factory.
package restricted_software

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /restrictedsoftware predates the provider's overall
// floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

// RestrictedSoftwareResource implements the Terraform resource for Jamf Pro
// restricted software records. No directory-service preflight is wired: the
// only name-keyed scope category (exclusion users) is free-text local
// usernames, not Jamf LDAP/cloud-IdP objects.
type RestrictedSoftwareResource struct {
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
	client *proclassic.Client
}

var _ resource.Resource = &RestrictedSoftwareResource{}
var _ resource.ResourceWithImportState = &RestrictedSoftwareResource{}
var _ resource.ResourceWithIdentity = &RestrictedSoftwareResource{}
var _ resource.ResourceWithModifyPlan = &RestrictedSoftwareResource{}
var _ resource.ResourceWithConfigValidators = &RestrictedSoftwareResource{}

// ConfigValidators returns the resource's plan-time cross-field checks.
func (r *RestrictedSoftwareResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		deleteApplicationRequiresExactMatchValidator{},
	}
}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewRestrictedSoftwareResource returns a new instance of RestrictedSoftwareResource.
func NewRestrictedSoftwareResource() resource.Resource {
	return &RestrictedSoftwareResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *RestrictedSoftwareResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_restricted_software"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *RestrictedSoftwareResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro restricted software ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource. Attribute names mirror
// the Jamf Pro admin UI labels (STYLE_GUIDE §Attribute names mirror the admin
// UI); the differing wire element names are noted in the attribute descriptions.
func (r *RestrictedSoftwareResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Scope > Targets sub-block: the all_computers flag plus the per-category
	// ID sets. Every attribute is Optional-only, matching the shared factories
	// in internal/common/scope: the null/`[]` (or null/`false`) distinction
	// carries the granular per-category ownership contract, so nothing here may
	// be Computed or carry a state-forwarding plan modifier (see STYLE_GUIDE.md
	// §Scope helper). The AllFlagConflictsWith relative paths resolve against
	// their siblings inside `targets`.
	targets := map[string]schema.Attribute{
		"all_computers": schema.BoolAttribute{
			MarkdownDescription: "Scope to every computer in the tenant. Forbids per-computer / per-group / per-building / per-department targets when true. Omit to leave the toggle as configured outside Terraform.",
			Optional:            true,
			Validators: []validator.Bool{
				scope.AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("computer_ids"),
					path.MatchRelative().AtParent().AtName("computer_group_ids"),
					path.MatchRelative().AtParent().AtName("building_ids"),
					path.MatchRelative().AtParent().AtName("department_ids"),
				),
			},
		},
		"computer_ids":       scope.IDSetAttribute("computer"),
		"computer_group_ids": scope.IDSetAttribute("computer group"),
		"building_ids":       scope.IDSetAttribute("building"),
		"department_ids":     scope.IDSetAttribute("department"),
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro restricted software record, the \"Restricted software\" entry under the Computers sidebar in the Jamf Pro admin UI. It restricts a process by name on the targeted computers, and can also kill the process, delete the application, and notify admins. Scope is computer-only, with targets and exclusions but no limitations." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Restricted software ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"general": schema.SingleNestedAttribute{
				MarkdownDescription: "General settings. Mirrors the admin UI's \"Options\" tab. `name` and `process_name` are required on create.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "Record ID under `general`. Matches the top-level `id`. Returned by Jamf Pro.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Display name for the restricted software record; must be unique within the tenant.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"process_name": schema.StringAttribute{
						MarkdownDescription: "**\"Process Name\"** in the Jamf Pro admin UI. The name of the process to restrict. To target a process inside an application bundle, enter the application bundle name (e.g. `Chess.app`). The match is case-sensitive; `*` is treated as a literal character.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					// Wire: <match_exact_process_name>. Server-defaults true.
					"restrict_exact_process_name": schema.BoolAttribute{
						MarkdownDescription: "**\"Restrict exact process name\"** in the Jamf Pro admin UI. Only restrict processes that match the exact process name. Defaults to `true`. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
					},
					// Wire: <send_notification>.
					"send_email_notification_on_violation": schema.BoolAttribute{
						MarkdownDescription: "**\"Send email notification on violation\"** in the Jamf Pro admin UI. When the process is found, send an email notification to Jamf Pro users with email notifications enabled (an SMTP server must be configured). Defaults to `false`. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
					},
					"kill_process": schema.BoolAttribute{
						MarkdownDescription: "**\"Kill process\"** in the Jamf Pro admin UI. Terminate the restricted process when found. Defaults to `false`. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
					},
					// Wire: <delete_executable>.
					"delete_application": schema.BoolAttribute{
						MarkdownDescription: "**\"Delete application\"** in the Jamf Pro admin UI. Delete the application running the restricted process. Defaults to `false`. Requires `restrict_exact_process_name = true`: Jamf Pro identifies the application to delete from an exact process name, and clears this flag without one. The provider checks the pairing at plan time. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
					},
					"display_message": schema.StringAttribute{
						MarkdownDescription: "**\"Message\"** in the Jamf Pro admin UI. Message to display to users when the process is found. Defaults to an empty string. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"site_id": schema.StringAttribute{
						MarkdownDescription: "**\"Site\"** in the Jamf Pro admin UI. Jamf Pro site ID scoping the record. Omit to leave the current value untouched; there is no blank-clear, so set `-1` to remove the site.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"site_name": schema.StringAttribute{
						// No UseStateForUnknown: site_name is derived from site_id,
						// so it must go Unknown (not pin the stale value) whenever
						// the record changes, or a site_id change trips the
						// post-apply consistency check.
						MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
				},
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "Scope, mirroring the \"Scope\" tab in the Jamf Pro admin UI. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it is left as configured outside Terraform, with updates preserving it. Targets nest under `targets`, mirroring the Targets sub-tab, as flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. Setting `targets.all_computers = true` forbids the per-category target ID sets. Scope limitations are not supported for restricted software.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"targets": schema.SingleNestedAttribute{
						MarkdownDescription: "Scope targets: the audience the record applies to. Mirrors the admin UI's Scope > Targets tab. Set `all_computers = true` for tenant-wide scope, or list specific IDs.",
						Optional:            true,
						Attributes:          targets,
					},
					"exclusions": schema.SingleNestedAttribute{
						MarkdownDescription: "Scope exclusions remove items that would otherwise be included by the targets. `directory_service_or_local_user_names` carries free-text local usernames (the admin UI \"Directory Service/Local Users\" exclusion), not Jamf Pro object IDs.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"computer_ids":                          scope.IDSetAttribute("computer"),
							"computer_group_ids":                    scope.IDSetAttribute("computer group"),
							"building_ids":                          scope.IDSetAttribute("building"),
							"department_ids":                        scope.IDSetAttribute("department"),
							"directory_service_or_local_user_names": scope.NameSetAttribute("directory service or local user"),
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *RestrictedSoftwareResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_restricted_software")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro restricted software ID.
func (r *RestrictedSoftwareResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
