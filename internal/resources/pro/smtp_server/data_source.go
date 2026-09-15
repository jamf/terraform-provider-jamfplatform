// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smtp_server

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

// SmtpServerDataSource implements the read-only data source for Jamf Pro SMTP
// Server settings.
type SmtpServerDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SmtpServerDataSource{}

// NewSmtpServerDataSource returns a new instance of the data source.
func NewSmtpServerDataSource() datasource.DataSource {
	return &SmtpServerDataSource{}
}

// Metadata sets the data source type name.
func (d *SmtpServerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smtp_server"
}

// Schema returns the data source schema. Secrets and rotation triggers are
// omitted (write-only, never readable).
func (d *SmtpServerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro SMTP Server settings (Settings → System → SMTP Server). One record per tenant. `WriteOnly` secrets are never returned." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the SMTP server connection is enabled.",
				Computed:            true,
			},
			"authentication_type": schema.StringAttribute{
				MarkdownDescription: "Authentication method: `NONE`, `BASIC`, `GRAPH_API`, or `GOOGLE_MAIL`.",
				Computed:            true,
			},
			"sender_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Sender identity applied to outbound mail.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"email_address": schema.StringAttribute{Computed: true, MarkdownDescription: "Sender email address."},
					"display_name":  schema.StringAttribute{Computed: true, MarkdownDescription: "Sender display name."},
				},
			},
			"connection_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "SMTP relay connection (present for NONE / BASIC).",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"host":               schema.StringAttribute{Computed: true, MarkdownDescription: "SMTP server hostname or IP."},
					"port":               schema.Int64Attribute{Computed: true, MarkdownDescription: "SMTP server port."},
					"encryption_type":    schema.StringAttribute{Computed: true, MarkdownDescription: "Connection encryption protocol."},
					"connection_timeout": schema.Int64Attribute{Computed: true, MarkdownDescription: "Connection timeout, in seconds."},
				},
			},
			"basic_auth_credentials": schema.SingleNestedAttribute{
				MarkdownDescription: "Basic credentials (present for BASIC). Only the non-secret username is readable.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"username": schema.StringAttribute{Computed: true, MarkdownDescription: "SMTP account username."},
				},
			},
			"graph_api_credentials": schema.SingleNestedAttribute{
				MarkdownDescription: "Microsoft Graph API credentials (present for GRAPH_API). The client secret is never readable.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"tenant_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Microsoft Entra tenant (directory) ID."},
					"client_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Microsoft Entra application (client) ID."},
				},
			},
			"google_mail_credentials": schema.SingleNestedAttribute{
				MarkdownDescription: "Google Workspace credentials (present for GOOGLE_MAIL). The client secret is never readable.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"client_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Google OAuth client ID."},
					"authentications": schema.ListNestedAttribute{
						MarkdownDescription: "Granted Google sender accounts and their OAuth status.",
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"email_address": schema.StringAttribute{Computed: true, MarkdownDescription: "Granted Google sender email address."},
								"status":        schema.StringAttribute{Computed: true, MarkdownDescription: "OAuth grant status."},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *SmtpServerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smtp_server")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current SMTP Server settings and populates Terraform state.
func (d *SmtpServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SmtpServerDataSourceModel
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

	got, err := d.client.GetSmtpServerV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro SMTP Server settings", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignSmtpServerDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro SMTP Server settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
