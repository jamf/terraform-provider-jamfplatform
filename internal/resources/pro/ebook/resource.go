// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ebook implements the jamfplatform_pro_ebook resource, data source,
// and list resource backed by the Jamf ProClassic /ebooks API. The construct
// name mirrors the Jamf Pro admin UI ("eBooks" under the Users sidebar).
//
// Ebook scope is the classic dual-target union — computer targets AND
// mobile-device targets AND user targets, plus the ebook-specific `class_ids`
// target — hand-composed from the shared scope sub-block primitives rather than
// the single-target computer/mobile factories (the union+classes shape is
// ebook's own). There are NO iBeacon targets anywhere in ebook scope.
package ebook

import (
	"context"
	"time"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /ebooks predates the provider's overall floor. The
// provider-level advisory still fires through providerdata.ConfigureProClassic
// when the tenant is below the floor.
const minJamfProVersion = ""

// EbookResource implements the Terraform resource for Jamf Pro ebooks.
type EbookResource struct {
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
	client *proclassic.Client
	// ldapSearcher backs the plan-time directory-service user-group preflight in
	// ModifyPlan. The LDAP group search is a Pro (v1) endpoint, so it is a
	// separate client from the ProClassic CRUD client. nil until Configure runs.
	ldapSearcher ldapgroups.Searcher
}

var _ resource.Resource = &EbookResource{}
var _ resource.ResourceWithImportState = &EbookResource{}
var _ resource.ResourceWithIdentity = &EbookResource{}
var _ resource.ResourceWithModifyPlan = &EbookResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	// defaultUpdateTimeout bounds up to four classic round trips, not the usual
	// two: the read-merge-write GET, then — when the merged scope carries class
	// members — a clearing PUT and a restoring PUT, then the refresh GET. The
	// restoring PUT is the only request whose failure leaves class_ids empty in
	// Jamf Pro, and it is last in the queue, so the budget has to cover the three
	// ahead of it with room to spare. 3 minutes also leaves the directory-group
	// match-conflict retry (helpers.RetryOnDirectoryGroupMatchConflict, 90s of
	// its own) able to spend its full deadline on one PUT without starving the
	// restore. See ebookScopeClassesCleared in input_builders.go.
	defaultUpdateTimeout = 3 * time.Minute
	// defaultDeleteTimeout bounds the whole Delete call: a single DELETE plus the
	// GET-by-id poll that confirms the async /ebooks removal landed. A single
	// delete clears in ~16s; 2 minutes is comfortable headroom before Delete
	// errors out a still-present ebook.
	defaultDeleteTimeout = 2 * time.Minute
)

