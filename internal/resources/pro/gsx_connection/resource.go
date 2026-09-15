// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package gsx_connection implements the jamfplatform_pro_gsx_connection_settings singleton
// resource and data source backed by the Jamf Pro GSX Connection settings API
// (Settings > Global > GSX connection).
package gsx_connection

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the GSX Connection settings endpoint is present at the provider's overall floor,
// matching every other settings singleton. The provider-level advisory still fires through
// providerdata.ConfigurePro.
const minJamfProVersion = ""

// GsxConnectionSettingsResource implements the singleton resource for Jamf Pro GSX
// Connection settings. Backed by an Update-only API (no Create/Delete on the remote):
// Create funnels into a full-replace PUT; Delete is a no-op that only removes the object
// from Terraform state.
//
// Write semantics (wire-probed 2026-06-09 against a live tenant):
//   - PUT /pro/v1/gsx-connection mandates `token` + `gsxKeystore` on EVERY write
//     (FIELD_REQUIRED otherwise), regardless of `enabled`.
//   - The server validates the supplied keystore against Apple's live GSX service on every
//     write, so a valid Apple-registered certificate is required for any write to succeed
//     (a self-signed cert is rejected with a GSX 401).
//   - Update therefore re-sends the full body — including the three secrets — on every
//     apply (Design B). This re-validates the certificate against Apple per apply.
type GsxConnectionSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &GsxConnectionSettingsResource{}
var _ resource.ResourceWithImportState = &GsxConnectionSettingsResource{}
var _ resource.ResourceWithIdentity = &GsxConnectionSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewGsxConnectionSettingsResource returns a new instance of the resource.
func NewGsxConnectionSettingsResource() resource.Resource {
	return &GsxConnectionSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *GsxConnectionSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_gsx_connection_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept only
// the fixed helpers.SingletonID value.
func (r *GsxConnectionSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". GSX Connection settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// writeOnlySecret returns the schema for one of the three Required write-only secrets.
// They are Required (not Optional): the GSX PUT mandates token + keystore on every write,
// so the value must always be present in config. WriteOnly keeps it out of state; there is
// no _wo_version companion because the "omit when unchanged" path can never apply.
func writeOnlySecret(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " Required and `WriteOnly`: sent to Jamf Pro on every apply, never persisted in Terraform state, and never returned on read. The certificate is re-validated against Apple's GSX service on every write, so this must always be present in config. To rotate the stored value, change it in config.",
		Required:            true,
		Sensitive:           true,
		WriteOnly:           true,
	}
}

// Schema returns the Terraform schema for the GSX Connection settings resource.
func (r *GsxConnectionSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro GSX Connection settings (Settings > Global > GSX connection). " +
			"Connects Jamf Pro to Apple's Global Service Exchange (GSX) for warranty, repair, and purchase-date lookups. " +
			"One record per tenant. " +
			"**Requires a valid Apple-registered GSX certificate.** Every apply re-validates the certificate, token, and account against Apple's live GSX service; a self-signed or invalid certificate is rejected. " +
			"Secrets are re-sent on every apply: `token_wo`, `keystore_bytes_wo`, and `keystore_password_wo` are Required and `WriteOnly`, so they are never stored in state and must always be present in config. " +
			"Import with `terraform import jamfplatform_pro_gsx_connection_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable connection to GSX\"** in the Jamf Pro admin UI. Whether the GSX connection is enabled. Omit to preserve the current value (adopted on first apply, left untouched on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "**\"Username\"** in the Jamf Pro admin UI. The GSX account email. The account needs Manager privileges and access to Web Services.",
				Required:            true,
			},
			"service_account_number": schema.StringAttribute{
				MarkdownDescription: "**\"GSX Account Number\"** in the Jamf Pro admin UI. The GSX service account number.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(maxAccountNumberLength),
				},
			},
			"ship_to_number": schema.StringAttribute{
				MarkdownDescription: "**\"GSX Ship-To Number\"** in the Jamf Pro admin UI. The GSX ship-to number. Optional; omit to preserve the current value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtMost(maxAccountNumberLength),
				},
			},
			"token_wo": writeOnlySecret(
				"**\"API Token\"** in the Jamf Pro admin UI. The GSX API token retrieved from your Apple GSX account.",
			),
			"keystore_bytes_wo": writeOnlySecret(
				"**\"Upload the GSX certificate or keystore\"** in the Jamf Pro admin UI. The base64-encoded GSX certificate keystore (p12). Supply with `filebase64(\"certificate.p12\")`.",
			),
			"keystore_password_wo": writeOnlySecret(
				"**\"Enter the keystore password\"** in the Jamf Pro admin UI. The password protecting the GSX certificate keystore.",
			),
			"keystore_name": schema.StringAttribute{
				MarkdownDescription: "The certificate keystore filename (e.g. `certificate.p12`). Optional; omit to preserve the current value, which Jamf Pro derives from the uploaded keystore.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"keystore_error_message": schema.StringAttribute{
				MarkdownDescription: "Read-only certificate validation error reported by Jamf Pro, if the uploaded keystore is invalid. Empty when the certificate is valid.",
				Computed:            true,
			},
			"keystore_expiration_epoch": schema.Int64Attribute{
				MarkdownDescription: "Read-only certificate expiry, in epoch milliseconds, derived by Jamf Pro from the uploaded keystore.",
				Computed:            true,
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
func (r *GsxConnectionSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_gsx_connection_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID value
// is accepted; any other identifier is rejected with a clear error.
//
//	terraform import jamfplatform_pro_gsx_connection_settings.<name> singleton
func (r *GsxConnectionSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_gsx_connection_settings")
}
