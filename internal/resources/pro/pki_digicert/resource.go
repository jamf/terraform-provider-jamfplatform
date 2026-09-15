// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package digicert implements the jamfplatform_pro_pki_digicert resource and
// data source backed by the Jamf Pro DigiCert Trust Lifecycle Manager API
// (Settings → Global → PKI certificates → Certificate Authorities). DigiCert TLM
// is one of the external certificate-authority integrations Jamf Pro can manage.
package pki_digicert

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty string skips the version check; the provider-level advisory
// still fires through providerdata.ConfigurePro.
const minJamfProVersion = ""

// DigicertResource implements the Terraform resource for a DigiCert Trust
// Lifecycle Manager integration.
type DigicertResource struct {
	client *pro.Client
}

var _ resource.Resource = &DigicertResource{}
var _ resource.ResourceWithImportState = &DigicertResource{}
var _ resource.ResourceWithIdentity = &DigicertResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDigicertResource returns a new instance of DigicertResource.
func NewDigicertResource() resource.Resource {
	return &DigicertResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DigicertResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_digicert"
}

// IdentitySchema defines the identifier used for import.
func (r *DigicertResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro DigiCert Trust Lifecycle Manager integration ID used to uniquely reference the integration.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the DigiCert integration resource.
func (r *DigicertResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro DigiCert Trust Lifecycle Manager integration (Settings → Global → PKI certificates → Certificate Authorities). " +
			"DigiCert TLM is an external certificate authority Jamf Pro uses to issue certificates referenced by configuration profiles.\n\n" +
			"### Client certificate\n\n" +
			"The certificate, a `.p12`/`.pfx` keystore, authenticates Jamf Pro to DigiCert One. Supply it through the `client_certificate` block as `data_wo` (base64 of the keystore: use `filebase64(\"cert.p12\")`) plus `password_wo`. Both are `WriteOnly`, never persisted in Terraform state, and never returned by Jamf Pro on read. " +
			"DigiCert treats the certificate as all-or-nothing, so the provider re-sends the whole certificate only when you bump `client_certificate.wo_version`. Editing `data_wo`/`password_wo` without bumping the version is deliberately a no-op. " +
			"Certificate metadata Jamf Pro parses from the uploaded keystore (serial, subject, issuer, expiry, filename) is surfaced in the read-only `client_certificate_details` block.\n\n" +
			"Import with `terraform import jamfplatform_pro_pki_digicert.<name> <id>`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Integration ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name for the Integration\"** in the Jamf Pro admin UI. A friendly name for this DigiCert integration. Jamf Pro requires it on create.",
				Required:            true,
			},
			"host_name": schema.StringAttribute{
				MarkdownDescription: "**\"DigiCert One Host Name\"** in the Jamf Pro admin UI. The DigiCert One host (e.g. `one.digicert.com`). Jamf Pro requires it on create.",
				Required:            true,
			},
			"revocation_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro requests certificate revocation through DigiCert. Omit to preserve the current value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"client_certificate": schema.SingleNestedAttribute{
				MarkdownDescription: "The client certificate (`.p12`/`.pfx` keystore) Jamf Pro uses to authenticate to DigiCert One. Omit the whole block to leave the stored certificate untouched. DigiCert treats the certificate as all-or-nothing; the provider re-sends it only on create (when the block is set) or when `wo_version` changes.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"data_wo": schema.StringAttribute{
						MarkdownDescription: "The base64-encoded certificate keystore (`.p12`/`.pfx`). Supply with `filebase64(\"cert.p12\")`. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned on read. Pair with `wo_version` to rotate.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"password_wo": schema.StringAttribute{
						MarkdownDescription: "The password protecting the certificate keystore. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**. Pair with `wo_version` to rotate.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"filename": schema.StringAttribute{
						MarkdownDescription: "The certificate keystore filename (e.g. `client.p12`). Required when the `client_certificate` block is set. Jamf Pro reads the certificate format from the extension (`.p12`/`.pfx`) and rejects an upload without it.",
						Required:            true,
					},
					"wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` certificate fields. Bump this integer (any change) to force the next update to re-send the full certificate (`data_wo` + `password_wo` + `filename`) to Jamf Pro. Initial create should set `wo_version = 1`. Leaving it unset or unchanged signals \"leave the stored certificate alone\": the provider omits the certificate from the next update, so Jamf Pro retains the existing value. Editing `data_wo`/`password_wo` without bumping `wo_version` is deliberately a no-op.",
						Optional:            true,
					},
				},
			},
			"client_certificate_details": schema.SingleNestedAttribute{
				MarkdownDescription: "Read-only metadata Jamf Pro derives from the uploaded client certificate. Populated after a certificate is stored; the certificate bytes and password are never returned.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"filename": schema.StringAttribute{
						MarkdownDescription: "The stored certificate filename.",
						Computed:            true,
					},
					"serial_number": schema.StringAttribute{
						MarkdownDescription: "The certificate serial number.",
						Computed:            true,
					},
					"subject": schema.StringAttribute{
						MarkdownDescription: "The certificate subject distinguished name.",
						Computed:            true,
					},
					"issuer": schema.StringAttribute{
						MarkdownDescription: "The certificate issuer distinguished name.",
						Computed:            true,
					},
					"expiration_date": schema.StringAttribute{
						MarkdownDescription: "The certificate expiry as an RFC 3339 timestamp.",
						Computed:            true,
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *DigicertResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_digicert")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro DigiCert integration ID.
func (r *DigicertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
