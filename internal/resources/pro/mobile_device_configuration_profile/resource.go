// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required. Empty:
// the classic /mobiledeviceconfigurationprofiles endpoint predates the
// provider's declared floor.
const minJamfProVersion = ""

// Resource implements jamfplatform_pro_mobile_device_configuration_profile.
type Resource struct {
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
	client *proclassic.Client
	// ldapSearcher backs the plan-time scope directory-service user-group
	// preflight (ModifyPlan). The LDAP group search is a Pro (v1) endpoint, so
	// it is a separate client from the ProClassic CRUD client. nil until Configure.
	ldapSearcher ldapgroups.Searcher
}

var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
	_ resource.ResourceWithIdentity    = &Resource{}
	_ resource.ResourceWithModifyPlan  = &Resource{}
)

const (
	defaultCreateTimeout = 90 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 90 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewResource returns a new Resource instance.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata sets the Terraform type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_configuration_profile"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro mobile device configuration profile ID.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mobile device configuration profile in Jamf Pro. The `general.payloads` attribute carries the raw `.mobileconfig` plist XML for the configuration the profile delivers to enrolled iOS, iPadOS and tvOS devices.\n\n### Payload diff handling\n\nJamf Pro normalises every uploaded payload: it assigns its own top-level identifiers, fills in default values for fields you omit, and re-serialises the XML. The provider hides those normalisations from `terraform plan`, so applies stay quiet when nothing meaningful has changed. Real drift still surfaces in two cases:\n\n  - You edited the payload in Terraform. `plan` shows the change and `apply` pushes it.\n  - Someone edited the profile in the Jamf Pro admin UI. `plan` shows the drift on the next refresh, so you can either bring the change back into your Terraform config or `apply` to reassert the Terraform-managed value.\n\nA small set of profile-level fields (`PayloadDisplayName`, `PayloadIdentifier`, `PayloadUUID`, `PayloadOrganization`, `PayloadDescription`, `PayloadEnabled`) is managed entirely by Jamf Pro. Any value you supply for them inside `payloads` is replaced, so the provider ignores them in the diff. Use `general.name`, `general.description` and the other top-level attributes to control the equivalent fields.\n\n### Scope\n\nScope blocks mirror `jamfplatform_pro_policy`. Targets, limitations and exclusions all carry flat sets of Jamf Pro IDs, or directory-service names where appropriate. `all_mobile_devices` and `all_jss_users` conflict with their per-ID siblings.\n\n### Profile identity on update\n\nOn update the provider re-applies the existing top-level `PayloadUUID` and `PayloadIdentifier` from state into every payload it sends back to Jamf Pro, so the profile's identity stays stable across applies. Without that, every update would look like a brand-new profile to enrolled devices and the OS would treat it as a fresh installation.\n\n### Characters Jamf Pro cannot store\n\n`&` and `<` come back with an extra layer of escaping, line feeds and tabs are removed, and emoji are replaced. This affects nearly every payload type here, including \"Application & Custom Settings\"; a web clip URL with a query string is the usual way to meet it. Write line breaks as `&#13;`. Rather than alter a payload silently, the provider refuses it on create, on edit and on import, naming the offending value." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Profile ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"general": schema.SingleNestedAttribute{
				MarkdownDescription: "Profile general settings. `name` and `payloads` are required.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "Profile ID under `general`. Matches the top-level `id`. Assigned by Jamf Pro.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "Display name of the profile. Must be unique within the tenant. This value is also used as the profile's display name inside the `.mobileconfig` payload, so any name you set inside `payloads` is overridden.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"description": optComputedString(
						"Free-text description of the profile. It doubles as the Self Service description, because Jamf Pro stores one for both. To set those separately, use the profile's Options and Self Service tabs and leave this attribute unset. Omit to leave the current value untouched.",
					),
					"level": schema.StringAttribute{
						MarkdownDescription: "Profile delivery level. Mirrors the admin UI dropdown: `Device Level` (default) or `User Level`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validLevels...),
						},
					},
					"distribution_method": schema.StringAttribute{
						MarkdownDescription: "How the profile reaches devices. `Install Automatically` pushes via MDM; `Make Available in Self Service` lists the profile in Self Service so users install it manually. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validDistributionMethods...),
						},
					},
					"redeploy_on_update": schema.StringAttribute{
						MarkdownDescription: "Redeployment behaviour when the profile changes. Valid values: `Newly Assigned` (push to newly-scoped devices only) or `All` (push to every scoped device on the next update). Jamf Pro does not echo this value back after it is set, so the provider treats it as write-only: once you set it, later refreshes will not snap it back to a default. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"redeploy_days_before_certificate_expires": schema.Int64Attribute{
						MarkdownDescription: "Number of days before a certificate in the profile expires that should trigger redeployment. `0` disables certificate-expiry redeployment. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
					},
					"uuid": schema.StringAttribute{
						MarkdownDescription: "Profile UUID assigned by Jamf Pro on creation. Also surfaces as the top-level `PayloadUUID` inside the `.mobileconfig` payload. Read-only.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"payloads": schema.StringAttribute{
						MarkdownDescription: "The `.mobileconfig` plist XML carrying the configuration the profile delivers. Must be a complete plist document; Jamf Pro rejects a bare dictionary fragment. See the resource description for how the provider handles diffs against Jamf Pro's own normalisations, and for the characters Jamf Pro cannot store inside a payload value.",
						Required:            true,
						Validators:          []validator.String{validators.PlistDocument()},
					},
					"category_id": schema.StringAttribute{
						MarkdownDescription: "Jamf Pro category ID. Use `-1` (default) for \"no category\". Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"category_name": schema.StringAttribute{
						// No UseStateForUnknown: derived from the mutable category_id, so it
						// must go Unknown when category_id changes. See STYLE_GUIDE §886.
						MarkdownDescription: "Category display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"site_id": schema.StringAttribute{
						MarkdownDescription: "Jamf Pro site ID. Use `-1` (default) for \"no site\". Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"site_name": schema.StringAttribute{
						// No UseStateForUnknown: derived from the mutable site_id, so it
						// must go Unknown when site_id changes. See STYLE_GUIDE §886.
						MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
				},
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "Profile scope. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it stays as configured outside Terraform, preserved across updates. `all_mobile_devices = true` forbids the per-device, per-group, per-building and per-department targets. `all_jss_users = true` forbids the per-user and per-user-group targets. `user_ids` and `user_group_ids` map to the admin UI's \"Users\" and \"User Groups\" lists.",
				Optional:            true,
				Attributes:          scope.MobileScopeAttributes(scope.MobileScopeOptions{IncludeIbeacons: true}),
			},
			"self_service": schema.SingleNestedAttribute{
				MarkdownDescription: "Self Service integration. Only meaningful when `general.distribution_method = \"Make Available in Self Service\"`. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"feature_on_main_page": optComputedBool("Feature the profile on the Self Service main page. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"removal_disallowed": schema.StringAttribute{
						MarkdownDescription: "Removal-by-end-user policy. Valid values: `Never`, `Always`, `With Authorization`. Pair `With Authorization` with `authorization_password` to require a password at removal time. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validRemovalDisallowedValues...),
						},
					},
					"authorization_password": schema.StringAttribute{
						MarkdownDescription: "Authorization password required to remove the profile. Only effective when `removal_disallowed = \"With Authorization\"`. Jamf Pro stores and returns the value in plaintext, so it is held in Terraform state and masked in plan/apply output.",
						Optional:            true,
						Sensitive:           true,
					},
					"categories": schema.ListNestedAttribute{
						MarkdownDescription: "Categories under which the profile appears in Self Service. Listing a category displays the profile in it, matching the admin UI's \"Display in\" tick; Jamf Pro keeps no undisplayed state and offers no per-category \"Feature in\" control for mobile profiles, so neither is exposed here. Declaring one or more entries replaces the stored list. An empty list reads as an omission. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":   schema.StringAttribute{MarkdownDescription: "Category ID.", Required: true},
								"name": optComputedStringInList("Category display name. Returned by Jamf Pro."),
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(context.Background(), timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// Configure wires the Jamf ProClassic client into the resource.
func (r *Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_configuration_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client

	// Pro (v1) client for the scope directory-service group preflight.
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_configuration_profile")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if proClient != nil {
		r.ldapSearcher = proClient
	}
}

// ImportState handles import by Jamf Pro profile ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func optComputedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func optComputedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
	}
}

func optComputedStringInList(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
	}
}

// _ tracks the types import — the model fields use it implicitly.
var _ = types.StringNull
