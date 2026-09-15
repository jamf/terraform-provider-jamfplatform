// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_adcs

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

// AdcsDataSource implements the Terraform data source for a Jamf Pro AD CS
// integration, read by id. The WriteOnly certificate bytes/password are never
// returned by the API and are not exposed here; only the server-readable scalars,
// the certificate metadata blocks, and the connector last-check-in are surfaced.
type AdcsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AdcsDataSource{}

// NewAdcsDataSource returns a new instance of the data source.
func NewAdcsDataSource() datasource.DataSource {
	return &AdcsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AdcsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_adcs"
}

// dsCertDetailsAttributes returns the Computed *_details nested attribute schema
// for the data source.
func dsCertDetailsAttributes(which string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"filename": schema.StringAttribute{
			MarkdownDescription: "Filename Jamf Pro recorded for the " + which + " certificate.",
			Computed:            true,
		},
		"serial_number": schema.StringAttribute{
			MarkdownDescription: "Serial number of the " + which + " certificate.",
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
			MarkdownDescription: "Expiry date of the " + which + " certificate.",
			Computed:            true,
		},
	}
}

// Schema returns the data source schema.
func (d *AdcsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read a Jamf Pro AD CS (Active Directory Certificate Services) integration by ID (Settings > Global > PKI certificates > Certificate Authorities). The certificate bytes and password are never returned by Jamf Pro and are not exposed here." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "AD CS Settings ID assigned by Jamf Pro.",
				Required:            true,
			},
			"connector_mode": schema.StringAttribute{
				MarkdownDescription: "AD CS connector mode. Either `INBOUND` or `OUTBOUND`.",
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Label for the integration.",
				Computed:            true,
			},
			"ca_name": schema.StringAttribute{
				MarkdownDescription: "The Certificate Authority name.",
				Computed:            true,
			},
			"fqdn": schema.StringAttribute{
				MarkdownDescription: "Fully-qualified domain name of the AD CS server.",
				Computed:            true,
			},
			"revocation_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether certificate revocation is enabled.",
				Computed:            true,
			},
			"adcs_url": schema.StringAttribute{
				MarkdownDescription: "The AD CS Connector URL (INBOUND).",
				Computed:            true,
			},
			"api_client_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the Jamf Pro API client (OUTBOUND); `null` for INBOUND.",
				Computed:            true,
			},
			"server_certificate_details": schema.SingleNestedAttribute{
				MarkdownDescription: "Metadata Jamf Pro parsed from the server certificate; `null` when none is configured.",
				Computed:            true,
				Attributes:          dsCertDetailsAttributes("server"),
			},
			"client_certificate_details": schema.SingleNestedAttribute{
				MarkdownDescription: "Metadata Jamf Pro parsed from the client certificate; `null` when none is configured.",
				Computed:            true,
				Attributes:          dsCertDetailsAttributes("client"),
			},
			"connector_last_check_in": schema.StringAttribute{
				MarkdownDescription: "Timestamp (RFC 3339) of the AD CS Connector's last check-in; `null` if it has never checked in.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *AdcsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_adcs")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the AD CS integration by id and populates Terraform state.
func (d *AdcsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AdcsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro AD CS integration without an 'id'.")
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := d.client.GetAdcsSettingsV1(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro AD CS integration", helpers.APIErrorDetail(err))
		return
	}
	assignAdcsDataSourceModel(&data, got)
	data.ID = types.StringValue(got.ID)

	tflog.Trace(ctx, "read Jamf Pro AD CS integration data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
