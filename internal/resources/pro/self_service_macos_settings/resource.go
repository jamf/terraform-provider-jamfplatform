// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package self_service_macos_settings implements the
// jamfplatform_pro_self_service_macos_settings singleton resource and data source backed
// by the Jamf Pro Self Service settings API (Settings > Self Service > macOS).
package self_service_macos_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the self-service settings endpoint is present at the provider's overall floor, so
// no per-resource gate is needed.
const minJamfProVersion = ""

// SelfServiceMacosSettingsResource implements the singleton resource for the Jamf Pro
// Self Service for macOS app settings. Backed by an Update-only API (no Create/Delete on
// the remote): Create funnels into Update; Delete is a no-op that only removes the object
// from Terraform state.
type SelfServiceMacosSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &SelfServiceMacosSettingsResource{}
var _ resource.ResourceWithImportState = &SelfServiceMacosSettingsResource{}
var _ resource.ResourceWithIdentity = &SelfServiceMacosSettingsResource{}
var _ resource.ResourceWithConfigValidators = &SelfServiceMacosSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewSelfServiceMacosSettingsResource returns a new instance of the resource.
func NewSelfServiceMacosSettingsResource() resource.Resource {
	return &SelfServiceMacosSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *SelfServiceMacosSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_macos_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept only
// the fixed helpers.SingletonID value.
func (r *SelfServiceMacosSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Self Service settings are one record per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators returns the cross-field validators evaluated at plan time: the
// server-enforced install-location requirement, the silent-coercion guard on the default
// home category, and the Saml-requires-login rule. All defer when either side is null or
// unknown.
func (r *SelfServiceMacosSettingsResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		installLocationRequiredValidator{},
		categoryRequiresBrowseValidator{},
		samlRequiresLoginValidator{},
	}
}

// ModifyPlan absorbs the server-side coercion on a login_method → "NotRequired" transition:
// disabling Self Service user login makes Jamf Pro revert a stored "Saml" authentication
// type to "Basic" (wire-probed 2026-06-10 — the PUT succeeds and the echo/GET carry Basic).
// When the plan carries "Saml" only via UseStateForUnknown (the user did not declare
// authentication_type) while login_method is planned "NotRequired", the carried value can
// never survive the apply — mark it Unknown so the post-write GET supplies the server's
// resolved value instead of tripping "inconsistent result after apply". A user-declared
// Saml + NotRequired pair is rejected at plan time by samlRequiresLoginValidator.
func (r *SelfServiceMacosSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy plan — nothing to modify
	}

	var loginMethod, plannedAuthType, configAuthType types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("login_method"), &loginMethod)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("authentication_type"), &plannedAuthType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("authentication_type"), &configAuthType)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if predictAuthTypeUnknown(loginMethod, plannedAuthType, configAuthType) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("authentication_type"), types.StringUnknown())...)
	}
}

// predictAuthTypeUnknown reports whether the planned authentication_type is a carried (not
// user-declared) "Saml" that the server will coerce away because login_method is planned
// "NotRequired".
func predictAuthTypeUnknown(loginMethod, plannedAuthType, configAuthType types.String) bool {
	return !loginMethod.IsNull() && !loginMethod.IsUnknown() && loginMethod.ValueString() == pro.SelfServiceLoginSettingsUserLoginLevelNotRequired &&
		!plannedAuthType.IsNull() && !plannedAuthType.IsUnknown() && plannedAuthType.ValueString() == pro.SelfServiceLoginSettingsAuthTypeSaml &&
		configAuthType.IsNull()
}

