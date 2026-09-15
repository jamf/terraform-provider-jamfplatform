// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

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

// FileShareDistributionPointDataSource implements the Terraform data source.
type FileShareDistributionPointDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &FileShareDistributionPointDataSource{}

// NewFileShareDistributionPointDataSource returns a new instance.
func NewFileShareDistributionPointDataSource() datasource.DataSource {
	return &FileShareDistributionPointDataSource{}
}

// Metadata sets the data source type name.
func (d *FileShareDistributionPointDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_file_share_distribution_point"
}

// Schema returns the data source schema. Look up by `id` or by `name`.
func (d *FileShareDistributionPointDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro file share distribution point by `id` or by `name`. Provide exactly one." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the distribution point to look up. Provide either `id` or `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the distribution point to look up. Provide either `id` or `name`.",
				Optional:            true,
				Computed:            true,
			},
			"server_name": schema.StringAttribute{
				MarkdownDescription: "Hostname or IP address of the distribution point server.",
				Computed:            true,
			},
			"file_sharing_connection_type": schema.StringAttribute{
				MarkdownDescription: "File sharing protocol (`AFP`, `SMB`, or `NONE`).",
				Computed:            true,
			},
			"principal": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the principal distribution point.",
				Computed:            true,
			},
			"backup_distribution_point_id": schema.StringAttribute{
				MarkdownDescription: "ID of the failover distribution point, or `-1` for none.",
				Computed:            true,
			},
			"enable_load_balancing": schema.BoolAttribute{
				MarkdownDescription: "Whether randomized load sharing with the failover is enabled.",
				Computed:            true,
			},
			"share_name": schema.StringAttribute{
				MarkdownDescription: "Name of the file share.",
				Computed:            true,
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "Port used for file sharing.",
				Computed:            true,
			},
			"workgroup": schema.StringAttribute{
				MarkdownDescription: "Workgroup or domain for the file share.",
				Computed:            true,
			},
			"read_write_username": schema.StringAttribute{
				MarkdownDescription: "Username for the read/write account.",
				Computed:            true,
			},
			"read_only_username": schema.StringAttribute{
				MarkdownDescription: "Username for the read-only account.",
				Computed:            true,
			},
			"https_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether packages may be downloaded over HTTPS.",
				Computed:            true,
			},
			"https_port": schema.Int64Attribute{
				MarkdownDescription: "Port used for HTTPS downloads.",
				Computed:            true,
			},
			"https_context": schema.StringAttribute{
				MarkdownDescription: "Context path appended to the server for HTTPS downloads.",
				Computed:            true,
			},
			"https_security_type": schema.StringAttribute{
				MarkdownDescription: "Authentication type for HTTPS downloads (`USERNAME_PASSWORD` or `NONE`).",
				Computed:            true,
			},
			"https_username": schema.StringAttribute{
				MarkdownDescription: "Username for the HTTPS account.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *FileShareDistributionPointDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_file_share_distribution_point")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a distribution point by ID or name and populates state.
func (d *FileShareDistributionPointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data FileShareDistributionPointDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !data.ID.IsNull() && data.ID.ValueString() != ""
	hasName := !data.Name.IsNull() && data.Name.ValueString() != ""
	if hasID == hasName {
		resp.Diagnostics.AddError(
			"Invalid lookup",
			"Provide exactly one of 'id' or 'name' to look up a Jamf Pro file share distribution point.",
		)
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var got *pro.DistributionPoint
	var err error
	if hasID {
		got, err = d.client.GetDistributionPointV1(readCtx, data.ID.ValueString())
	} else {
		got, err = d.client.ResolveDistributionPointV1ByName(readCtx, data.Name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro file share distribution point", helpers.APIErrorDetail(err))
		return
	}
	assignFileShareDistributionPointDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro file share distribution point data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
