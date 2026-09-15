// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package gsx_connection

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

// GsxConnectionSettingsDataSource implements the Terraform data source for Jamf Pro GSX
// Connection settings. It exposes only the non-secret fields — the GSX API never returns
// the token or keystore secrets.
type GsxConnectionSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &GsxConnectionSettingsDataSource{}

// NewGsxConnectionSettingsDataSource returns a new instance of the data source.
func NewGsxConnectionSettingsDataSource() datasource.DataSource {
	return &GsxConnectionSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *GsxConnectionSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_gsx_connection_settings"
}

// Schema returns the data source schema.
func (d *GsxConnectionSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro GSX Connection settings (Settings > Global > GSX connection). One record per tenant. Jamf Pro never returns the token or keystore secrets, so they are not exposed here." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the GSX connection is enabled.",
				Computed:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The GSX account email.",
				Computed:            true,
			},
			"service_account_number": schema.StringAttribute{
				MarkdownDescription: "The GSX service account number.",
				Computed:            true,
			},
			"ship_to_number": schema.StringAttribute{
				MarkdownDescription: "The GSX ship-to number.",
				Computed:            true,
			},
			"keystore_name": schema.StringAttribute{
				MarkdownDescription: "The certificate keystore filename.",
				Computed:            true,
			},
			"keystore_error_message": schema.StringAttribute{
				MarkdownDescription: "Certificate validation error reported by Jamf Pro, if any.",
				Computed:            true,
			},
			"keystore_expiration_epoch": schema.Int64Attribute{
				MarkdownDescription: "Certificate expiry, in epoch milliseconds.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *GsxConnectionSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_gsx_connection_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current GSX Connection settings and populates Terraform state.
func (d *GsxConnectionSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data GsxConnectionSettingsDataSourceModel
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

	got, err := d.client.GetGSXConnectionV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro GSX Connection settings", helpers.APIErrorDetail(err))
		return
	}
	assignGsxConnectionSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro GSX Connection settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
