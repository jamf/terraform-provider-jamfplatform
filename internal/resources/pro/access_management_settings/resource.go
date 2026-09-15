// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package access_management_settings implements the
// jamfplatform_pro_access_management_settings singleton resource and data source
// backed by the Jamf Pro Access Management settings API
// (/v4/enrollment/access-management).
//
// Access Management for Managed Apple Accounts: when access-management controls
// are enabled in Apple Business Manager / Apple School Manager, this setting names
// the Automated Device Enrollment (ADE) server object Jamf Pro returns in its Get
// Token response, so ABM/ASM can restrict Managed Apple Account sign-in to managed
// or supervised devices. There is no admin-UI panel for this today (a future Jamf
// Pro version will add one); it is API-only.
package access_management_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: Access Management settings require Jamf Pro 11.18.0, which is below the
// provider's overall floor (providerdata.ProviderMinJamfProVersion), so no
// per-resource gate is needed — the provider-wide advisory already covers it.
const minJamfProVersion = ""

// AccessManagementSettingsResource implements the singleton resource for Jamf Pro
// Access Management settings. Backed by an Update-only API (no Create/Delete on the
// remote): Create funnels into Update; Delete is a no-op that only removes the object
// from Terraform state.
type AccessManagementSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &AccessManagementSettingsResource{}
var _ resource.ResourceWithImportState = &AccessManagementSettingsResource{}
var _ resource.ResourceWithIdentity = &AccessManagementSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAccessManagementSettingsResource returns a new instance of the resource.
func NewAccessManagementSettingsResource() resource.Resource {
	return &AccessManagementSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AccessManagementSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_access_management_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *AccessManagementSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Access Management settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Access Management settings resource.
func (r *AccessManagementSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro Access Management settings for Managed Apple Accounts. One record per tenant. " +
			"When access-management controls are enabled in Apple Business Manager / Apple School Manager, this names the Automated Device Enrollment (ADE) server object Jamf Pro returns in its Get Token response, so ABM/ASM can restrict Managed Apple Account sign-in to managed or supervised devices only. " +
			"The ADE server object must belong to the same ABM/ASM tenant the Managed Apple Accounts originate from; only one tenant can be configured at a time. " +
			"Requires Jamf Pro 11.18.0 or later and an ADE (MDM server) token configured in Jamf Pro. " +
			"Omitting `automated_device_enrollment_server_uuid` keeps the value currently set on the tenant, including on the first apply: this resource adopts the existing setting. " +
			"To clear the setting, set `automated_device_enrollment_server_uuid = \"\"`. Omitting it does not clear it. " +
			"Import with `terraform import jamfplatform_pro_access_management_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"automated_device_enrollment_server_uuid": schema.StringAttribute{
				MarkdownDescription: "Server UUID of the Automated Device Enrollment (ADE) server object Jamf Pro names in its Get Token response for Managed Apple Account access management. " +
					"Copy it from an ADE instance (`jamfplatform_pro_automated_device_enrollment.<name>.server_uuid`) or from Settings > Automated Device Enrollment in the Jamf Pro web app. " +
					"The server object must be associated with the same Apple Business Manager / Apple School Manager tenant the Managed Apple Accounts originate from. " +
					"Omit to preserve the current value; set to `\"\"` to clear it (no ADE server configured).",
				Optional: true,
				Computed: true,
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
func (r *AccessManagementSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_access_management_settings")
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
//	terraform import jamfplatform_pro_access_management_settings.<name> singleton
func (r *AccessManagementSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_access_management_settings")
}