// NewEbookResource returns a new instance of EbookResource.
func NewEbookResource() resource.Resource {
	return &EbookResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *EbookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ebook"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *EbookResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro ebook ID used to uniquely reference the ebook.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource. Attribute names mirror
// the Jamf Pro admin UI labels; differing wire element names are noted in the
// attribute descriptions.
func (r *EbookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro ebook (the \"eBooks\" entry under the Users sidebar). Distributes either an in-house file (PDF, EPUB or iBook hosted at `general.url`) or an Apple Books title (a `books.apple.com` URL). For App Store ebooks Jamf Pro derives `general.file_type` and `general.version` from the URL, so leave them unset. Scope is the dual-target union of computers, mobile devices and users, plus `class_ids`; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Ebook ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"general": schema.SingleNestedAttribute{
				MarkdownDescription: "General settings. `name` and `url` are required on create. Read-only fields (`category_name`, `site_name`, `id`) are returned by Jamf Pro.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "Ebook ID under `general`. Matches the top-level `id`. Returned by Jamf Pro.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "Ebook display name. Must be unique within the tenant.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"url": schema.StringAttribute{
						MarkdownDescription: "Ebook URL. For an in-house ebook this is where the file is hosted; for an App-Store ebook this is the Apple Books (`books.apple.com`) page. Required on create.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"author": optComputedString("Ebook author. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"deployment_type": schema.StringAttribute{
						// Optional+Computed and the server always echoes a value,
						// so an unset deployment_type must stay Unknown on create
						// rather than copying the null prior state — otherwise the
						// server's default trips the post-apply consistency check.
						MarkdownDescription: "Distribution Method. One of `Make Available in Self Service` or `Install Automatically/Prompt Users to Install`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(deploymentTypeSelfService, deploymentTypeAutomatic),
						},
					},
					"deploy_as_managed": optComputedBool("Make the ebook managed when possible (UI \"Make eBook managed when possible\"). Omit to leave the current value untouched; set `true`/`false` to change it."),
					"free":              optComputedBool("Whether the ebook is free. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"file_type": optComputedString(
						"File Type. User-set for an in-house ebook (`PDF`, `EPUB`, `IBOOK`). For an App Store ebook, leave it unset: Jamf Pro resolves it from the Apple Books URL and returns it. No strict value validation is applied, because Jamf Pro canonicalises the casing. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
					),
					"version":     optComputedString("Ebook version. User-set for an in-house ebook; returned by Jamf Pro for an App Store ebook. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"category_id": optComputedString("Jamf Pro category ID. Omit to leave the current value untouched; set `-1` to clear the category."),
					"category_name": schema.StringAttribute{
						// No UseStateForUnknown: category_name is derived from
						// category_id, so it must go Unknown (not pin the stale
						// value) when category_id changes.
						MarkdownDescription: "Category display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"site_id": optComputedString("Jamf Pro site ID scoping the ebook. Omit to leave the current value untouched; set `-1` to clear the site."),
					"site_name": schema.StringAttribute{
						// No UseStateForUnknown: site_name is derived from site_id.
						MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
				},
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "Ebook scope: the dual-target union. Computer targets, mobile-device targets, user targets, and `class_ids` all coexist. Each category is independently owned. Declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it is left as configured outside Terraform, and updates preserve it. Setting `all_computers = true` forbids `computer_ids` / `computer_group_ids`; `all_mobile_devices = true` forbids `mobile_device_ids` / `mobile_device_group_ids`; `all_jss_users = true` forbids `user_ids` / `user_group_ids`. Targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. There are no iBeacon targets.",
				Optional:            true,
				Attributes:          ebookScopeAttributes(),
			},
			"self_service": schema.SingleNestedAttribute{
				MarkdownDescription: "Self Service integration. Relevant when `general.deployment_type` is `Make Available in Self Service`. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"display_name":                    optComputedString("Self Service display name (UI \"Self Service Display Name\", in-house ebooks). Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"install_button_text":             optComputedString("Install-button label (UI \"Button Name\", macOS only). Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"self_service_description":        optComputedString("Self Service description. Markdown supported. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"force_users_to_view_description": optComputedBool("Force users to view the description before installing (macOS only). Omit to leave the current value untouched; set `true`/`false` to change it."),
					"feature_on_main_page":            optComputedBool("Feature the ebook on the Self Service main page. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_enabled":            optComputedBool("Whether Self Service surfaces a notification when the ebook becomes available (macOS only). Pair with `notification_method`. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_method":             optComputedString("Notification delivery method (e.g. `Self Service`). The server defaults a method when notifications are enabled. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"notification_subject":            optComputedString("Notification subject line. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"notification_message":            optComputedString("Notification body text. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"icon_id":                         optComputedString("Self Service icon ID. Reference an already-uploaded icon (e.g. `jamfplatform_pro_icon.<x>.id`); App-Store ebooks auto-populate it from the store artwork. Uploading icon bytes inline is not supported. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."),
					"icon_uri": schema.StringAttribute{
						// Derived from icon_id; plain Computed (no UseStateForUnknown).
						MarkdownDescription: "Self Service icon URI. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"categories": schema.SetNestedAttribute{
						MarkdownDescription: "Set of Self Service categories the ebook appears under (macOS and iOS app only). Each item identifies the category by `id`; `name` is returned by Jamf Pro. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":         schema.StringAttribute{MarkdownDescription: "Category ID.", Required: true},
								"name":       optComputedString("Category display name. Returned by Jamf Pro."),
								"display_in": optComputedBool("Display the ebook in this category."),
								"feature_in": optComputedBool("Feature the ebook in this category."),
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

// ebookScopeAttributes hand-composes the ebook <scope> attribute map from the
// shared scope sub-block primitives, splitting into targets / limitations /
// exclusions to mirror the admin UI tabs. The all-flags and per-category target
// ID sets nest under `targets`. Ebook's union+classes shape is its own — it
// deliberately does NOT reuse scope.ComputerScopeAttributes /
// scope.MobileScopeAttributes (those are single-target sugar). The three
// all-flags use value-discriminated AllFlagConflictsWith validators with
// relative paths (so they resolve against their sibling sets inside `targets`).
//
// Every attribute in the block — the per-category sets and the all-flags — is
// Optional-only, matching the shared factories in internal/common/scope: the
// null/`[]` (or null/`false`) distinction carries the granular per-category
// ownership contract, so nothing here may be Computed or carry a
// state-forwarding plan modifier (see STYLE_GUIDE.md §Scope helper).
func ebookScopeAttributes() map[string]schema.Attribute {
	limitations := map[string]schema.Attribute{
		"network_segment_ids":                   scope.IDSetAttribute("network segment"),
		"directory_service_or_local_user_names": scope.NameSetAttribute("directory service or local user"),
		"directory_service_user_group_names":    scope.NameSetAttribute("directory service user group"),
	}
	exclusions := map[string]schema.Attribute{
		"computer_ids":                          scope.IDSetAttribute("computer"),
		"computer_group_ids":                    scope.IDSetAttribute("computer group"),
		"mobile_device_ids":                     scope.IDSetAttribute("mobile device"),
		"mobile_device_group_ids":               scope.IDSetAttribute("mobile device group"),
		"building_ids":                          scope.IDSetAttribute("building"),
		"department_ids":                        scope.IDSetAttribute("department"),
		"user_ids":                              scope.IDSetAttribute("user"),
		"user_group_ids":                        scope.IDSetAttribute("user group"),
		"network_segment_ids":                   scope.IDSetAttribute("network segment"),
		"directory_service_or_local_user_names": scope.NameSetAttribute("directory service or local user"),
		"directory_service_user_group_names":    scope.NameSetAttribute("directory service user group"),
	}

	targets := map[string]schema.Attribute{
		"all_computers": schema.BoolAttribute{
			MarkdownDescription: "Scope to every computer in the tenant. Forbids `computer_ids` / `computer_group_ids` when true. Omit to leave the toggle as configured outside Terraform.",
			Optional:            true,
			Validators: []validator.Bool{
				scope.AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("computer_ids"),
					path.MatchRelative().AtParent().AtName("computer_group_ids"),
				),
			},
		},
		"all_mobile_devices": schema.BoolAttribute{
			MarkdownDescription: "Scope to every mobile device in the tenant. Forbids `mobile_device_ids` / `mobile_device_group_ids` when true. Omit to leave the toggle as configured outside Terraform.",
			Optional:            true,
			Validators: []validator.Bool{
				scope.AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("mobile_device_ids"),
					path.MatchRelative().AtParent().AtName("mobile_device_group_ids"),
				),
			},
		},
		"all_jss_users": schema.BoolAttribute{
			MarkdownDescription: "Scope to every Jamf Pro user in the tenant. Forbids `user_ids` / `user_group_ids` when true. Omit to leave the toggle as configured outside Terraform.",
			Optional:            true,
			Validators: []validator.Bool{
				scope.AllFlagConflictsWith(
					path.MatchRelative().AtParent().AtName("user_ids"),
					path.MatchRelative().AtParent().AtName("user_group_ids"),
				),
			},
		},
		"computer_ids":            scope.IDSetAttribute("computer"),
		"computer_group_ids":      scope.IDSetAttribute("computer group"),
		"mobile_device_ids":       scope.IDSetAttribute("mobile device"),
		"mobile_device_group_ids": scope.IDSetAttribute("mobile device group"),
		"building_ids":            scope.IDSetAttribute("building"),
		"department_ids":          scope.IDSetAttribute("department"),
		"user_ids":                scope.IDSetAttribute("user"),
		"user_group_ids":          scope.IDSetAttribute("user group"),
		"class_ids":               scope.IDSetAttribute("class"),
	}

	return map[string]schema.Attribute{
		"targets": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope targets: the audience the ebook applies to. Mirrors the admin UI's Targets tab: set `all_computers` / `all_mobile_devices` / `all_jss_users` for tenant-wide scope, or list specific IDs (the dual-target union of computers, mobile devices, users, and classes). Omit the block to leave any existing values untouched (they are not cleared on update).",
			Optional:            true,
			Attributes:          targets,
		},
		"limitations": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope limitations narrow the audience after the targets resolve. `directory_service_or_local_user_names` and `directory_service_user_group_names` carry names (not IDs) because that is how Jamf Pro identifies these directory-service objects. Omit the block to leave any existing values untouched (they are not cleared on update).",
			Optional:            true,
			Attributes:          limitations,
		},
		"exclusions": schema.SingleNestedAttribute{
			MarkdownDescription: "Scope exclusions remove items that would otherwise be included by targets or limitations. Omit the block to leave any existing values untouched (they are not cleared on update).",
			Optional:            true,
			Attributes:          exclusions,
		},
	}
}

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper, plus a Pro (v1) client for the scope
// directory-service group preflight.
func (r *EbookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ebook")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client

	// Pro (v1) client for the scope directory-service group preflight. Same
	// provider data and version contract; ConfigurePro returns (nil, nil) during
	// early lifecycle, leaving the preflight a no-op until the provider is fully
	// configured.
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ebook")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if proClient != nil {
		r.ldapSearcher = proClient
	}
}

// ImportState handles import by the Jamf Pro ebook ID.
func (r *EbookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan runs the plan-time directory-service user-group preflight on the
// scope limitations/exclusions: each directory_service_user_group_names entry is
// matched against the tenant's configured LDAP / cloud-IdP, surfacing an unknown
// group as a clear plan error instead of the opaque apply-time failure.
// Best-effort: a search transport error or an unconfigured directory downgrades
// to a warning. No-op on destroy (null plan) and when no scope groups are declared.
func (r *EbookResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Runs ahead of any guard below: an object entering or leaving management
	// changes what its scope receives, so creates and destroys are reported too.
	r.reportScopeImpact(ctx, req, resp)

	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}

	var plan EbookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Scope == nil {
		return
	}

	scopeRoot := path.Root("scope")
	if plan.Scope.Limitations != nil {
		resp.Diagnostics.Append(scope.ValidateDirectoryServiceUserGroupNames(
			ctx, r.ldapSearcher, plan.Scope.Limitations.DirectoryServiceUserGroupNames,
			scopeRoot.AtName("limitations").AtName("directory_service_user_group_names"),
		)...)
	}
	if plan.Scope.Exclusions != nil {
		resp.Diagnostics.Append(scope.ValidateDirectoryServiceUserGroupNames(
			ctx, r.ldapSearcher, plan.Scope.Exclusions.DirectoryServiceUserGroupNames,
			scopeRoot.AtName("exclusions").AtName("directory_service_user_group_names"),
		)...)
	}
}

// optComputedString returns an Optional+Computed StringAttribute with the
// UseNonNullStateForUnknown plan modifier for server-augmented fields. Used both
// at top level and inside SetNested list elements — UseNonNullStateForUnknown
// (not UseStateForUnknown) is required for nested-list growth (see
// feedback_writeonly_nested_attrs).
func optComputedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
	}
}

// optComputedBool is the bool sibling of optComputedString.
func optComputedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
	}
}
