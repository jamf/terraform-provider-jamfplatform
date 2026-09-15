// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetJamfProServerURLV1
//
// Deliberately NOT adopted (this construct is read-only by design):
//   pro.UpdateJamfProServerURLV1 — the write path is out of scope; there is no
//   resource for this object. A data source never writes, so the documented
//   PUT-gateway behaviour is moot here.
//
// Status: current. Last reviewed 2026-06-13.

// Package jamf_pro_server_url implements the read-only
// jamfplatform_pro_jamf_pro_server_url data source, which surfaces the Jamf Pro
// server URL clients check in against (Settings > Jamf Pro Server URL). Singleton
// — one record per tenant. Read-only by design: there is no resource and no write
// path. Practitioners reference the exposed url elsewhere in their configuration
// (enrollment URLs, webhooks, scripts).
package jamf_pro_server_url

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

const defaultReadTimeout = 60 * time.Second

// minJamfProVersion is the minimum Jamf Pro tenant version required by this data
// source. Empty: no per-resource floor; the Jamf Pro Server URL endpoint predates
// the provider's overall floor (11.0.0).
const minJamfProVersion = ""

// JamfProServerURLDataSourceModel is the data source model for the Jamf Pro server URL.
type JamfProServerURLDataSourceModel struct {
	ID       types.String   `tfsdk:"id"`
	URL      types.String   `tfsdk:"url"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

// JamfProServerURLDataSource implements the Terraform data source for the Jamf Pro server URL.
type JamfProServerURLDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &JamfProServerURLDataSource{}

// NewJamfProServerURLDataSource returns a new instance of JamfProServerURLDataSource.
func NewJamfProServerURLDataSource() datasource.DataSource {
	return &JamfProServerURLDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *JamfProServerURLDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_pro_server_url"
}

// Schema returns the data source schema.
func (d *JamfProServerURLDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the Jamf Pro server URL (Settings > Jamf Pro Server URL), the URL devices check in against. One value per tenant. Read-only; reference the returned `url` elsewhere in your configuration, such as in enrollment URLs, webhooks, or scripts." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The Jamf Pro server URL that devices check in against.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *JamfProServerURLDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_pro_server_url")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the Jamf Pro server URL and populates state. The server URL is always
// set on a live tenant, so Read surfaces whatever the server returns; it errors only
// on a transport failure.
func (d *JamfProServerURLDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data JamfProServerURLDataSourceModel
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

	got, err := d.client.GetJamfProServerURLV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro server URL", helpers.APIErrorDetail(err))
		return
	}
	data.URL = types.StringValue(got.URL)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro server URL data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
