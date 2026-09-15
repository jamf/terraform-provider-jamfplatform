// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_check_in_settings

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

// ComputerCheckInSettingsDataSource implements the Terraform data source for Jamf Pro Client
// Check-In settings.
type ComputerCheckInSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &ComputerCheckInSettingsDataSource{}

// NewComputerCheckInSettingsDataSource returns a new instance of ComputerCheckInSettingsDataSource.
func NewComputerCheckInSettingsDataSource() datasource.DataSource {
	return &ComputerCheckInSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ComputerCheckInSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_check_in_settings"
}

// Schema returns the data source schema.
func (d *ComputerCheckInSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Client Check-In settings. One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"check_in_frequency": schema.Int64Attribute{
				MarkdownDescription: "Recurring Check-in Frequency, in minutes.",
				Computed:            true,
			},
			"create_startup_script": schema.BoolAttribute{
				MarkdownDescription: "Create a startup script.",
				Computed:            true,
			},
			"startup_log": schema.BoolAttribute{
				MarkdownDescription: "Log Computer Usage Information at startup.",
				Computed:            true,
			},
			"startup_policies": schema.BoolAttribute{
				MarkdownDescription: "Check for policies triggered by startup.",
				Computed:            true,
			},
			"startup_ssh": schema.BoolAttribute{
				MarkdownDescription: "Ensure SSH is enabled.",
				Computed:            true,
			},
			"create_login_hook": schema.BoolAttribute{
				MarkdownDescription: "Create a login event.",
				Computed:            true,
			},
			"login_hook_log": schema.BoolAttribute{
				MarkdownDescription: "Log Computer Usage Information at login.",
				Computed:            true,
			},
			"login_hook_policies": schema.BoolAttribute{
				MarkdownDescription: "Check for policies triggered by login.",
				Computed:            true,
			},
			"allow_network_state_change_triggers": schema.BoolAttribute{
				MarkdownDescription: "Allow Network State Change Triggers.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *ComputerCheckInSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_check_in_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current Client Check-In settings and populates Terraform state.
func (d *ComputerCheckInSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ComputerCheckInSettingsDataSourceModel
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

	got, err := d.client.GetCheckInSettingsV3(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Client Check-In settings", helpers.APIErrorDetail(err))
		return
	}
	assignComputerCheckInSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Client Check-In settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
