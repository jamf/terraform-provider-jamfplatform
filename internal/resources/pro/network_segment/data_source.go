// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// NetworkSegmentDataSource implements the Terraform data source for Jamf Pro network
// segments. The singular data source supports lookup by ID OR by name — exactly one
// of the two must be supplied.
type NetworkSegmentDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &NetworkSegmentDataSource{}
	_ datasource.DataSourceWithConfigure        = &NetworkSegmentDataSource{}
	_ datasource.DataSourceWithConfigValidators = &NetworkSegmentDataSource{}
)

// NewNetworkSegmentDataSource returns a new instance of NetworkSegmentDataSource.
func NewNetworkSegmentDataSource() datasource.DataSource {
	return &NetworkSegmentDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *NetworkSegmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_network_segment"
}

// Schema returns the data source schema.
func (d *NetworkSegmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro network segment by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Network segment ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Network segment display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"starting_address":     schema.StringAttribute{MarkdownDescription: "Starting IP address of the network segment.", Computed: true},
			"ending_address":       schema.StringAttribute{MarkdownDescription: "Ending IP address of the network segment.", Computed: true},
			"building":             schema.StringAttribute{MarkdownDescription: "Building associated with the network segment.", Computed: true},
			"department":           schema.StringAttribute{MarkdownDescription: "Department associated with the network segment.", Computed: true},
			"override_buildings":   schema.BoolAttribute{MarkdownDescription: "Whether devices joining this segment have their building overridden.", Computed: true},
			"override_departments": schema.BoolAttribute{MarkdownDescription: "Whether devices joining this segment have their department overridden.", Computed: true},
			"distribution_point":   schema.StringAttribute{MarkdownDescription: "Distribution point assigned by Jamf Pro for this segment.", Computed: true},
			"distribution_server":  schema.StringAttribute{MarkdownDescription: "Distribution server assigned by Jamf Pro for this segment.", Computed: true},
			"swu_server":           schema.StringAttribute{MarkdownDescription: "Software update server assigned by Jamf Pro for this segment.", Computed: true},
			"url":                  schema.StringAttribute{MarkdownDescription: "NetBoot/imaging URL assigned by Jamf Pro for this segment.", Computed: true},
			"timeouts":             timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *NetworkSegmentDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *NetworkSegmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_network_segment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a network segment by ID or by name and populates Terraform state.
func (d *NetworkSegmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data NetworkSegmentDataSourceModel
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

	var (
		got *proclassic.NetworkSegment
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetNetworkSegmentByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetNetworkSegmentByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing network segment selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro network segment", helpers.APIErrorDetail(err))
		return
	}
	assignNetworkSegmentDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro network segment data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
