// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mac_app_store_app implements the jamfplatform_pro_mac_app_store_app
// resource, data source, and list resource backed by the Jamf ProClassic
// /macapplications API. The construct name mirrors the Jamf Pro admin UI
// ("App Store App" under the "Mac Apps" sidebar) and disambiguates from the
// mobile App Store App equivalent. Scope is computer-only and omits iBeacon
// targets (the endpoint silently drops them) via the shared computer-scope
// helper with IncludeIbeacons=false.
package mac_app_store_app

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /macapplications predates the provider's overall
// floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

// MacAppResource implements the Terraform resource for Jamf Pro App Store Mac apps.
type MacAppResource struct {
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
	client *proclassic.Client
	// ldapSearcher backs the plan-time directory-service user-group preflight in
	// ModifyPlan. The LDAP group search is a Pro (v1) endpoint, so it is a
	// separate client from the ProClassic CRUD client. nil until Configure runs.
	ldapSearcher ldapgroups.Searcher
}

var _ resource.Resource = &MacAppResource{}
var _ resource.ResourceWithImportState = &MacAppResource{}
var _ resource.ResourceWithIdentity = &MacAppResource{}
var _ resource.ResourceWithModifyPlan = &MacAppResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewMacAppResource returns a new instance of MacAppResource.
func NewMacAppResource() resource.Resource {
	return &MacAppResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *MacAppResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mac_app_store_app"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *MacAppResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Mac App Store app ID used to uniquely reference the app.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource. Attribute names mirror
// the Jamf Pro admin UI labels; differing wire element names are noted in the
// attribute descriptions.
func (r *MacAppResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro App Store Mac app, the \"App Store App\" entry under the \"Mac Apps\" sidebar. `general.name`, `general.version`, `general.bundle_id` and `general.url` are required on create and stored verbatim: no App Store metadata is resolved from the URL. Scope targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. Scope omits iBeacon limitations and exclusions because Jamf Pro silently drops them.\n\nUpdates are merged rather than replaced. Removing a whole optional block (`self_service` or `vpp`) from your configuration does not clear it: Jamf Pro keeps the values you set previously. To clear a block, null its individual fields instead of deleting the block." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"general": schema.SingleNestedAttribute{
				MarkdownDescription: "General settings. `name`, `version`, `bundle_id`, and `url` are required on create. Read-only fields (`category_name`, `site_name`, `id`) are returned by Jamf Pro.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "App ID under `general`. Matches the top-level `id`. Returned by Jamf Pro.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "App display name. Must be unique within the tenant.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "App version string. Stored verbatim; Jamf Pro does not resolve it from the App Store.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"bundle_id": schema.StringAttribute{
						MarkdownDescription: "App bundle identifier (e.g. `com.apple.iMovieApp`). Stored verbatim; Jamf Pro does not resolve it from the App Store.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"url": schema.StringAttribute{
						MarkdownDescription: "App Store (iTunes) URL. Required on create: Jamf Pro rejects a create without it. Stored verbatim, and does not auto-populate `name`, `version` or `bundle_id`.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"is_free": optComputedBool("Whether the app is free. Server-defaults to false on create. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"deployment_type": schema.StringAttribute{
						// Optional+Computed and the server always echoes a value
						// (defaults to "Make Available in Self Service"), so an
						// unset deployment_type must stay Unknown on create rather
						// than copying the null prior state — otherwise the
						// server's default trips the post-apply consistency check.
						// UseNonNullStateForUnknown behaves like UseStateForUnknown
						// once a value is present.
						MarkdownDescription: "Install method. One of `Make Available in Self Service` or `Install Automatically/Prompt Users to Install`. Server-defaults to `Make Available in Self Service` on create. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(deploymentTypeSelfService, deploymentTypeAutomatic),
						},
					},
					"category_id": optComputedString("Jamf Pro category ID. Use `-1` for \"No category\". Omit to leave the current value untouched."),
					"category_name": schema.StringAttribute{
						// No UseStateForUnknown: category_name is derived from
						// category_id, so it must go Unknown (not pin the stale
						// value) when category_id changes, or the post-apply
						// consistency check trips.
						MarkdownDescription: "Category display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"site_id": optComputedString("Jamf Pro site ID scoping the app. Use `-1` for \"No site\". Omit to leave the current value untouched."),
					"site_name": schema.StringAttribute{
						// No UseStateForUnknown: site_name is derived from site_id
						// (same rationale as category_name above).
						MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
				},
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "App scope. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it stays as configured outside Terraform, preserved across updates. Targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. Setting `all_computers = true` forbids `computer_ids`, `computer_group_ids`, `building_ids` and `department_ids`. Setting `all_jss_users = true` forbids `user_ids` and `user_group_ids`. iBeacon limitations and exclusions are deliberately absent, because Jamf Pro silently drops them.",
				Optional:            true,
				Attributes:          scope.ComputerScopeAttributes(scope.ComputerScopeOptions{IncludeIbeacons: false}),
			},
			"self_service": schema.SingleNestedAttribute{
				MarkdownDescription: "Self Service integration. Relevant when `general.deployment_type` is `Make Available in Self Service`. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"install_button_text":             optComputedString("Install-button label shown on the app's Self Service page. Omit to leave the current value untouched."),
					"self_service_description":        optComputedString("Self Service description. Markdown supported. Omit to leave the current value untouched."),
					"force_users_to_view_description": optComputedBool("Force users to view the description before installing. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"feature_on_main_page":            optComputedBool("Feature the app on the Self Service main page. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_enabled":            optComputedBool("Whether Self Service surfaces a notification when the app becomes available. Pair with `notification_method`. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_method":             optComputedString("Notification delivery method (e.g. `Self Service`). The server defaults a method when notifications are enabled. Omit to leave the current value untouched."),
					"notification_subject":            optComputedString("Notification subject line. Omit to leave the current value untouched."),
					"notification_message":            optComputedString("Notification body text. Omit to leave the current value untouched."),
					"self_service_icon": schema.SingleNestedAttribute{
						MarkdownDescription: "Self Service icon. Set `id` to reference an already-uploaded icon; `uri` is returned by Jamf Pro. Uploading icon bytes inline is not supported, because Jamf Pro re-encodes PNGs and the result would diff forever. Open an issue if you need it.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"id":  optComputedString("Icon ID assigned by Jamf Pro."),
							"uri": optComputedString("Icon URI. Returned by Jamf Pro."),
						},
					},
					"self_service_categories": schema.SetNestedAttribute{
						MarkdownDescription: "Set of Self Service categories the app appears under. Each item identifies the category by `id`; `name` is returned by Jamf Pro.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":         schema.StringAttribute{MarkdownDescription: "Category ID.", Required: true},
								"name":       optComputedString("Category display name. Returned by Jamf Pro."),
								"display_in": optComputedBool("Display the app in this category."),
								"feature_in": optComputedBool("Feature the app in this category."),
							},
						},
					},
				},
			},
			"vpp": schema.SingleNestedAttribute{
				MarkdownDescription: "Volume Purchasing (VPP) assignment. `assign_vpp_device_based_licenses` and `vpp_admin_account_id` are writable only for a genuinely VPP-backed title. Setting `assign_vpp_device_based_licenses = true` on a non-VPP app is rejected with error 409, \"App is not available for device assignment\". The license counts are calculated by Jamf Pro. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"assign_vpp_device_based_licenses": optComputedBool("Assign VPP device-based licenses. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"vpp_admin_account_id":             optComputedString("VPP admin account ID. `-1` when the app is not VPP-backed. Omit to leave the current value untouched."),
					"total_vpp_licenses":               computedInt64("Total VPP licenses. Returned by Jamf Pro."),
					"remaining_vpp_licenses":           computedInt64("Remaining VPP licenses. Returned by Jamf Pro."),
					"used_vpp_licenses":                computedInt64("Used VPP licenses. Returned by Jamf Pro."),
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
func (r *MacAppResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mac_app_store_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client

	// Also obtain a Pro (v1) client for the scope directory-service group
	// preflight. Same provider data and version contract; ConfigurePro returns
	// (nil, nil) during early lifecycle, leaving the preflight a no-op until the
	// provider is fully configured.
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mac_app_store_app")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if proClient != nil {
		r.ldapSearcher = proClient
	}
}

// ImportState handles import by the Jamf Pro app ID.
func (r *MacAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan runs the plan-time directory-service user-group preflight on the
// scope limitations/exclusions: each directory_service_user_group_names entry
// is matched against the tenant's configured LDAP / cloud-IdP, surfacing an
// unknown group as a clear plan error instead of the opaque apply-time 409
// ("Problem matching limitation user group"). Best-effort: a search transport
// error or an unconfigured directory downgrades to a warning. No-op on destroy
// (null plan) and when no scope groups are declared.
func (r *MacAppResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Runs ahead of any guard below: an object entering or leaving management
	// changes what its scope receives, so creates and destroys are reported too.
	r.reportScopeImpact(ctx, req, resp)

	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}

	var plan MacAppResourceModel
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
// UseNonNullStateForUnknown plan modifier for server-augmented fields. Used
// both at top level and inside SetNested list elements — see the policy
// resource doc comment for why UseNonNullStateForUnknown (not UseStateForUnknown)
// is required for nested-list growth.
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

// computedInt64 returns a Computed-only Int64Attribute for server-derived
// values (VPP license counts).
func computedInt64(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
}
