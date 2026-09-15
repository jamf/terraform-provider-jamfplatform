// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mobile_device_app implements the jamfplatform_pro_mobile_device_app
// resource, data source, and list resource backed by the Jamf ProClassic
// /mobiledeviceapplications API. The construct name mirrors the Jamf Pro admin
// UI ("App Store App" / in-house app under the "Mobile Device Apps" sidebar)
// and disambiguates from the Mac App Store App equivalent. The resource models
// the app's metadata for whatever the app is (App Store, manually added, or
// in-house) — there is no IPA binary upload endpoint, so binary upload is not
// modeled. Scope is mobile-device-only and omits iBeacon targets (the endpoint
// drops them) via the shared mobile-scope helper with IncludeIbeacons=false.
package mobile_device_app

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /mobiledeviceapplications predates the provider's
// overall floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

// MobileAppResource implements the Terraform resource for Jamf Pro mobile device apps.
type MobileAppResource struct {
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
	client *proclassic.Client
	// ldapSearcher backs the plan-time directory-service user-group preflight in
	// ModifyPlan. The LDAP group search is a Pro (v1) endpoint, so it is a
	// separate client from the ProClassic CRUD client. nil until Configure runs.
	ldapSearcher ldapgroups.Searcher
}

var _ resource.Resource = &MobileAppResource{}
var _ resource.ResourceWithImportState = &MobileAppResource{}
var _ resource.ResourceWithIdentity = &MobileAppResource{}
var _ resource.ResourceWithModifyPlan = &MobileAppResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	// defaultDeleteTimeout bounds the whole Delete: the classic
	// /mobiledeviceapplications DELETE returns a misleading HTTP 400 on an
	// accepted, asynchronous removal, so Delete issues it once then polls
	// GET-by-id until not-found. 2 minutes is comfortable headroom before a
	// still-present app is reported as a failure.
	defaultDeleteTimeout = 2 * time.Minute
	// deletePollInterval is how often Delete re-checks GET-by-id while confirming
	// the async removal.
	deletePollInterval = 10 * time.Second
)

