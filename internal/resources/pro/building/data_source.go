// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// BuildingDataSource implements the Terraform data source for Jamf Pro buildings.
type BuildingDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &BuildingDataSource{}
	_ datasource.DataSourceWithConfigure = &BuildingDataSource{}
)

// NewBuildingDataSource returns a new instance of BuildingDataSource.
func NewBuildingDataSource() datasource.DataSource {
	return &BuildingDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *BuildingDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_building"
}

// Schema returns the data source schema.
func (d *BuildingDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro building by ID." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Building ID to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Building display name.",
				Computed:            true,
			},
			"city": schema.StringAttribute{
				MarkdownDescription: "City the building is located in.",
				Computed:            true,
			},
			"country": schema.StringAttribute{
				MarkdownDescription: "Country the building is located in.",
				Computed:            true,
			},
			"state_province": schema.StringAttribute{
				MarkdownDescription: "State, province, or administrative region.",
				Computed:            true,
			},
			"street_address_1": schema.StringAttribute{
				MarkdownDescription: "Primary street address line.",
				Computed:            true,
			},
			"street_address_2": schema.StringAttribute{
				MarkdownDescription: "Secondary street address line.",
				Computed:            true,
			},
			"zip_postal_code": schema.StringAttribute{
				MarkdownDescription: "Zip or postal code.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *BuildingDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_building")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a building by ID and populates Terraform state.
func (d *BuildingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data BuildingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "The id attribute must be provided to read a Jamf Pro building.")
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := d.client.GetBuildingV1(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro building", helpers.APIErrorDetail(err))
		return
	}
	assignBuildingDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro building data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
