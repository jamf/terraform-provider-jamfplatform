// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package self_service_plus_settings implements the jamfplatform_pro_self_service_plus_settings
// singleton resource and data source backed by the Jamf Pro Self Service Plus settings API.
package self_service_plus_settings

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
// Empty: the Self Service Plus settings endpoint is present at the provider's overall
// floor, so no per-resource gate is needed.
const minJamfProVersion = ""

// SelfServicePlusSettingsResource implements the singleton resource for Jamf Pro
// Self Service Plus settings. Backed by an Update-only API (no Create/Delete on the
// remote): Create funnels into Update; Delete is a no-op that only removes the
// object from Terraform state.
type SelfServicePlusSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &SelfServicePlusSettingsResource{}
var _ resource.ResourceWithImportState = &SelfServicePlusSettingsResource{}
var _ resource.ResourceWithIdentity = &SelfServicePlusSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewSelfServicePlusSettingsResource returns a new instance of SelfServicePlusSettingsResource.
func NewSelfServicePlusSettingsResource() resource.Resource {
	return &SelfServicePlusSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *SelfServicePlusSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_plus_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *SelfServicePlusSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Self Service Plus settings are one record per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Self Service Plus settings resource.
func (r *SelfServicePlusSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro Self Service Plus settings. One record per tenant. " +
			"Import with `terraform import jamfplatform_pro_self_service_plus_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Self Service Plus is enabled for the tenant.",
				Required:            true,
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
func (r *SelfServicePlusSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_plus_settings")
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
//	terraform import jamfplatform_pro_self_service_plus_settings.<name> singleton
func (r *SelfServicePlusSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_self_service_plus_settings")
}
