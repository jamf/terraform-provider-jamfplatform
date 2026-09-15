// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package activation_code implements the jamfplatform_pro_activation_code singleton
// resource and data source backed by the Jamf ProClassic /activationcode endpoint.
package activation_code

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the classic /activationcode endpoint predates the provider's overall floor.
const minJamfProVersion = ""

// ActivationCodeResource implements the singleton resource for the Jamf Pro activation
// code. Backed by an Update-only ProClassic API (GET + PUT; no Create/Delete on the
// remote): Create funnels into Update; Delete is a no-op that only removes the object
// from Terraform state. The activation code is a license secret — never delete or
// blank it, and never PUT a partial body (a partial write risks wiping the license).
type ActivationCodeResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &ActivationCodeResource{}
var _ resource.ResourceWithImportState = &ActivationCodeResource{}
var _ resource.ResourceWithIdentity = &ActivationCodeResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewActivationCodeResource returns a new instance of ActivationCodeResource.
func NewActivationCodeResource() resource.Resource {
	return &ActivationCodeResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ActivationCodeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_activation_code"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *ActivationCodeResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". The activation code is one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the activation code resource.
func (r *ActivationCodeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage: resourceDeprecation,
		MarkdownDescription: resourceDeprecationCallout +
			"Manages the Jamf Pro activation code and organization name (Settings → System → Activation Code). " +
			"One record per tenant. The activation code is a license secret, and an invalid code can disable the tenant. " +
			"Import with `terraform import jamfplatform_pro_activation_code.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_name": schema.StringAttribute{
				MarkdownDescription: "The organization name registered against the activation code.",
				Required:            true,
			},
			"code": schema.StringAttribute{
				MarkdownDescription: "The Jamf Pro activation code (license key). Treated as sensitive. " +
					"Changing this to an invalid value can disable the tenant, so handle with care.",
				Required:  true,
				Sensitive: true,
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
func (r *ActivationCodeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_activation_code")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error.
//
//	terraform import jamfplatform_pro_activation_code.<name> singleton
func (r *ActivationCodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_activation_code")
}
