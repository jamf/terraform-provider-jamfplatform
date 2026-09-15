// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmarks

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &BenchmarksDataSource{}

// NewBenchmarksDataSource instantiates the benchmarks listing data source.
func NewBenchmarksDataSource() datasource.DataSource {
	return &BenchmarksDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *BenchmarksDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cbengine_benchmarks"
}

const defaultReadTimeout = 90 * time.Second

// Schema defines the Terraform schema for listing CBEngine benchmarks.
func (d *BenchmarksDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns all Compliance Benchmarks from Jamf Pro. Requires **Compliance Benchmarks API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"timeouts": timeouts.Attributes(ctx),
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"benchmarks": schema.ListNestedAttribute{
				MarkdownDescription: "CBEngine benchmarks returned by ListBenchmarks.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Benchmark identifier.",
							Computed:            true,
						},
						"title": schema.StringAttribute{
							MarkdownDescription: "Benchmark title.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Benchmark description, if provided.",
							Computed:            true,
						},
						"update_available": schema.BoolAttribute{
							MarkdownDescription: "Indicates whether an update is available for the benchmark.",
							Computed:            true,
						},
						"modified": schema.BoolAttribute{
							MarkdownDescription: "Indicates whether the benchmark has been modified from its baseline.",
							Computed:            true,
						},
						"sync_state": schema.StringAttribute{
							MarkdownDescription: "Current synchronization state reported by CBEngine.",
							Computed:            true,
						},
						"target_device_groups": schema.ListAttribute{
							MarkdownDescription: "Device groups targeted by the benchmark.",
							ElementType:         types.StringType,
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure wires the provider client into the data source.
func (d *BenchmarksDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected *providerdata.Data. Please report this issue to the provider developers.",
		)
		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_cbengine_benchmarks", providerdata.ComplianceBenchmarksScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = compliancebenchmarks.New(pd.Client)
}

// Read invokes ListBenchmarks and maps the response into Terraform state.
func (d *BenchmarksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data BenchmarksDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeoutsValue := data.Timeouts

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure provider block is set up correctly.",
		)
		return
	}

	benchmarks, err := d.client.ListBenchmarks(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list CBEngine benchmarks", helpers.APIErrorDetail(err))
		return
	}

	entries := make([]BenchmarkListItem, 0, len(benchmarks.Benchmarks))
	for _, bench := range benchmarks.Benchmarks {
		targetGroups := types.ListNull(types.StringType)
		if len(bench.Target.DeviceGroups) > 0 {
			values := make([]attr.Value, len(bench.Target.DeviceGroups))
			for i, group := range bench.Target.DeviceGroups {
				values[i] = helpers.StringValueOrNull(group)
			}
			targetGroups, _ = types.ListValue(types.StringType, values)
		}

		entries = append(entries, BenchmarkListItem{
			ID:                 helpers.StringValueOrNull(bench.ID),
			Title:              helpers.StringValueOrNull(bench.Title),
			Description:        helpers.StringValueOrNull(bench.Description),
			UpdateAvailable:    types.BoolValue(bench.UpdateAvailable),
			Modified:           types.BoolValue(bench.Modified),
			SyncState:          helpers.StringValueOrNull(bench.SyncState),
			TargetDeviceGroups: targetGroups,
		})
	}

	data.ID = types.StringValue("cbengine_benchmarks")
	data.Benchmarks = entries
	data.Timeouts = timeoutsValue

	tflog.Trace(ctx, "read cbengine benchmarks data source", map[string]any{
		"count": len(entries),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
