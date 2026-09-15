// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package login_page

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

// LoginPageSettingsDataSource implements the Terraform data source for the Jamf Pro
// login page settings.
type LoginPageSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &LoginPageSettingsDataSource{}

// NewLoginPageSettingsDataSource returns a new instance of the data source.
func NewLoginPageSettingsDataSource() datasource.DataSource {
	return &LoginPageSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *LoginPageSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_login_page_settings"
}

// Schema returns the data source schema.
func (d *LoginPageSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro login page disclaimer settings (Settings > System > Login page). One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"include_custom_disclaimer": schema.BoolAttribute{
				MarkdownDescription: "Whether the custom disclaimer message is shown on the Jamf Pro login page.",
				Computed:            true,
			},
			"disclaimer_heading": schema.StringAttribute{
				MarkdownDescription: "Title text of the disclaimer dialog.",
				Computed:            true,
			},
			"disclaimer_main_text": schema.StringAttribute{
				MarkdownDescription: "Body text of the disclaimer dialog.",
				Computed:            true,
			},
			"action_text": schema.StringAttribute{
				MarkdownDescription: "Label on the button that acknowledges the disclaimer dialog.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *LoginPageSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_login_page_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current login page settings and populates Terraform state.
func (d *LoginPageSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data LoginPageSettingsDataSourceModel
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

	got, err := d.client.GetLoginCustomizationV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro login page settings", helpers.APIErrorDetail(err))
		return
	}
	assignLoginPageSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro login page settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
