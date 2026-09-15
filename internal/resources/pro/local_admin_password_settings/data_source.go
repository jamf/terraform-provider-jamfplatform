// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package local_admin_password_settings

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

// LocalAdminPasswordSettingsDataSource is the read-only mirror of the LAPS
// settings resource.
type LocalAdminPasswordSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &LocalAdminPasswordSettingsDataSource{}

// NewLocalAdminPasswordSettingsDataSource constructs a new data source.
func NewLocalAdminPasswordSettingsDataSource() datasource.DataSource {
	return &LocalAdminPasswordSettingsDataSource{}
}

// Metadata sets the data source type name.
func (d *LocalAdminPasswordSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_local_admin_password_settings"
}

// Schema returns the data source schema. Every attribute is Computed.
func (d *LocalAdminPasswordSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro local administrator password (LAPS) settings (UI: Settings → Computer Management → Security → \"Password settings for managed local administrator accounts\"). One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":                                 schema.StringAttribute{Computed: true, MarkdownDescription: "Fixed singleton identifier. Always `singleton`."},
			"laps_for_prestage_accounts_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether LAPS is enabled for managed local administrator accounts created via PreStage enrollment."},
			"rotation_interval":                  schema.StringAttribute{Computed: true, MarkdownDescription: "How often passwords are automatically rotated, or `Never` when automatic rotation is off."},
			"rotation_after_viewing_interval":    schema.StringAttribute{Computed: true, MarkdownDescription: "How long after a password is viewed in the inventory record before it is rotated."},
			"timeouts":                           timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *LocalAdminPasswordSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_local_admin_password_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read populates state from the LAPS settings endpoint.
func (d *LocalAdminPasswordSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data LocalAdminPasswordSettingsDataSourceModel
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

	got, err := d.client.GetLocalAdminPasswordSettingsV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro local administrator password settings", helpers.APIErrorDetail(err))
		return
	}

	assignLocalAdminPasswordSettingsDataSourceModel(&data, got, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro local administrator password settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
