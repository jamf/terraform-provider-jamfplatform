// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package patch_policy implements the jamfplatform_pro_patch_policy resource,
// data source, and list resource. Every read of an individual policy — CRUD, the
// data source, and the list resource's per-item hydration — goes through the
// Jamf ProClassic /patchpolicies/id/{id} surface, which is the only one carrying
// a policy's scope and user_interaction sections. The list resource *enumerates*
// on the Pro v2 /patch-policies collection instead, because the classic
// collection reads it would otherwise use are deprecated and were for a time
// withdrawn from the published Classic spec; see crud.go for the
// endpoint-by-endpoint record.
//
// The construct name mirrors the Jamf Pro admin UI ("Patch Policies", a tab
// on a software title under Computers → Patch management). A policy is created
// against a patch software title
// configuration and targets a single version of that title that has a package
// assigned. Scope is a LIMITED computer-scope subset (targets + limitations +
// exclusions, no users) hand-composed from the shared scope primitives.
package patch_policy

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /patchpolicies predates the provider's overall floor.
// The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

// distributionMethods is the set of valid distribution_method enum values.
// selfservice = UI "Make Available in Self Service"; prompt = UI "Install
// Automatically". Invalid values coerce to prompt server-side, so the provider
// validates to the two to surface the choice explicitly.
var distributionMethods = proclassic.PatchPolicyGeneralDistributionMethodValues()

// PatchPolicyResource implements the Terraform resource for Jamf Pro patch
// policies. No directory-service preflight is wired: patch-policy scope exposes
// no readable name-keyed (LDAP/cloud-IdP) category.
type PatchPolicyResource struct {
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
	client *proclassic.Client
}

