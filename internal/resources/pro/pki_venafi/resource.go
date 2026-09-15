// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package venafi implements the jamfplatform_pro_pki_venafi resource and data
// source backed by the Jamf Pro Venafi certificate-authority API
// (`/api/v1/pki/venafi`). In the Jamf Pro admin UI these live under Settings →
// Global → PKI certificates → Certificate Authorities.
//
// NOTE: the underlying Jamf Pro endpoints are a PREVIEW API and may change.
package pki_venafi

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty string skips the version check — the Venafi PKI endpoints
// are a preview API with no published version floor; the provider's overall
// floor (11.0.0) governs.
const minJamfProVersion = ""

// PkiVenafiResource implements the Terraform resource for Jamf Pro Venafi CAs.
type PkiVenafiResource struct {
	client *pro.Client
}

var _ resource.Resource = &PkiVenafiResource{}
var _ resource.ResourceWithImportState = &PkiVenafiResource{}
var _ resource.ResourceWithIdentity = &PkiVenafiResource{}
var _ resource.ResourceWithModifyPlan = &PkiVenafiResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPkiVenafiResource returns a new instance of PkiVenafiResource.
func NewPkiVenafiResource() resource.Resource {
	return &PkiVenafiResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PkiVenafiResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_venafi"
}

// IdentitySchema defines the identifier used for import.
func (r *PkiVenafiResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Venafi CA ID used to uniquely reference the certificate authority.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Venafi CA resource.
func (r *PkiVenafiResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro Venafi certificate authority (Settings → Global → PKI certificates → Certificate Authorities).\n\n" +
			"**Preview feature:** Jamf Pro's Venafi integration is a preview and may change in a future Jamf Pro release.\n\n" +
			"### Refresh token\n\n" +
			"`refresh_token_wo` is `WriteOnly`: sent to Jamf Pro on writes, never stored in Terraform state, and never returned on read. Pair it with `refresh_token_wo_version` to rotate: bump the integer to re-send the current token; leave it unchanged to preserve the stored token. `refresh_token_configured` reports whether Jamf Pro currently holds a token.\n\n" +
			"### Jamf public key\n\n" +
			"`jamf_public_key` is the PEM public key Jamf Pro mints for this CA. It is read-only, populated on create and on every read. Bump `jamf_public_key_rotation` to regenerate it; the old key is invalidated.\n\n" +
			"### Proxy trust store\n\n" +
			"`proxy_trust_store` is the PKI proxy server's public PEM. It round-trips byte-for-byte through Jamf Pro, so it is a plain managed value rather than a write-only one. Set it to upload; set it to `\"\"` to remove it." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Venafi CA ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. The Venafi CA name. Required.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"proxy_address": schema.StringAttribute{
				MarkdownDescription: "**\"Proxy Server\"** in the Jamf Pro admin UI. The Jamf Pro PKI Proxy Server address as `host:port`, with no scheme (e.g. `proxy.example.com:8443`). Jamf Pro rejects a `https://` prefix. Omit to preserve the stored value. Jamf Pro also rejects an empty value (`\"\"`) with \"HTTP Host must not be empty\", so this cannot be cleared once set.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "**\"Client ID\"** in the Jamf Pro admin UI. The Venafi OAuth client identifier. Omit to preserve the stored value; set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"revocation_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable certificate revocation\"** in the Jamf Pro admin UI. Whether Jamf Pro may revoke certificates issued by this CA. Omit to preserve the stored value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"refresh_token_wo": schema.StringAttribute{
				MarkdownDescription: "**\"Refresh Token\"** in the Jamf Pro admin UI. The Venafi refresh token. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**. Jamf Pro never returns it on read, so rotation runs through the companion `refresh_token_wo_version` integer. Bump that to re-send `refresh_token_wo`.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"refresh_token_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `refresh_token_wo`. Bump this integer (any change) to force a new write that re-sends `refresh_token_wo` to Jamf Pro. Set it on create alongside `refresh_token_wo` (e.g. `1`). Leaving it unset or unchanged signals \"leave the stored token alone\": the provider omits the token from the next write, so Jamf Pro retains the existing value.",
				Optional:            true,
			},
			"refresh_token_configured": schema.BoolAttribute{
				MarkdownDescription: "Reported by Jamf Pro; whether a Venafi refresh token is currently stored for this CA.",
				Computed:            true,
			},
			"jamf_public_key": schema.StringAttribute{
				MarkdownDescription: "The PEM public key Jamf Pro mints for this CA. Read-only; populated on create and every read. Bump `jamf_public_key_rotation` to regenerate it.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"jamf_public_key_rotation": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for `jamf_public_key`. Bump this integer (any change) to regenerate the Jamf-minted public key; Jamf Pro invalidates the previous key. Leaving it unset or unchanged leaves the key alone.",
				Optional:            true,
			},
			"proxy_trust_store": schema.StringAttribute{
				MarkdownDescription: "The PKI Proxy Server's **public** PEM certificate chain, which secures communication between Jamf Pro and the proxy. It is not a secret; it round-trips byte-for-byte through Jamf Pro. Set to upload; set to `\"\"` to remove it. Omit to preserve the stored value.",
				Optional:            true,
				Computed:            true,
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
func (r *PkiVenafiResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_venafi")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ModifyPlan predicts the planned value of the server-minted jamf_public_key:
// when jamf_public_key_rotation changes versus prior state the key will be
// regenerated, so plan it Unknown to avoid a "provider produced inconsistent
// result after apply" error. Otherwise the attribute-level UseStateForUnknown
// keeps the stored key.
func (r *PkiVenafiResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// No prediction needed on create (no prior state) or destroy (null plan).
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan PkiVenafiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state PkiVenafiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if shouldRotate(plan.JamfPublicKeyRotation, state.JamfPublicKeyRotation) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("jamf_public_key"), types.StringUnknown())...)
	}
}

// ImportState handles import by the Jamf Pro Venafi CA ID.
func (r *PkiVenafiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
