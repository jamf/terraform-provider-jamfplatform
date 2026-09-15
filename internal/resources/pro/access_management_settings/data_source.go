// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package access_management_settings

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

// AccessManagementSettingsDataSource implements the Terraform data source for Jamf Pro
// Access Management settings.
type AccessManagementSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AccessManagementSettingsDataSource{}

// NewAccessManagementSettingsDataSource returns a new instance of the data source.
func NewAccessManagementSettingsDataSource() datasource.DataSource {
	return &AccessManagementSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AccessManagementSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_access_management_settings"
}

// Schema returns the data source schema.
func (d *AccessManagementSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Access Management settings for Managed Apple Accounts. One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"automated_device_enrollment_server_uuid": schema.StringAttribute{
				MarkdownDescription: "Server UUID of the Automated Device Enrollment (ADE) server object Jamf Pro names in its Get Token response for Managed Apple Account access management. Null when no ADE server is configured.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *AccessManagementSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_access_management_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current Access Management settings and populates Terraform state.
func (d *AccessManagementSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AccessManagementSettingsDataSourceModel
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

	got, err := d.client.GetEnrollmentAccessManagementV4(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Access Management settings", helpers.APIErrorDetail(err))
		return
	}
	assignAccessManagementSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Access Management settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
