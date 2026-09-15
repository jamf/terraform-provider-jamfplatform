// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package adcs implements the jamfplatform_pro_pki_adcs resource and data source
// backed by the Jamf Pro AD CS Settings API (Settings > Global > PKI certificates
// > Certificate Authorities; the Jamf Pro admin UI calls this the AD CS — Active
// Directory Certificate Services — connector). An AD CS integration runs in one of
// two modes, selected by connector_mode:
//
//   - INBOUND: Jamf Pro reaches an AD CS Connector over a server URL, presenting a
//     client certificate and trusting a server certificate.
//   - OUTBOUND: an AD CS Connector polls Jamf Pro using a Jamf Pro API client
//     (referenced by api_client_id) that holds the AD CS certificate-job roles.
//
// The mode is immutable once set (RequiresReplace) — Jamf Pro rejects PATCH-ing a
// mode flip with HTTP 400.
package pki_adcs

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource.
//
// TODO(maintainer): left empty pending a release-notes check. The AD CS OUTBOUND
// connector mode may carry a higher floor than the inbound integration; pin this
// if the OUTBOUND endpoint shipped after the provider's overall floor (11.0.0).
const minJamfProVersion = ""

// AdcsResource implements the Terraform resource for a Jamf Pro AD CS integration.
type AdcsResource struct {
	client *pro.Client
}

var _ resource.Resource = &AdcsResource{}
var _ resource.ResourceWithImportState = &AdcsResource{}
var _ resource.ResourceWithIdentity = &AdcsResource{}
var _ resource.ResourceWithConfigValidators = &AdcsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAdcsResource returns a new instance of AdcsResource.
func NewAdcsResource() resource.Resource {
	return &AdcsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AdcsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_adcs"
}

// IdentitySchema defines the identifier used for import.
func (r *AdcsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro AD CS Settings ID used to uniquely reference the integration.",
				RequiredForImport: true,
			},
		},
	}
}

// certDetailsAttributes returns the Computed *_details nested attribute schema
// shared by server_certificate_details and client_certificate_details.
func certDetailsAttributes(which string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"filename": schema.StringAttribute{
			MarkdownDescription: "Filename Jamf Pro recorded for the uploaded " + which + " certificate.",
			Computed:            true,
		},
		"serial_number": schema.StringAttribute{
			MarkdownDescription: "Serial number of the " + which + " certificate, as parsed by Jamf Pro.",
			Computed:            true,
		},
		"subject": schema.StringAttribute{
			MarkdownDescription: "Subject distinguished name of the " + which + " certificate.",
			Computed:            true,
		},
		"issuer": schema.StringAttribute{
			MarkdownDescription: "Issuer distinguished name of the " + which + " certificate.",
			Computed:            true,
		},
		"expiration_date": schema.StringAttribute{
			MarkdownDescription: "Expiry date of the " + which + " certificate, as reported by Jamf Pro.",
			Computed:            true,
		},
	}
}

