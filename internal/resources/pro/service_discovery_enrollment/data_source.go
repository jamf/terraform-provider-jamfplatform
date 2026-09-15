// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package service_discovery_enrollment

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

// ServiceDiscoveryEnrollmentDataSource implements the Terraform data source for the
// Jamf Pro service-discovery well-known settings. It returns every per-organization
// row Jamf Pro knows about, so users can discover the available server_uuids and
// their current enrollment types.
type ServiceDiscoveryEnrollmentDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &ServiceDiscoveryEnrollmentDataSource{}

// NewServiceDiscoveryEnrollmentDataSource returns a new instance of the data source.
func NewServiceDiscoveryEnrollmentDataSource() datasource.DataSource {
	return &ServiceDiscoveryEnrollmentDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ServiceDiscoveryEnrollmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_service_discovery_enrollment"
}

// Schema returns the data source schema.
func (d *ServiceDiscoveryEnrollmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro service-discovery (\"well-known\") settings for Account-Driven " +
			"enrollment. Returns one row per synced Apple Business/School Manager (AxM) organization, with its " +
			"`server_uuid`, display name, and current `enrollment_type`. Useful for discovering the `server_uuid` values to " +
			"manage with `jamfplatform_pro_service_discovery_enrollment`. Requires Jamf Pro 11.25.0 or later. One record " +
			"per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"well_known_setting": schema.ListNestedAttribute{
				MarkdownDescription: "One row per synced AxM organization Jamf Pro knows about.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"server_uuid": schema.StringAttribute{
							MarkdownDescription: "Server UUID of the organization's Automated Device Enrollment token.",
							Computed:            true,
						},
						"enrollment_type": schema.StringAttribute{
							MarkdownDescription: "Current Account-Driven enrollment type hosted for this org: `none`, `mdm-byod`, or `mdm-adde`.",
							Computed:            true,
						},
						"org_name": schema.StringAttribute{
							MarkdownDescription: "Display name of the Apple Business/School Manager organization.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *ServiceDiscoveryEnrollmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_service_discovery_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current well-known settings and populates Terraform state.
func (d *ServiceDiscoveryEnrollmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ServiceDiscoveryEnrollmentDataSourceModel
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

	got, err := d.client.GetServiceDiscoveryEnrollmentWellKnownSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro service discovery well-known settings", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignServiceDiscoveryEnrollmentDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro service discovery well-known settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