// NewMobileAppResource returns a new instance of MobileAppResource.
func NewMobileAppResource() resource.Resource {
	return &MobileAppResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *MobileAppResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_app"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *MobileAppResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro mobile device app ID used to uniquely reference the app.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource. Attribute names mirror
// the Jamf Pro admin UI labels; differing wire element names are noted in the
// attribute descriptions.
func (r *MobileAppResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro mobile device app: the \"App Store App\" and in-house app entries under the \"Mobile Device Apps\" sidebar. The resource models the app's metadata only; uploading an in-house binary (IPA) is not supported. `general.name`, `general.version` and `general.bundle_id` are required. `general.os_type` is required only for in-house apps, and an App Store app carrying an `itunes_store_url` does not need it. Scope targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. iBeacon scope limitations and exclusions are not supported for mobile device apps.\n\nUpdates are merged rather than replaced. Removing a whole optional block (`scope`, `self_service`, `vpp` or `app_configuration`) from your configuration does not clear it: Jamf Pro keeps the values you set previously. To clear a block, null its individual fields instead of deleting the block." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"general": schema.SingleNestedAttribute{
				MarkdownDescription: "General settings. `name`, `version`, and `bundle_id` are required; `os_type` is required only for in-house apps. Read-only fields (`description`, `category_name`, `site_name`, `id`) are returned by Jamf Pro. The app's display name always equals `name`, so it is not modeled separately.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "App ID under `general`. Matches the top-level `id`. Returned by Jamf Pro.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "App display name. Must be unique within the tenant; also used as the app's display name in Self Service.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "App version string.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"bundle_id": schema.StringAttribute{
						MarkdownDescription: "App bundle identifier (e.g. `com.apple.Maps`). Required on create.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"os_type": schema.StringAttribute{
						MarkdownDescription: "Operating system the app targets. One of `iOS` or `tvOS`. Required for in-house apps; App Store apps (those with an `itunes_store_url`) do not need it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(osTypeIOS, osTypeTVOS),
						},
					},
					"description": mirrorString(
						"App description, which Jamf Pro keeps in step with `self_service.self_service_description`, and syncs from the App Store while `keep_description_and_icon_up_to_date = true`. Set `self_service.self_service_description` to change it.",
						planmodifiers.ObjectSource(path.MatchRoot("self_service")),
					),
					"is_free": optComputedBool("Whether the app is free. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"deployment_type": mirrorString(
						"Install method, shown in the admin UI as \"Distribution Method\": `Make Available in Self Service` while `deploy_automatically` is `false`, and `Install Automatically/Prompt Users to Install` while it is `true`. Set `deploy_automatically` to choose the install method.",
						planmodifiers.BoolSource(path.MatchRoot("general").AtName("deploy_automatically")),
					),
					"external_url":                           optComputedString("External / in-house hosting URL. Independent of the App Store URL; setting it flips `host_externally` to true server-side. Omit to leave the current value untouched."),
					"itunes_store_url":                       optComputedString("Canonical App Store (iTunes) URL. Setting it also populates the deprecated `url` mirror server-side. Omit to leave the current value untouched."),
					"itunes_country_region":                  optComputedString("Two-letter App Store country/region code used to resolve store metadata. Omit to leave the current value untouched."),
					"itunes_sync_time":                       optComputedInt64("App Store sync time as a Unix epoch. Maintained by Jamf Pro."),
					"category_id":                            optComputedString("Jamf Pro category ID. Use `-1` for \"No category\". Omit to leave the current value untouched."),
					"category_name":                          computedString("Category display name. Returned by Jamf Pro; not user-settable."),
					"site_id":                                optComputedString("Jamf Pro site ID scoping the app. Use `-1` for \"No site\". Omit to leave the current value untouched."),
					"site_name":                              computedString("Site display name. Returned by Jamf Pro; not user-settable."),
					"make_available_after_install":           optComputedBool("Make the app available in Self Service after it is installed automatically. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"keep_description_and_icon_up_to_date":   optComputedBool("Keep the app description and icon in sync with the App Store listing. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"keep_app_updated_on_devices":            optComputedBool("Automatically update the app on managed devices when a new version ships. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"deploy_as_managed_app":                  optComputedBool("Deploy as a managed app (enables managed-app capabilities such as app configuration). Omit to leave the current value untouched; set `true`/`false` to change it."),
					"take_over_management":                   optComputedBool("Take over management of the app if it is already installed unmanaged. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"deploy_automatically":                   optComputedBool("Automatically push the app to in-scope devices. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"remove_app_when_mdm_profile_is_removed": optComputedBool("Remove the app from a device when its MDM profile is removed. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"prevent_backup_of_app_data":             optComputedBool("Prevent the app's data from being backed up to iCloud / iTunes. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"allow_user_to_delete":                   optComputedBool("Allow the user to delete the managed app. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"require_network_tethered":               optComputedBool("Require a network-tethered connection to install. Relevant for automatically-deployed apps. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"host_externally":                        optComputedBool("Host the app externally (in-house hosting). Flips to true automatically when an `external_url` or App Store URL is set. Omit to leave the current value untouched; set `true`/`false` to change it."),
				},
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "App scope. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it stays as configured outside Terraform, preserved across updates. Targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services. Setting `all_mobile_devices = true` forbids `mobile_device_ids`, `mobile_device_group_ids`, `building_ids` and `department_ids`. Setting `all_jss_users = true` forbids `user_ids` and `user_group_ids`. iBeacon limitations and exclusions are not supported for mobile device apps.",
				Optional:            true,
				Attributes:          scope.MobileScopeAttributes(scope.MobileScopeOptions{IncludeIbeacons: false}),
			},
			"self_service": schema.SingleNestedAttribute{
				MarkdownDescription: "Self Service integration. Relevant when `general.deployment_type` is `Make Available in Self Service`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"install_button_text": optComputedString("Install-button label shown on the app's Self Service page. Omit to leave the current value untouched."),
					"after_install_button_text": schema.StringAttribute{
						MarkdownDescription: "Button label shown after the app is installed. Requires `general.make_available_after_install = true`; Jamf Pro discards the label otherwise and never returns it. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators:          []validator.String{requiresMakeAvailableAfterInstall{}},
					},
					"self_service_description": optComputedString("Self Service description. Markdown supported. Omit to leave the current value untouched."),
					"feature_on_main_page":     optComputedBool("Feature the app on the Self Service main page. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_enabled":     optComputedBool("Whether Self Service surfaces a notification when the app becomes available. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_subject":     optComputedString("Notification subject line. Omit to leave the current value untouched."),
					"notification_message":     optComputedString("Notification body text. Omit to leave the current value untouched."),
					"self_service_icon": schema.SingleNestedAttribute{
						MarkdownDescription: "Self Service icon. Set `id` to reference an already-uploaded icon (e.g. `jamfplatform_pro_icon`); `uri` is returned by Jamf Pro. Uploading icon bytes inline is not supported.",
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
								"name":       optComputedStringInList("Category display name. Returned by Jamf Pro."),
								"display_in": optComputedBoolInList("Display the app in this category."),
							},
						},
					},
				},
			},
			"vpp": schema.SingleNestedAttribute{
				MarkdownDescription: "Volume Purchasing (VPP) assignment. `assign_vpp_device_based_licenses` and `vpp_admin_account_id` are writable only for a genuinely VPP-backed title; setting `assign_vpp_device_based_licenses = true` on an app that is not VPP-backed is rejected.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"assign_vpp_device_based_licenses": optComputedBool("Assign VPP device-based licenses. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"vpp_admin_account_id":             optComputedString("VPP admin account ID. `-1` when the app is not VPP-backed. Omit to leave the current value untouched."),
				},
			},
			"app_configuration": schema.SingleNestedAttribute{
				MarkdownDescription: "Managed-app configuration. `preferences` carries the app-configuration plist; CRLF and LF newlines are treated as equivalent so an import or admin-UI edit does not permadiff against an LF-authored config.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"preferences": schema.StringAttribute{
						MarkdownDescription: "App-configuration property list content. Whitespace, indentation, and newline-style differences are ignored when comparing, so reformatting the same configuration does not show as a change. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{preferencesNewlineSemanticEquality{}},
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
// providerdata.ConfigureProClassic helper, plus a Pro (v1) client for the scope
// directory-service group preflight.
func (r *MobileAppResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client

	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_app")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if proClient != nil {
		r.ldapSearcher = proClient
	}
}

// ImportState handles import by the Jamf Pro app ID.
func (r *MobileAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan runs the plan-time directory-service user-group preflight on the
// scope limitations/exclusions: each directory_service_user_group_names entry
// is matched against the tenant's configured LDAP / cloud-IdP, surfacing an
// unknown group as a clear plan error instead of the opaque apply-time 409
// ("Problem matching limitation user group"). Best-effort: a search transport
// error or an unconfigured directory downgrades to a warning. No-op on destroy
// (null plan) and when no scope groups are declared.
func (r *MobileAppResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Runs ahead of any guard below: an object entering or leaving management
	// changes what its scope receives, so creates and destroys are reported too.
	r.reportScopeImpact(ctx, req, resp)

	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}

	var plan MobileAppResourceModel
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

// optComputedString returns a top-level Optional+Computed StringAttribute with
// UseStateForUnknown. UseStateForUnknown copies the prior state value — INCLUDING
// null — into an unset plan, so a field the server omits when unconfigured (e.g.
// self_service.after_install_button_text, which the server returns absent → null)
// stays null on every plan instead of churning to "(known after apply)". For
// fields the server does default (e.g. install_button_text → "Install") the prior
// state is non-null and the behaviour is identical to UseNonNullStateForUnknown.
//
// Do NOT use this inside SetNested/ListNested elements: list growth produces a
// null prior for the new element, which UseStateForUnknown would copy as a known
// null and then trip "inconsistent result" when the element materialises — use
// optComputedStringInList there.
func optComputedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// optComputedBool is the bool sibling of optComputedString (top-level).
func optComputedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
	}
}

// optComputedStringInList is the SetNested/ListNested-element flavour of
// optComputedString: it uses UseNonNullStateForUnknown so a new element's null
// prior is NOT copied as a known value (which would break the post-apply
// consistency check as the set grows). See the policy/profile resources for why
// nested-list Optional+Computed must use the non-null variant.
func optComputedStringInList(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
	}
}

