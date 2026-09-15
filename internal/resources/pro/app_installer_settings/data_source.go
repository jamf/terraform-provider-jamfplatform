// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_settings

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

// AppInstallerSettingsDataSource implements the Terraform data source for Jamf Pro
// App Installer global settings.
type AppInstallerSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AppInstallerSettingsDataSource{}

// NewAppInstallerSettingsDataSource returns a new instance of AppInstallerSettingsDataSource.
func NewAppInstallerSettingsDataSource() datasource.DataSource {
	return &AppInstallerSettingsDataSource{}
}

// Metadata sets the data source type name.
func (d *AppInstallerSettingsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_installer_settings"
}

// Schema returns the data source schema. All attributes are Computed.
func (d *AppInstallerSettingsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro App Installer global settings. These settings are one per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"deployment_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Deployment batch process controls. Null when not configured.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"batch_size": schema.Int64Attribute{
						MarkdownDescription: "Number of devices per command batch.",
						Computed:            true,
					},
					"batch_frequency": schema.Int64Attribute{
						MarkdownDescription: "Minutes between batch deployments.",
						Computed:            true,
					},
					"days": schema.SetAttribute{
						MarkdownDescription: "Days of the week on which deployments run.",
						Computed:            true,
						ElementType:         types.StringType,
					},
					"server_time_from": schema.StringAttribute{
						MarkdownDescription: "Start of the daily deployment window. Format: `HH:MM:SSZ`.",
						Computed:            true,
					},
					"server_time_to": schema.StringAttribute{
						MarkdownDescription: "End of the daily deployment window. Format: `HH:MM:SSZ`.",
						Computed:            true,
					},
				},
			},
			"end_user_experience": schema.SingleNestedAttribute{
				MarkdownDescription: "End-user notification and deferral settings. Null when not configured.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"notification_frequency": schema.Int64Attribute{
						MarkdownDescription: "Hours between repeat notifications.",
						Computed:            true,
					},
					"notification_message": schema.StringAttribute{
						MarkdownDescription: "Message shown when the notification first appears.",
						Computed:            true,
					},
					"update_deadline": schema.Int64Attribute{
						MarkdownDescription: "Hours the user may defer before the install is forced.",
						Computed:            true,
					},
					"force_quit_message": schema.StringAttribute{
						MarkdownDescription: "Message shown when the user is prompted to quit the app.",
						Computed:            true,
					},
					"force_quit_grace_period": schema.Int64Attribute{
						MarkdownDescription: "Minutes the user is given to quit the app before the install proceeds.",
						Computed:            true,
					},
					"update_complete_message": schema.StringAttribute{
						MarkdownDescription: "Message shown when the installation completes.",
						Computed:            true,
					},
					"relaunch": schema.BoolAttribute{
						MarkdownDescription: "Whether the app is relaunched automatically after update.",
						Computed:            true,
					},
					"suppress": schema.BoolAttribute{
						MarkdownDescription: "Whether notifications are suppressed.",
						Computed:            true,
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *AppInstallerSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_installer_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current App Installer global settings and populates state.
func (d *AppInstallerSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AppInstallerSettingsDataSourceModel
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

	got, err := d.client.GetAppInstallerGlobalSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro App Installer global settings", helpers.APIErrorDetail(err))
		return
	}
	assignAppInstallerSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro App Installer settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