// Schema returns the Terraform schema for the AD CS resource.
func (r *AdcsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro AD CS (Active Directory Certificate Services) integration (Settings > Global > PKI certificates > Certificate Authorities). " +
			"An AD CS connector issues certificates to managed devices. The integration runs in one of two modes selected by `connector_mode`:\n\n" +
			"- **`INBOUND`**: Jamf Pro reaches an AD CS Connector at `adcs_url`, presenting a `client_certificate` and trusting a `server_certificate`.\n" +
			"- **`OUTBOUND`**: an AD CS Connector polls Jamf Pro using a Jamf Pro API client (referenced by `api_client_id`) that is permitted to read and update AD CS certificate jobs. Jamf Pro called that pair *Read AD CS Certificate Jobs* and *Update AD CS Certificate Jobs* before the Platform API GA; this provider has no Jamf Account permission recorded for it, so grant it from the Jamf Pro AD CS documentation rather than from the table below.\n\n" +
			"`connector_mode` is immutable. Changing it forces resource replacement, because Jamf Pro rejects an in-place mode flip.\n\n" +
			"Certificate bytes (`data_wo`) and the client certificate `password_wo` are `WriteOnly`: sent to Jamf Pro on writes, but never persisted in Terraform state and never returned on read. Bump a block's `wo_version` to re-send that certificate; Jamf Pro accepts a certificate in full or not at all. On update, an omitted certificate is left unchanged.\n\n" +
			"The `connector_mode` cross-field validator sees only what your configuration *declares*. Because Jamf Pro preserves an omitted optional field, a value left over from a previous apply is not re-validated: the validator catches a both-declared conflict, not a preserved one.\n\n" +
			"Import with `terraform import jamfplatform_pro_pki_adcs.<name> <id>`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "AD CS Settings ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"connector_mode": schema.StringAttribute{
				MarkdownDescription: "AD CS connector mode. One of `INBOUND` or `OUTBOUND`. Immutable: changing it forces resource replacement. `INBOUND` requires `adcs_url`, `server_certificate` and `client_certificate`, and forbids `api_client_id`. `OUTBOUND` requires `api_client_id`, and forbids `adcs_url`, `server_certificate` and `client_certificate`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(connectorModeInbound, connectorModeOutbound),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. A label for the integration. Jamf Pro requires it on create in both modes.",
				Required:            true,
			},
			"ca_name": schema.StringAttribute{
				MarkdownDescription: "**\"CA Name\"** in the Jamf Pro admin UI. The Certificate Authority name. Jamf Pro requires it on create in both modes.",
				Required:            true,
			},
			"fqdn": schema.StringAttribute{
				MarkdownDescription: "**\"FQDN\"** in the Jamf Pro admin UI. The fully-qualified domain name of the AD CS server. Jamf Pro requires it on create in both modes.",
				Required:            true,
			},
			"revocation_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable Revocation\"** in the Jamf Pro admin UI. Whether certificate revocation is enabled. Optional; omit to preserve the current value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"adcs_url": schema.StringAttribute{
				MarkdownDescription: "**\"AD CS Connector URL\"** in the Jamf Pro admin UI. The AD CS Connector address, without a scheme (e.g. `connector.example.com`). **`INBOUND` only.** Optional; omit to preserve the current value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"api_client_id": schema.StringAttribute{
				MarkdownDescription: "**\"API Client ID\"** in the Jamf Pro admin UI. The UUID (`client_id`) of an existing Jamf Pro API client the AD CS Connector authenticates as when it polls for certificate jobs. That client must be permitted to read and update those jobs: the privileges Jamf Pro called *Read AD CS Certificate Jobs* and *Update AD CS Certificate Jobs* before the Platform API GA, which this provider has no Jamf Account permission recorded for. **`OUTBOUND` only.** Optional; omit to preserve the current value. API clients are created in Jamf Account, not by this provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_certificate": schema.SingleNestedAttribute{
				MarkdownDescription: "**`INBOUND` only.** The server (trust) certificate Jamf Pro trusts for the AD CS Connector (`.pem` or `.cer`). It is public, but Jamf Pro never returns the bytes on read, so it is modelled `WriteOnly`: the bytes stay out of state, and you rotate them by bumping `wo_version`. Required when `connector_mode = \"INBOUND\"`; forbidden for `OUTBOUND`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"data_wo": schema.StringAttribute{
						MarkdownDescription: "The base64-encoded server certificate (`.pem` or `.cer`). Supply it with `filebase64(\"server.pem\")`. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned on read. Sent when `wo_version` changes.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
						Validators: []validator.String{
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("wo_version")),
						},
					},
					"filename": schema.StringAttribute{
						MarkdownDescription: "Filename for the uploaded server certificate (e.g. `server.pem`). Required when the `server_certificate` block is set, because Jamf Pro reads the certificate format from the extension.",
						Required:            true,
					},
					"wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `data_wo`. Bump this integer (any change) to re-send the server certificate on the next apply; leaving it unset or unchanged means \"leave the stored certificate alone\". Set it on create when you supply `data_wo`.",
						Optional:            true,
					},
				},
			},
			"client_certificate": schema.SingleNestedAttribute{
				MarkdownDescription: "**`INBOUND` only.** The confidential client certificate Jamf Pro presents to the AD CS Connector (`.pfx` or `.p12`). Required when `connector_mode = \"INBOUND\"`; forbidden for `OUTBOUND`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"data_wo": schema.StringAttribute{
						MarkdownDescription: "The base64-encoded client certificate keystore (`.pfx` or `.p12`). Supply it with `filebase64(\"client.p12\")`. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned on read. Sent when `wo_version` changes.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
						Validators: []validator.String{
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("wo_version")),
						},
					},
					"password_wo": schema.StringAttribute{
						MarkdownDescription: "The password protecting the client certificate keystore. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned on read. Sent together with `data_wo` when `wo_version` changes.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"filename": schema.StringAttribute{
						MarkdownDescription: "Filename for the uploaded client certificate (e.g. `client.p12`). Required when the `client_certificate` block is set, because Jamf Pro reads the certificate format from the extension.",
						Required:            true,
					},
					"wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `data_wo` (and `password_wo`). Bump this integer (any change) to re-send the client certificate on the next apply; leaving it unset or unchanged means \"leave the stored certificate alone\". Set it on create when you supply `data_wo`.",
						Optional:            true,
					},
				},
			},
			"server_certificate_details": schema.SingleNestedAttribute{
				MarkdownDescription: "Read-only metadata Jamf Pro parsed from the uploaded server certificate. `null` when no server certificate is configured (e.g. `OUTBOUND` mode).",
				Computed:            true,
				Attributes:          certDetailsAttributes("server"),
			},
			"client_certificate_details": schema.SingleNestedAttribute{
				MarkdownDescription: "Read-only metadata Jamf Pro parsed from the uploaded client certificate. `null` when no client certificate is configured (e.g. `OUTBOUND` mode).",
				Computed:            true,
				Attributes:          certDetailsAttributes("client"),
			},
			"connector_last_check_in": schema.StringAttribute{
				MarkdownDescription: "Read-only timestamp (RFC 3339) of the AD CS Connector's last check-in with Jamf Pro. `null` if the connector has never checked in.",
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

// ConfigValidators returns the cross-field validators evaluated at plan time.
func (r *AdcsResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		connectorModeConfigValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *AdcsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_adcs")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro AD CS Settings ID.
func (r *AdcsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
