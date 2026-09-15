// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_macos_settings

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

// SelfServiceMacosSettingsDataSource implements the Terraform data source for the Jamf Pro
// Self Service for macOS app settings.
type SelfServiceMacosSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SelfServiceMacosSettingsDataSource{}

// NewSelfServiceMacosSettingsDataSource returns a new instance of the data source.
func NewSelfServiceMacosSettingsDataSource() datasource.DataSource {
	return &SelfServiceMacosSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *SelfServiceMacosSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_macos_settings"
}

// Schema returns the data source schema.
func (d *SelfServiceMacosSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Self Service for macOS app settings (Settings > Self Service > macOS). One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"install_automatically": schema.BoolAttribute{
				MarkdownDescription: "Whether the Self Service app is installed automatically on computers.",
				Computed:            true,
			},
			"install_location": schema.StringAttribute{
				MarkdownDescription: "Path at which the Self Service app is installed.",
				Computed:            true,
			},
			"login_method": schema.StringAttribute{
				MarkdownDescription: "Self Service user login behavior: `NotRequired` (login disabled), `Anonymous` (login optional), or `Required`.",
				Computed:            true,
			},
			"authentication_type": schema.StringAttribute{
				MarkdownDescription: "Login type used when asking users to log in: `Basic` (Directory Service account or Jamf Pro user account) or `Saml` (Single Sign-On).",
				Computed:            true,
			},
			"keychain_credential_storage_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether users may store their login credentials in Keychain Access.",
				Computed:            true,
			},
			"fido2_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether FIDO2 authentication is enabled.",
				Computed:            true,
			},
			"notifications_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Self Service notifications are displayed for items available to users.",
				Computed:            true,
			},
			"alert_user_approved_mdm": schema.BoolAttribute{
				MarkdownDescription: "Whether users are notified that they must approve the organization's MDM profile.",
				Computed:            true,
			},
			"default_landing_page": schema.StringAttribute{
				MarkdownDescription: "Content that displays when Self Service opens: `HOME`, `BROWSE`, `HISTORY`, or `NOTIFICATIONS`.",
				Computed:            true,
			},
			"default_home_category_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the category shown when the landing page is `BROWSE`. `-1` means All Items.",
				Computed:            true,
			},
			"bookmarks_display_name": schema.StringAttribute{
				MarkdownDescription: "Name displayed for the Bookmarks section in Self Service.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *SelfServiceMacosSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_macos_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current Self Service macOS settings and populates Terraform state.
func (d *SelfServiceMacosSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SelfServiceMacosSettingsDataSourceModel
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

	got, err := d.client.GetSelfServiceSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Self Service macOS settings", helpers.APIErrorDetail(err))
		return
	}
	assignSelfServiceMacosSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Self Service macOS settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