// Schema returns the Terraform schema for the Self Service macOS settings resource.
func (r *SelfServiceMacosSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Self Service for macOS app settings (Settings > Self Service > macOS). " +
			"One record per tenant. " +
			"A field you omit keeps its current Jamf Pro value, including on the first apply: " +
			"this resource adopts the existing settings and only changes the fields you declare. " +
			"`default_home_category_id` only applies when `default_landing_page = \"BROWSE\"`. Under any other landing " +
			"page Jamf Pro silently resets it to `-1` (All Items), so the provider rejects that combination at plan time. " +
			"Import with `terraform import jamfplatform_pro_self_service_macos_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"install_automatically": schema.BoolAttribute{
				MarkdownDescription: "Whether the Self Service app is installed automatically on computers (\"Install automatically\"). " +
					"When `true`, `install_location` must hold a valid path. " +
					"Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"install_location": schema.StringAttribute{
				MarkdownDescription: "Path at which the Self Service app is installed (\"Install location\"), e.g. `/Applications`. " +
					"The app filename (\"/ Self Service.app\") is appended by Jamf Pro and is not part of this value. " +
					"Stored verbatim; no leading slash is added for you. " +
					"Required to be non-empty when `install_automatically` is `true`. " +
					"Omit to leave the current value untouched.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"login_method": schema.StringAttribute{
				MarkdownDescription: "Self Service user login behavior. The UI's \"Enable Self Service user login\" checkbox and " +
					"\"Login method\" dropdown both map to this single value. " +
					"`NotRequired` disables user login. `Anonymous` makes login optional: users may log in to view items " +
					"available to them. `Required` forces users to log in. " +
					"Setting `NotRequired` while the authentication type is `Saml` makes Jamf Pro revert it to `Basic` " +
					"and disable \"Single Sign-On for Self Service for macOS\" in the tenant's Single Sign-On settings " +
					"(`jamfplatform_pro_sso_settings.sso_for_macos_self_service_enabled`), the one write on this page " +
					"that reaches outside it. " +
					"Omit to leave the current value untouched.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(pro.SelfServiceLoginSettingsUserLoginLevelValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"authentication_type": schema.StringAttribute{
				MarkdownDescription: "Login type used when asking users to log in (\"Authentication type\"). " +
					"`Basic` is a Directory Service account or Jamf Pro user account. `Saml` is Single Sign-On. " +
					"Setting `Saml` requires \"Single Sign-On for Self Service for macOS\" to be enabled in the tenant's " +
					"Single Sign-On settings (`jamfplatform_pro_sso_settings` attribute `sso_for_macos_self_service_enabled`); " +
					"Jamf Pro otherwise rejects the write (PREREQUISITE_NOT_MET). " +
					"Requires `login_method` `Anonymous` or `Required`. Disabling user login makes Jamf Pro revert this " +
					"to `Basic` **and switch that Single Sign-On toggle off** (see `login_method`). " +
					"Omit to leave the current value untouched.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(pro.SelfServiceLoginSettingsAuthTypeValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"keychain_credential_storage_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether users may store their login credentials in Keychain Access " +
					"(\"Allow users to store their login credentials in Keychain Access\"). " +
					"Only applies when `authentication_type = \"Basic\"`; under `Saml` the value is retained but inert. " +
					"Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"fido2_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether FIDO2 authentication is enabled (\"Enable FIDO2 authentication\"). " +
					"Only applies when `authentication_type = \"Saml\"`; under `Basic` the value is retained but inert. " +
					"Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"notifications_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Self Service notifications are displayed for items available to users " +
					"(\"Enable Self Service notifications\"). " +
					"Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"alert_user_approved_mdm": schema.BoolAttribute{
				MarkdownDescription: "Whether users are notified in Self Service or Notification Center that they must approve " +
					"the organization's MDM profile (\"Enable User Approved MDM profile notification\"). " +
					"Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"default_landing_page": schema.StringAttribute{
				MarkdownDescription: "Content that displays when Self Service opens (\"Landing page\"): " +
					"`HOME`, `BROWSE`, `HISTORY`, or `NOTIFICATIONS`. " +
					"Omit to leave the current value untouched.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(pro.SelfServiceInteractionSettingsDefaultLandingPageValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_home_category_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the category shown when `default_landing_page = \"BROWSE\"` (Landing page > Browse > " +
					"\"Category\"). `-1` means All Items. Reference a category with `jamfplatform_pro_category`. " +
					"Only applies under `BROWSE`: with any other landing page Jamf Pro silently resets the value to `-1`, " +
					"so a value other than `-1` requires `default_landing_page = \"BROWSE\"` to be declared alongside it. " +
					"Omit to leave the current value untouched.",
				Optional: true,
				Computed: true,
				Validators: []validator.Int64{
					int64validator.AtLeast(-4),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"bookmarks_display_name": schema.StringAttribute{
				MarkdownDescription: "Name to display for the Bookmarks section in Self Service (\"Bookmarks display name\"), " +
					"e.g. \"Plug-ins\" or \"Websites\". Must not be blank. " +
					"Omit to leave the current value untouched.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
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
func (r *SelfServiceMacosSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_macos_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID value is
// accepted; any other identifier is rejected with a clear error so users do not accidentally
// end up with mis-keyed state that the resource silently normalizes on the next Read.
//
//	terraform import jamfplatform_pro_self_service_macos_settings.<name> singleton
func (r *SelfServiceMacosSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_self_service_macos_settings")
}
