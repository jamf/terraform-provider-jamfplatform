// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// BenchmarkDataSource implements the Terraform data source for Jamf Compliance Benchmarks.
type BenchmarkDataSource struct {
	client *compliancebenchmarks.Client
}

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &BenchmarkDataSource{}

// NewBenchmarkDataSource returns a new instance of BenchmarkDataSource.
func NewBenchmarkDataSource() datasource.DataSource {
	return &BenchmarkDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *BenchmarkDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cbengine_benchmark"
}

// Schema sets the Terraform schema for the data source.
func (d *BenchmarkDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns a benchmark by ID or title. Requires **Compliance Benchmarks API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The benchmark ID to fetch. Optional if title is set.",
				Optional:            true,
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "The benchmark title to fetch. Optional if id is set.",
				Optional:            true,
			},
			"benchmark_id": schema.StringAttribute{
				MarkdownDescription: "Benchmark ID.",
				Computed:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Tenant ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description.",
				Computed:            true,
			},
			"sources": schema.ListNestedAttribute{
				MarkdownDescription: "mSCP sources (branch + revision) spanned by the benchmark.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"branch": schema.StringAttribute{
							MarkdownDescription: "Branch.",
							Computed:            true,
						},
						"revision": schema.StringAttribute{
							MarkdownDescription: "Revision.",
							Computed:            true,
						},
					},
				},
			},
			"selected_os_versions": schema.SetNestedAttribute{
				MarkdownDescription: "Operating system versions the benchmark applies to.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"os_type": schema.StringAttribute{
							MarkdownDescription: "Operating system type (e.g. `MAC_OS`).",
							Computed:            true,
						},
						"os_version": schema.Int64Attribute{
							MarkdownDescription: "Major operating system version (e.g. `26` = macOS Tahoe).",
							Computed:            true,
						},
					},
				},
			},
			"available_os_versions": schema.ListNestedAttribute{
				MarkdownDescription: "All operating system versions available for the benchmark's baseline.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"os_type": schema.StringAttribute{
							MarkdownDescription: "Operating system type (e.g. `MAC_OS`).",
							Computed:            true,
						},
						"os_version": schema.Int64Attribute{
							MarkdownDescription: "Major operating system version.",
							Computed:            true,
						},
					},
				},
			},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: "Rules.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Rule ID.",
							Computed:            true,
						},
						"section_name": schema.StringAttribute{
							MarkdownDescription: "Section name for the rule.",
							Computed:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the rule is enabled.",
							Computed:            true,
						},
						"title": schema.StringAttribute{
							MarkdownDescription: "Title of the rule.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Description of the rule.",
							Computed:            true,
						},
						"references": schema.ListAttribute{
							MarkdownDescription: "References for the rule.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"odv_value": schema.StringAttribute{
							MarkdownDescription: "ODV value.",
							Computed:            true,
						},
						"odv_hint": schema.StringAttribute{
							MarkdownDescription: "ODV hint.",
							Computed:            true,
						},
						"odv_placeholder": schema.StringAttribute{
							MarkdownDescription: "ODV placeholder.",
							Computed:            true,
						},
						"odv_type": schema.StringAttribute{
							MarkdownDescription: "ODV type.",
							Computed:            true,
						},
						"odv_validation_min": schema.Int64Attribute{
							MarkdownDescription: "Minimum value.",
							Computed:            true,
						},
						"odv_validation_max": schema.Int64Attribute{
							MarkdownDescription: "Maximum value.",
							Computed:            true,
						},
						"odv_validation_enum_values": schema.ListAttribute{
							MarkdownDescription: "Allowed enum values.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"odv_validation_regex": schema.StringAttribute{
							MarkdownDescription: "Regex pattern.",
							Computed:            true,
						},
						"supported_os": schema.ListNestedAttribute{
							MarkdownDescription: "Supported operating systems.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"os_type": schema.StringAttribute{
										MarkdownDescription: "OS type (e.g. `MAC_OS`, `IOS`).",
										Computed:            true,
									},
									"os_version": schema.Int64Attribute{
										MarkdownDescription: "OS version.",
										Computed:            true,
									},
									"management_type": schema.StringAttribute{
										MarkdownDescription: "Management type (e.g. `MANAGED`, `BYOD`).",
										Computed:            true,
									},
								},
							},
						},
						"os_specific_defaults": schema.MapNestedAttribute{
							MarkdownDescription: "OS-specific rule defaults.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"title": schema.StringAttribute{
										MarkdownDescription: "OS-specific rule title.",
										Computed:            true,
									},
									"description": schema.StringAttribute{
										MarkdownDescription: "OS-specific rule description.",
										Computed:            true,
									},
									"odv_value": schema.StringAttribute{
										MarkdownDescription: "Recommended ODV value.",
										Computed:            true,
									},
									"odv_hint": schema.StringAttribute{
										MarkdownDescription: "Recommended ODV hint.",
										Computed:            true,
									},
								},
							},
						},
						"depends_on": schema.ListAttribute{
							MarkdownDescription: "IDs of rules this rule depends on.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"reportable": schema.BoolAttribute{
							MarkdownDescription: "Whether the rule produces reportable compliance data.",
							Computed:            true,
						},
						"smart_card": schema.BoolAttribute{
							MarkdownDescription: "Whether the rule is related to smart card configuration.",
							Computed:            true,
						},
					},
				},
			},
			"target_device_groups": schema.SetAttribute{
				MarkdownDescription: "All device group Platform IDs targeted by this benchmark.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"enforcement_mode": schema.StringAttribute{
				MarkdownDescription: "Enforcement mode.",
				Computed:            true,
			},
			"deleted": schema.BoolAttribute{
				MarkdownDescription: "Deleted flag.",
				Computed:            true,
			},
			"update_available": schema.BoolAttribute{
				MarkdownDescription: "Update available flag.",
				Computed:            true,
			},
			"can_switch_to_enforce": schema.BoolAttribute{
				MarkdownDescription: "Whether the benchmark can be switched to MONITOR_AND_ENFORCE enforcement mode.",
				Computed:            true,
			},
			"last_updated_at": schema.StringAttribute{
				MarkdownDescription: "Last updated at (RFC3339).",
				Computed:            true,
			},
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *BenchmarkDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_cbengine_benchmark", providerdata.ComplianceBenchmarksScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = compliancebenchmarks.New(pd.Client)
}

// Read fetches a benchmark by ID or title and populates the Terraform state.
func (d *BenchmarkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data BenchmarkDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure provider block is set up correctly.",
		)
		return
	}

	var bench *compliancebenchmarks.BenchmarkResponseV2
	var err error
	if !data.ID.IsNull() && data.ID.ValueString() != "" {
		bench, err = d.client.GetBenchmark(ctx, data.ID.ValueString())
	} else if !data.Title.IsNull() && data.Title.ValueString() != "" {
		id, idErr := d.client.ResolveBenchmarkIDByName(ctx, data.Title.ValueString())
		if idErr != nil {
			resp.Diagnostics.AddError("Unable to find benchmark", helpers.APIErrorDetail(idErr))
			return
		}
		bench, err = d.client.GetBenchmark(ctx, id)
	} else {
		resp.Diagnostics.AddError(
			"Missing Required Attribute",
			"Either 'id' or 'title' must be set to look up a benchmark.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get benchmark",
			helpers.APIErrorDetail(err),
		)
		return
	}

	prevID := data.ID
	assignBenchmarkDataSourceFromResponse(&data, bench)
	data.ID = prevID

	tflog.Trace(ctx, "read a data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
