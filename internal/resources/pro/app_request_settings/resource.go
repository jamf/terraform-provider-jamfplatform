// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package app_request_settings implements the jamfplatform_pro_app_request_settings
// singleton resource backed by the Jamf Pro App Request settings API (Settings → Self
// Service → App Request).
package app_request_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the App Request settings endpoint is present at the provider's overall floor.
const minJamfProVersion = ""

// AppRequestSettingsResource implements the singleton resource for Jamf Pro App Request
// settings. Backed by a GET/PUT-only API (no Create/Delete on the remote): Create funnels
// into Update; Delete is a no-op that only removes the object from Terraform state.
type AppRequestSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &AppRequestSettingsResource{}
var _ resource.ResourceWithImportState = &AppRequestSettingsResource{}
var _ resource.ResourceWithIdentity = &AppRequestSettingsResource{}
var _ resource.ResourceWithModifyPlan = &AppRequestSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAppRequestSettingsResource returns a new instance of the resource.
func NewAppRequestSettingsResource() resource.Resource {
	return &AppRequestSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AppRequestSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_request_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept only
// the fixed helpers.SingletonID value.
func (r *AppRequestSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". App Request settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the App Request settings resource.
func (r *AppRequestSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro App Request settings (Settings → Self Service → App Request). " +
			"App Request lets Self Service users on iOS request apps that admins then approve. One record per tenant. " +
			"This resource adopts the existing settings on first apply. Omitting `enabled`, `app_store_locale` or `requester_user_group_id` keeps that field at its current Jamf Pro value. `approver_emails` is required and always reflects exactly the addresses you declare, an empty set clearing every approver. " +
			"Import with `terraform import jamfplatform_pro_app_request_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Enable App Requests in Self Service on iOS. When `true`, `requester_user_group_id` must reference a valid static user group, the tenant must have at least one App Request form field (`jamfplatform_pro_app_request_form_field`), and an SMTP server must be configured (`jamfplatform_pro_smtp_server`) so approval emails can be sent. Where Terraform creates the form field in the same run, add a `depends_on`. Omit to leave the current value untouched.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"app_store_locale": schema.StringAttribute{
				MarkdownDescription: "App Store country or region used to resolve requested apps. Either the literal `deviceLocale`, which follows each device's own locale, or an upper-case ISO 3166-1 alpha-2 country code such as `US`. Validated at plan time against the tenant's supported list; the `jamfplatform_pro_app_store_country_codes` data source returns that list. Omit to leave the current value untouched.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"approver_emails": schema.SetAttribute{
				MarkdownDescription: "Email addresses of the App Request approvers. The set is applied exactly as declared: removing an address revokes it, and an empty set clears every approver. The Jamf Pro admin UI expects at least one approver while App Requests are enabled, so declare an empty set only where they are switched off.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"requester_user_group_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the static Jamf Pro user group whose members may request apps (see `jamfplatform_pro_user_group`). Required when `enabled` is `true`, and it must reference a static group; Jamf Pro rejects a smart group or an unknown ID. Only valid while `enabled` is `true`: when App Requests are disabled the group is cleared and may not be set. Omit it while enabled to leave the current value untouched.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
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

// ModifyPlan runs the plan-time preflights against the resolved plan: the App Store locale
// is validated against the tenant's supported country codes, and an enabled configuration
// is checked for a requester user group. Both surface as plan errors rather than apply-time
// 400s; a locale fetch failure or unconfigured client downgrades to a warning. No-op on
// destroy.
func (r *AppRequestSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan AppRequestSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The config (explicit user choice) drives the requester cross-field rules; the plan
	// value is UseStateForUnknown-resolved and may carry a group forward during a disable.
	var configRequester types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("requester_user_group_id"), &configRequester)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupPath := path.Root("requester_user_group_id")
	resp.Diagnostics.Append(validateEnabledRequiresRequesterGroup(plan.Enabled, plan.RequesterUserGroupID, groupPath)...)
	resp.Diagnostics.Append(validateRequesterRequiresEnabled(plan.Enabled, configRequester, groupPath)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// When App Requests are disabled and the user did not set a requester group, clear any
	// group carried from prior state so the plan matches the disabled write (which sends
	// null). Safe to override only because the config value is null (the attribute is
	// Computed). An explicitly-set group while disabled is rejected above.
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() && !plan.Enabled.ValueBool() &&
		configRequester.IsNull() && !plan.RequesterUserGroupID.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, groupPath, types.Int64Null())...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if r.client != nil {
		resp.Diagnostics.Append(validateAppStoreLocale(ctx, r.client, plan.AppStoreLocale, path.Root("app_store_locale"))...)
	}
}

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *AppRequestSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_request_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID value
// is accepted.
//
//	terraform import jamfplatform_pro_app_request_settings.<name> singleton
func (r *AppRequestSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_app_request_settings")
}
