// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package certificate_authority implements the jamfplatform_pro_pki_certificate_authority
// data source backed by the Jamf Pro PKI Certificate Authority API
// (Settings > Global > PKI certificates > Certificate Authorities). It is read-only: the
// API exposes only GET + DER/PEM downloads, so there is no resource counterpart.
package pki_certificate_authority

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required. Empty: the PKI
// certificate-authority read endpoints sit at the provider's overall floor. The
// provider-level advisory still fires through providerdata.ConfigurePro.
const minJamfProVersion = ""

const defaultReadTimeout = 60 * time.Second

// CertificateAuthorityDataSource implements the read-only data source.
type CertificateAuthorityDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &CertificateAuthorityDataSource{}

// NewCertificateAuthorityDataSource returns a new instance of the data source.
func NewCertificateAuthorityDataSource() datasource.DataSource {
	return &CertificateAuthorityDataSource{}
}

// Metadata sets the data source type name.
func (d *CertificateAuthorityDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_certificate_authority"
}

// Schema returns the data source schema.
func (d *CertificateAuthorityDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads X.509 details of a Jamf Pro Certificate Authority (Settings > Global > PKI certificates > Certificate Authorities). " +
			"Omit `id` to read the **active** CA (the Jamf Pro built-in CA on most tenants); set `id` to read a specific CA. " +
			"Read-only. Jamf Pro exposes only read and certificate-download operations for Certificate Authorities, so there is no resource counterpart. The certificate is surfaced as a Computed `pem`." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The Certificate Authority id to read. Omit to read the **active** CA; `id` is then set to `active` in state.",
				Optional:            true,
				Computed:            true,
			},
			"subject_x500_principal": schema.StringAttribute{
				MarkdownDescription: "The certificate subject distinguished name (X.500 principal).",
				Computed:            true,
			},
			"issuer_x500_principal": schema.StringAttribute{
				MarkdownDescription: "The certificate issuer distinguished name (X.500 principal).",
				Computed:            true,
			},
			"serial_number": schema.StringAttribute{
				MarkdownDescription: "The certificate serial number (hex).",
				Computed:            true,
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "The X.509 certificate version.",
				Computed:            true,
			},
			"not_after": schema.Int64Attribute{
				MarkdownDescription: "Certificate expiry, in epoch seconds.",
				Computed:            true,
			},
			"not_before": schema.Int64Attribute{
				MarkdownDescription: "Certificate validity start, in epoch seconds.",
				Computed:            true,
			},
			"key_usage": schema.ListAttribute{
				MarkdownDescription: "The certificate key-usage extensions (e.g. `digitalSignature`, `keyCertSign`).",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"key_usage_extended": schema.ListAttribute{
				MarkdownDescription: "The certificate extended-key-usage OIDs.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"sha1_fingerprint": schema.StringAttribute{
				MarkdownDescription: "The SHA-1 fingerprint of the certificate.",
				Computed:            true,
			},
			"sha256_fingerprint": schema.StringAttribute{
				MarkdownDescription: "The SHA-256 fingerprint of the certificate.",
				Computed:            true,
			},
			"signature_algorithm": schema.StringAttribute{
				MarkdownDescription: "The certificate signature algorithm (e.g. `SHA256withRSA`).",
				Computed:            true,
			},
			"signature_algorithm_oid": schema.StringAttribute{
				MarkdownDescription: "The certificate signature algorithm OID.",
				Computed:            true,
			},
			"signature_value": schema.StringAttribute{
				MarkdownDescription: "The certificate signature value (hex).",
				Computed:            true,
			},
			"pem": schema.StringAttribute{
				MarkdownDescription: "The Certificate Authority certificate in PEM format.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *CertificateAuthorityDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_certificate_authority")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the active CA (when `id` is omitted) or a CA by id, plus its PEM blob.
func (d *CertificateAuthorityDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data CertificateAuthorityDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	byID := !data.ID.IsNull() && !data.ID.IsUnknown()

	var (
		record *pro.CertificateRecord
		pem    []byte
		err    error
	)
	if byID {
		id := data.ID.ValueString()
		record, err = d.client.GetCertificateAuthorityV1(readCtx, id)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read Jamf Pro Certificate Authority", helpers.APIErrorDetail(err))
			return
		}
		pem, err = d.client.DownloadCertificateAuthorityPemV1(readCtx, id)
	} else {
		record, err = d.client.GetActiveCertificateAuthorityV1(readCtx)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read active Jamf Pro Certificate Authority", helpers.APIErrorDetail(err))
			return
		}
		pem, err = d.client.DownloadActiveCertificateAuthorityPemV1(readCtx)
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to download Jamf Pro Certificate Authority PEM", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignCertificateAuthorityDataSourceModel(ctx, &data, record, pem)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !byID {
		data.ID = types.StringValue("active")
	}

	tflog.Trace(ctx, "read Jamf Pro PKI Certificate Authority data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
