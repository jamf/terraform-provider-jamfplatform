// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mdm_profile_settings implements the jamfplatform_pro_mdm_profile_settings
// singleton resource and data source backed by the Jamf Pro device communication settings API.
package mdm_profile_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the device communication settings endpoint is present at the provider's overall
// floor, so no per-resource gate is needed.
const minJamfProVersion = ""

// MDMProfileSettingsResource implements the singleton resource for Jamf Pro
// device communication settings. Backed by an Update-only API (no Create/Delete on the
// remote): Create funnels into Update; Delete is a no-op that only removes the object
// from Terraform state.
type MDMProfileSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &MDMProfileSettingsResource{}
var _ resource.ResourceWithImportState = &MDMProfileSettingsResource{}
var _ resource.ResourceWithIdentity = &MDMProfileSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewMDMProfileSettingsResource returns a new instance of MDMProfileSettingsResource.
func NewMDMProfileSettingsResource() resource.Resource {
	return &MDMProfileSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *MDMProfileSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mdm_profile_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *MDMProfileSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Device communication settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the device communication settings resource.
func (r *MDMProfileSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro device communication settings (Settings → Device communication → MDM profile settings). " +
			"Your tenant has one. " +
			"Import with `terraform import jamfplatform_pro_mdm_profile_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"auto_renew_computer_profile_when_ca_renewed": schema.BoolAttribute{
				MarkdownDescription: "When the certificate authority is renewed, automatically renew the computer MDM profile. " +
					"Maps to the \"Automatically renew the MDM profile when the built-in CA is renewed\" computer control under MDM profile settings. Omit to leave the current value untouched on update; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"auto_renew_computer_profile_before_expiry": schema.BoolAttribute{
				MarkdownDescription: "Automatically renew the computer MDM profile before its device identity certificate expires. " +
					"Maps to the \"Automatically renew the MDM profile before the device identity certificate expires\" computer control under MDM profile settings. Omit to leave the current value untouched on update; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"computer_profile_expiration_limit_days": schema.Int64Attribute{
				MarkdownDescription: "Number of days before the computer device identity certificate expires at which Jamf Pro begins renewing the MDM profile. " +
					"Maps to the computer \"expiration limit (in days)\" field under MDM profile settings. Omit to leave the current value untouched on update; an integer has no blank-clear, so set a concrete value to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"auto_renew_mobile_device_profile_when_ca_renewed": schema.BoolAttribute{
				MarkdownDescription: "When the certificate authority is renewed, automatically renew the mobile device MDM profile. " +
					"Maps to the \"Automatically renew the MDM profile when the built-in CA is renewed\" mobile device control under MDM profile settings. Omit to leave the current value untouched on update; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"auto_renew_mobile_device_profile_before_expiry": schema.BoolAttribute{
				MarkdownDescription: "Automatically renew the mobile device MDM profile before its device identity certificate expires. " +
					"Maps to the \"Automatically renew the MDM profile before the device identity certificate expires\" mobile device control under MDM profile settings. Omit to leave the current value untouched on update; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"mobile_device_profile_expiration_limit_days": schema.Int64Attribute{
				MarkdownDescription: "Number of days before the mobile device identity certificate expires at which Jamf Pro begins renewing the MDM profile. " +
					"Maps to the mobile device \"expiration limit (in days)\" field under MDM profile settings. Omit to leave the current value untouched on update; an integer has no blank-clear, so set a concrete value to change it.",
				Optional: true,
				Computed: true,
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *MDMProfileSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mdm_profile_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error so users do
// not accidentally end up with mis-keyed state that the resource silently normalizes
// on the next Read.
//
//	terraform import jamfplatform_pro_mdm_profile_settings.<name> singleton
func (r *MDMProfileSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_mdm_profile_settings")
}