var _ resource.Resource = &PatchPolicyResource{}
var _ resource.ResourceWithImportState = &PatchPolicyResource{}
var _ resource.ResourceWithIdentity = &PatchPolicyResource{}
var _ resource.ResourceWithModifyPlan = &PatchPolicyResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPatchPolicyResource returns a new instance of PatchPolicyResource.
func NewPatchPolicyResource() resource.Resource {
	return &PatchPolicyResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PatchPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_policy"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *PatchPolicyResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro patch policy ID used to uniquely reference the policy.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the patch policy resource. Attribute
// names mirror the Jamf Pro admin UI labels (STYLE_GUIDE §Attribute names mirror
// the admin UI); the differing wire element names are noted in the attribute
// descriptions.
func (r *PatchPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro patch policy, found in the UI under **Computers → Patch management** on a software title's **Patch Policies** tab (the **New Patch Policy** form). A patch policy is created against a patch software title configuration (`software_title_configuration_id`, a `jamfplatform_pro_patch_software_title` ID) and deploys a single `target_version` of that title. Only versions that have a package assigned on the title can be targeted.\n\n" +
			"The form spans three tabs: **General** (`name`, `enabled`, `target_version`, `distribution_method`, `allow_downgrade`, `patch_unknown`), **Scope** (`scope`) and **User Interaction** (`user_interaction`). Five **General**-tab fields are read-only, because Jamf Pro derives them from the selected `target_version`'s patch definition: `release_date`, `incremental_update`, `reboot`, `minimum_os` and `kill_apps`.\n\n" +
			"Updates are merged rather than replaced. Removing a whole optional block (`scope` or `user_interaction`) from your configuration does not clear it: Jamf Pro keeps the values you set previously. To clear a block, null its individual fields instead of deleting the block." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Patch policy ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"software_title_configuration_id": schema.StringAttribute{
				MarkdownDescription: "ID of the patch software title configuration this policy deploys (a `jamfplatform_pro_patch_software_title` ID). A policy belongs to the title it was created against and cannot be moved to another, so changing this forces replacement.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Display name for the patch policy.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the patch policy is enabled. A policy can be enabled only when its scope resolves to at least one in-site smart group. Jamf Pro applies its own default on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"target_version": schema.StringAttribute{
				MarkdownDescription: "The software version this policy deploys (e.g. `8.33.2.2`). Only a version that has a package assigned on the title (`version_packages` on `jamfplatform_pro_patch_software_title`) can be targeted.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"distribution_method": schema.StringAttribute{
				MarkdownDescription: "How the patch is delivered. `selfservice` is the admin UI's \"Make Available in Self Service\", and `prompt` is \"Install Automatically\". Jamf Pro applies its own default on create. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.OneOf(distributionMethods...)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"allow_downgrade": schema.BoolAttribute{
				MarkdownDescription: "**\"Allow downgrade\"** in the Jamf Pro admin UI. Allow installing the target version even when a newer version is present. Jamf Pro applies its own default on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"patch_unknown": schema.BoolAttribute{
				MarkdownDescription: "**\"Patch Unknown Version\"** in the Jamf Pro admin UI. Patch computers whose currently-installed version cannot be determined. Jamf Pro applies its own default on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			// Server-derived from the target_version patch definition. Plain
			// Computed (no plan modifier): they must go Unknown when
			// target_version changes so the new definition's values are read.
			"release_date": schema.Int64Attribute{
				MarkdownDescription: "Release date of the target version's patch definition (UI \"Release Date\"), as a Unix epoch in milliseconds. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"incremental_update": schema.BoolAttribute{
				MarkdownDescription: "Whether the target version's patch definition is an incremental update (UI \"Requires Incremental Update\"). Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"reboot": schema.BoolAttribute{
				MarkdownDescription: "Whether installing the target version requires a reboot (UI \"Reboot Required\"). Returned by Jamf Pro from the patch definition; not user-settable.",
				Computed:            true,
			},
			"minimum_os": schema.StringAttribute{
				MarkdownDescription: "Minimum macOS version required by the target version's patch definition (UI \"Minimum OS\"). Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"kill_apps": schema.ListNestedAttribute{
				MarkdownDescription: "Applications the patch definition closes before installing the target version (UI \"Apps That Must Quit\"). Returned by Jamf Pro from the patch definition; not user-settable.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"kill_app_name": schema.StringAttribute{
							MarkdownDescription: "Display name of the application closed before patching (e.g. `Jamf Composer.app`).",
							Computed:            true,
						},
						"kill_app_bundle_id": schema.StringAttribute{
							MarkdownDescription: "Bundle identifier of the application closed before patching (e.g. `com.jamfsoftware.Composer`).",
							Computed:            true,
						},
					},
				},
			},
			// The Scope tab's UI renders Users / User Groups buttons under
			// Targets and Exclusions, but the /patchpolicies endpoint ignores
			// user-scope entries (LIMITED computer-scope subset). Omitted from
			// the schema; the user-facing description frames this as "does not
			// apply to patch policies".
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "Policy scope, the \"Scope\" tab in the Jamf Pro admin UI. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it stays as configured outside Terraform, preserved across updates. Targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. Setting `all_computers = true` forbids the per-computer, per-group, per-building and per-department targets. Targets, limitations and exclusions are all addressed by computer, computer group, building, department, network segment and iBeacon.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"targets": schema.SingleNestedAttribute{
						MarkdownDescription: "Scope targets: the audience the policy applies to. Mirrors the admin UI's Targets tab. Set `all_computers` for tenant-wide scope, or list specific IDs.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"all_computers": schema.BoolAttribute{
								// Optional-only, matching the shared factories in
								// internal/common/scope: the null/false distinction carries
								// the granular per-category ownership contract, so the flag
								// must not be Computed or carry a state-forwarding plan
								// modifier (see STYLE_GUIDE.md §Scope helper).
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
						},
					},
					"limitations": schema.SingleNestedAttribute{
						MarkdownDescription: "Scope limitations restrict the targets to computers that also match these network segments / iBeacon ranges.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"network_segment_ids": scope.IDSetAttribute("network segment"),
							"ibeacon_ids":         scope.IDSetAttribute("iBeacon"),
						},
					},
					"exclusions": schema.SingleNestedAttribute{
						MarkdownDescription: "Scope exclusions remove items that would otherwise be included by the targets.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"computer_ids":        scope.IDSetAttribute("computer"),
							"computer_group_ids":  scope.IDSetAttribute("computer group"),
							"building_ids":        scope.IDSetAttribute("building"),
							"department_ids":      scope.IDSetAttribute("department"),
							"network_segment_ids": scope.IDSetAttribute("network segment"),
							"ibeacon_ids":         scope.IDSetAttribute("iBeacon"),
						},
					},
				},
			},
			"user_interaction": schema.SingleNestedAttribute{
				// Optional-only (NOT Optional+Computed): the server echoes a full
				// default user_interaction superset on GET. An Optional+Computed
				// SingleNestedAttribute backed by a *struct trips the framework's
				// Unknown-decode at apply (feedback_optional_computed_nested_object).
				// Read is state-gated, so an undeclared block stays null and the
				// server defaults are not surfaced — same tradeoff as scope.
				MarkdownDescription: "User interaction, the \"User Interaction\" tab in the Jamf Pro admin UI. Controls the Self Service description, button text and icon, plus the deferral notifications, deadlines and grace period. Jamf Pro applies full defaults on create when the block, or a nested field, is omitted; those defaults are not surfaced in state unless you declare the block. To clear a stored value, null the individual field rather than deleting the block. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"install_button_text": schema.StringAttribute{
						MarkdownDescription: "Text on the Self Service install button (UI \"Button Name\", under \"Display in Self Service\"). Defaults to `Update`. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"self_service_description": schema.StringAttribute{
						MarkdownDescription: "Description shown in Self Service (UI \"Description\"). Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"self_service_icon_id": schema.StringAttribute{
						MarkdownDescription: "Jamf Pro icon ID shown in Self Service (UI \"Icon\"). Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
					},
					"notifications": schema.SingleNestedAttribute{
						MarkdownDescription: "Notifications shown to the user before the deadline. Omit the block to leave any existing values untouched (they are not cleared on update).",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								MarkdownDescription: "Whether notifications for the patch policy are shown in Notification Center (UI \"Display notifications for the patch policy in Notification Center\", under \"Notifications and Reminders\"). Omit to leave the current value untouched; set `true`/`false` to change it.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
							},
							"subject": schema.StringAttribute{
								MarkdownDescription: "Notification subject. Omit to leave the current value untouched.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
							},
							// message and type are Optional-only (NOT Computed): the
							// classic GET never echoes them — re-confirmed on Jamf Pro
							// 11.31.1 (2026-09-06), where a GET taken straight after the
							// POST that set all four returned notification_enabled,
							// notification_subject and the reminders block and omitted
							// exactly these two. So there is nothing to compute. As
							// Computed they planned Unknown when unset and stayed Unknown
							// after apply whenever the state-gated flatten skipped the
							// block ("invalid result object after apply"). Optional-only
							// ⇒ unset plans as a known null; the sticky read still
							// preserves a user-set value the server drops.
							"message": schema.StringAttribute{
								MarkdownDescription: "Notification message body. Write-only in practice: Jamf Pro does not return it, so a configured value is preserved in state but never refreshed.",
								Optional:            true,
							},
							"type": schema.StringAttribute{
								MarkdownDescription: "Notification type (e.g. `Self Service`). Write-only in practice: Jamf Pro does not return it.",
								Optional:            true,
							},
							"reminders": schema.SingleNestedAttribute{
								MarkdownDescription: "Reminder cadence for the notifications. Omit the block to leave any existing values untouched (they are not cleared on update).",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"enabled": schema.BoolAttribute{
										MarkdownDescription: "Whether reminders are shown. Omit to leave the current value untouched; set `true`/`false` to change it.",
										Optional:            true,
										Computed:            true,
										PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
									},
									"frequency": schema.Int64Attribute{
										MarkdownDescription: "Reminder frequency in hours (UI default `24`). Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
										Optional:            true,
										Computed:            true,
										PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseNonNullStateForUnknown()},
									},
								},
							},
						},
					},
					"deadlines": schema.SingleNestedAttribute{
						MarkdownDescription: "Install deadline after which the patch is enforced. Omit the block to leave any existing values untouched (they are not cleared on update).",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								MarkdownDescription: "Whether a Self Service update deadline is enforced (UI \"Enable Self Service update deadline\", under \"Deadline and Grace Period\"; UI default `true`). Omit to leave the current value untouched; set `true`/`false` to change it.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
							},
							"period": schema.Int64Attribute{
								MarkdownDescription: "Deadline period in days (UI \"Update Deadline\"; default `7`). Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseNonNullStateForUnknown()},
							},
						},
					},
					"grace_period": schema.SingleNestedAttribute{
						MarkdownDescription: "Grace period granted to a user who is actively running a to-be-killed application at install time. Omit the block to leave any existing values untouched (they are not cleared on update).",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"duration": schema.Int64Attribute{
								MarkdownDescription: "Grace period duration in minutes (UI default `15`). Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseNonNullStateForUnknown()},
							},
							"notification_center_subject": schema.StringAttribute{
								MarkdownDescription: "Notification Center subject for the grace-period message (UI default `Important`). Omit to leave the current value untouched.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
							},
							"message": schema.StringAttribute{
								MarkdownDescription: "Grace-period message shown to the user. Omit to leave the current value untouched.",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *PatchPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_policy")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro patch policy ID.
func (r *PatchPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
