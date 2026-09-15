// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact_alert_notification_settings

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

// ImpactAlertNotificationSettingsDataSource implements the Terraform data source for Jamf
// Pro Impact Alert Notification settings.
type ImpactAlertNotificationSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &ImpactAlertNotificationSettingsDataSource{}

// NewImpactAlertNotificationSettingsDataSource returns a new instance of the data source.
func NewImpactAlertNotificationSettingsDataSource() datasource.DataSource {
	return &ImpactAlertNotificationSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ImpactAlertNotificationSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_impact_alert_notification_settings"
}

// Schema returns the data source schema.
func (d *ImpactAlertNotificationSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Impact Alert Notification settings (Settings > System > Impact alert notifications). One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"deployable_objects_alert_enabled": schema.BoolAttribute{
				MarkdownDescription: "Display deployment impact alert on Save for deployable objects (policies, configuration profiles, apps, managed software updates).",
				Computed:            true,
			},
			"deployable_objects_confirmation_code_enabled": schema.BoolAttribute{
				MarkdownDescription: "Require Jamf Pro users to type a confirmation code (COMMIT) to acknowledge edits to deployable objects before saving.",
				Computed:            true,
			},
			"scopeable_objects_alert_enabled": schema.BoolAttribute{
				MarkdownDescription: "Display criteria impact alert on Save for scopeable object edits (smart groups, static groups, classes).",
				Computed:            true,
			},
			"scopeable_objects_confirmation_code_enabled": schema.BoolAttribute{
				MarkdownDescription: "Require Jamf Pro users to type a confirmation code (COMMIT) to acknowledge edits to scopeable objects before saving.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *ImpactAlertNotificationSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_impact_alert_notification_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current Impact Alert Notification settings and populates Terraform state.
func (d *ImpactAlertNotificationSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ImpactAlertNotificationSettingsDataSourceModel
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

	got, err := d.client.GetImpactAlertNotificationSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Impact Alert Notification settings", helpers.APIErrorDetail(err))
		return
	}
	assignImpactAlertNotificationSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Impact Alert Notification settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
