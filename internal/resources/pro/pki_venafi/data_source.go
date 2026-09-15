// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_venafi

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PkiVenafiDataSource implements the Terraform data source for Jamf Pro Venafi
// CAs. The refresh token is never exposed — Jamf Pro never returns it on read.
type PkiVenafiDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &PkiVenafiDataSource{}

// NewPkiVenafiDataSource returns a new instance of PkiVenafiDataSource.
func NewPkiVenafiDataSource() datasource.DataSource {
	return &PkiVenafiDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PkiVenafiDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_venafi"
}

// Schema returns the data source schema.
func (d *PkiVenafiDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Venafi certificate authority by ID (Settings → Global → PKI certificates). The refresh token is never exposed; Jamf Pro does not return it on read. **Preview feature:** it may change in a future Jamf Pro release." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Venafi CA ID to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The Venafi CA name.",
				Computed:            true,
			},
			"proxy_address": schema.StringAttribute{
				MarkdownDescription: "The Jamf Pro PKI Proxy Server address (`host:port`).",
				Computed:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "The Venafi OAuth client identifier.",
				Computed:            true,
			},
			"revocation_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro may revoke certificates issued by this CA.",
				Computed:            true,
			},
			"refresh_token_configured": schema.BoolAttribute{
				MarkdownDescription: "Whether a Venafi refresh token is currently stored for this CA.",
				Computed:            true,
			},
			"jamf_public_key": schema.StringAttribute{
				MarkdownDescription: "The PEM public key Jamf Pro mints for this CA.",
				Computed:            true,
			},
			"proxy_trust_store": schema.StringAttribute{
				MarkdownDescription: "The PKI Proxy Server's public PEM certificate chain, if uploaded.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *PkiVenafiDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_venafi")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a Venafi CA by ID and populates Terraform state, including the
// jamf public key and proxy trust store (separate GETs).
func (d *PkiVenafiDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PkiVenafiDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "The id attribute must be provided to read a Jamf Pro Venafi CA.")
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	rec, err := d.client.GetVenafiV1(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro Venafi CA", helpers.APIErrorDetail(err))
		return
	}
	assignVenafiDataSourceModel(&data, rec)

	// jamf public key (byte-stable) — absent → null.
	if pem, keyErr := d.client.GetVenafiJamfPublicKeyV1(readCtx, data.ID.ValueString()); keyErr != nil {
		if !helpers.IsNotFoundError(keyErr) {
			resp.Diagnostics.AddError("Error reading Jamf Pro Venafi public key", helpers.APIErrorDetail(keyErr))
			return
		}
		data.JamfPublicKey = types.StringNull()
	} else if len(pem) == 0 {
		data.JamfPublicKey = types.StringNull()
	} else {
		data.JamfPublicKey = types.StringValue(string(pem))
	}

	// proxy trust store — 404 → null.
	if pem, tsErr := d.client.GetVenafiProxyTrustStoreV1(readCtx, data.ID.ValueString()); tsErr != nil {
		if !helpers.IsNotFoundError(tsErr) {
			resp.Diagnostics.AddError("Error reading Jamf Pro Venafi proxy trust store", helpers.APIErrorDetail(tsErr))
			return
		}
		data.ProxyTrustStore = types.StringNull()
	} else if len(pem) == 0 {
		data.ProxyTrustStore = types.StringNull()
	} else {
		data.ProxyTrustStore = types.StringValue(string(pem))
	}

	tflog.Trace(ctx, "read Jamf Pro Venafi CA data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
