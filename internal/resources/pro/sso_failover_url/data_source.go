// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_failover_url

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

// SsoFailoverURLDataSource implements the read-only mirror.
type SsoFailoverURLDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SsoFailoverURLDataSource{}

// NewSsoFailoverURLDataSource constructs a new SsoFailoverURLDataSource.
func NewSsoFailoverURLDataSource() datasource.DataSource {
	return &SsoFailoverURLDataSource{}
}

// Metadata sets the data source type name.
func (d *SsoFailoverURLDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_sso_failover_url"
}

// Schema returns the data source schema.
func (d *SsoFailoverURLDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the Jamf Pro SSO failover URL for the current tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"failover_url": schema.StringAttribute{
				MarkdownDescription: "Current failover URL. Treat as a credential.",
				Computed:            true,
				Sensitive:           true,
			},
			"generation_time": schema.Int64Attribute{
				MarkdownDescription: "Generation timestamp in Unix milliseconds.",
				Computed:            true,
			},
			"generation_time_utc": schema.StringAttribute{
				MarkdownDescription: "Generation timestamp formatted as RFC3339 UTC.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *SsoFailoverURLDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_sso_failover_url")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read populates state from /v1/sso/failover.
func (d *SsoFailoverURLDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SsoFailoverURLDataSourceModel
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

	got, err := d.client.GetSsoFailoverV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro SSO failover URL", helpers.APIErrorDetail(err))
		return
	}
	assignDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro SSO failover URL data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