// optComputedBoolInList is the bool sibling of optComputedStringInList.
func optComputedBoolInList(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
	}
}

// optComputedInt64 is the Int64 sibling of optComputedString. Int64 has no
// UseNonNullStateForUnknown in the stock plan modifiers; UseStateForUnknown is
// safe here because itunes_sync_time is a single top-level scalar (not nested in
// a growing list), so the null-prior-state consistency trap does not apply.
func optComputedInt64(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
}

// computedString returns a Computed-only StringAttribute for server-managed
// fields (description, category_name, site_name) with UseStateForUnknown so
// no-op plans stay empty (the standard pattern shared with mac_app_store_app).
//
// These echo values the server derives from other inputs (category_name/
// site_name from category_id/site_id, description from the App Store sync). If a
// user changes the driving input, the plan shows the stale echo and the apply
// recomputes it — the accepted ProClassic latent posture (mac_app carries the
// same).
//
// NB: this is intentionally NOT used for display_name (deterministically ==
// name) or internal_app (server-flips based on the store/hosting inputs) —
// those echoes would trip "inconsistent result after apply" when their driving
// input changes, so they are not modeled at all (callers read `name`, and
// in-house status is derivable from whether a store/external URL is set).
func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// mirrorString is computedString for an attribute Jamf Pro derives from a
// SIBLING this resource manages, rather than assigns independently.
//
// UseStateForUnknown is wrong for a mirror: it promises the value cannot change
// unless the practitioner changes this attribute, but it changes when the
// sibling changes, so the plan carries the stale one into an apply that returns
// the new one — "Provider produced inconsistent result after apply". Dropping
// the modifier is also wrong: the attribute then goes Unknown on every plan and
// `terraform plan -refresh=false` is permanently dirty for a value nobody
// touched. planmodifiers.MirrorOfString does the only correct thing, which is
// to watch the source.
//
// general.description mirrors self_service.self_service_description and
// general.deployment_type mirrors general.deploy_automatically, both wire-proven
// against Jamf Pro 11.31.1 on 2026-09-07 — see flattenMobileAppGeneral.
//
// description watches the whole self_service block rather than the description
// within it: self_service is Optional, and a path into a null block resolves
// only as far as the block. See planmodifiers.ObjectSource.
func mirrorString(desc string, sources ...planmodifiers.SourceComparer) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{planmodifiers.MirrorOfString(sources...)},
	}
}
