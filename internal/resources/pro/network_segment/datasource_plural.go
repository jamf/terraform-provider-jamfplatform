// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// NetworkSegmentsDataSource implements the Terraform data source for Jamf Pro
// network segment searches.
type NetworkSegmentsDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource              = &NetworkSegmentsDataSource{}
	_ datasource.DataSourceWithConfigure = &NetworkSegmentsDataSource{}
)

// NewNetworkSegmentsDataSource returns a new instance of NetworkSegmentsDataSource.
func NewNetworkSegmentsDataSource() datasource.DataSource {
	return &NetworkSegmentsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *NetworkSegmentsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_network_segments"
}

// Schema returns the plural data source schema. The nested object reflects exactly
// what the classic /networksegments list endpoint returns — id, name, and the IP
// range — and omits the per-item fields that require an extra GET per segment.
func (d *NetworkSegmentsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List Jamf Pro network segments. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched. Omit the filter to receive every network segment. Per-item fields beyond id, name, and the IP range require a singular `jamfplatform_pro_network_segment` data source lookup." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter":   filters.ClassicFilterAttribute(),
			"network_segments": schema.ListNestedAttribute{
				MarkdownDescription: "Network segments matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{MarkdownDescription: "Network segment ID assigned by Jamf Pro.", Computed: true},
						"name":             schema.StringAttribute{MarkdownDescription: "Network segment display name.", Computed: true},
						"starting_address": schema.StringAttribute{MarkdownDescription: "Starting IP address of the network segment.", Computed: true},
						"ending_address":   schema.StringAttribute{MarkdownDescription: "Ending IP address of the network segment.", Computed: true},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *NetworkSegmentsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_network_segments")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches network segments and populates state. Applies the optional client-side
// substring filter after the full list is retrieved.
func (d *NetworkSegmentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data NetworkSegmentsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	listResp, err := d.client.ListNetworkSegments(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro network segments", helpers.APIErrorDetail(err))
		return
	}

	items := []proclassic.NetworkSegmentsItemNetworkSegment{}
	if listResp != nil {
		items = listResp.NetworkSegments
	}

	filter := filters.ClassicFilterModel{}
	if data.Filter != nil {
		filter = *data.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, func(s proclassic.NetworkSegmentsItemNetworkSegment) string {
		if s.Name == nil {
			return ""
		}
		return *s.Name
	})

	results := make([]NetworkSegmentsDataSourceResultModel, 0, len(items))
	for _, s := range items {
		results = append(results, NetworkSegmentsDataSourceResultModel{
			ID:              helpers.StringValueFromIntPtr(s.ID),
			Name:            helpers.StringPointerValueOrNull(s.Name),
			StartingAddress: helpers.StringPointerValueOrNull(s.StartingAddress),
			EndingAddress:   helpers.StringPointerValueOrNull(s.EndingAddress),
		})
	}

	data.NetworkSegments = results
	data.ID = types.StringValue("network_segments")

	tflog.Trace(ctx, "listed Jamf Pro network segments data source", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"count":          len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
