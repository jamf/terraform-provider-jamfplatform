// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_digicert

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DigicertDataSource implements the Terraform data source for a DigiCert Trust
// Lifecycle Manager integration.
type DigicertDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &DigicertDataSource{}

// NewDigicertDataSource returns a new instance of DigicertDataSource.
func NewDigicertDataSource() datasource.DataSource {
	return &DigicertDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DigicertDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_digicert"
}

// Schema returns the data source schema. The certificate bytes and password are
// never exposed — Jamf Pro never returns them on read.
func (d *DigicertDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro DigiCert Trust Lifecycle Manager integration by ID. The certificate bytes and password are never exposed; Jamf Pro does not return them on read." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "DigiCert integration ID to look up.",
				Required:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name for the integration.",
				Computed:            true,
			},
			"host_name": schema.StringAttribute{
				MarkdownDescription: "DigiCert One host name.",
				Computed:            true,
			},
			"revocation_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether certificate revocation is enabled.",
				Computed:            true,
			},
			"client_certificate_details": schema.SingleNestedAttribute{
				MarkdownDescription: "Read-only metadata Jamf Pro derives from the stored client certificate.",
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
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *DigicertDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_digicert")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a DigiCert integration by ID and populates Terraform state.
func (d *DigicertDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DigicertDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "The id attribute must be provided to read a Jamf Pro DigiCert integration.")
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := d.client.GetDigicertTrustLifecycleManagerV1(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro DigiCert integration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDigicertDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro DigiCert integration data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
