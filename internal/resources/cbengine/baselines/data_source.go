// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package baselines

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &BaselinesDataSource{}

// NewBaselinesDataSource returns a new instance of BaselinesDataSource.
func NewBaselinesDataSource() datasource.DataSource {
	return &BaselinesDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *BaselinesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cbengine_baselines"
}

const defaultReadTimeout = 90 * time.Second

// Schema sets the Terraform schema for the data source.
func (d *BaselinesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns the mSCP baselines allowed for Compliance Benchmarks. Requires **Compliance Benchmarks API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"timeouts": timeouts.Attributes(ctx),
			"baselines": schema.ListNestedAttribute{
				MarkdownDescription: "List of baselines.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Unique identifier for the baseline.",
							Computed:            true,
						},
						"baseline_id": schema.StringAttribute{
							MarkdownDescription: "Baseline ID.",
							Computed:            true,
						},
						"title": schema.StringAttribute{
							MarkdownDescription: "Title of the baseline.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Description of the baseline.",
							Computed:            true,
						},
						"rule_count": schema.Int64Attribute{
							MarkdownDescription: "Number of rules in the baseline.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *BaselinesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_cbengine_baselines", providerdata.ComplianceBenchmarksScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = compliancebenchmarks.New(pd.Client)
}

// Read implements datasource.DataSource for BaselinesDataSource. It fetches the list of baselines from the API and sets the state.
func (d *BaselinesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data BaselinesDataSourceModel

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

	baselinesResp, err := d.client.ListBaselines(readCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get baselines",
			helpers.APIErrorDetail(err),
		)
		return
	}

	var baselines []BaselineModel
	for _, b := range baselinesResp.Baselines {
		baselines = append(baselines, BaselineModel{
			ID:          types.StringValue(b.ID),
			BaselineID:  types.StringValue(b.BaselineID),
			Title:       types.StringValue(b.Title),
			Description: types.StringValue(b.Description),
			RuleCount:   types.Int64Value(int64(b.RuleCount)),
		})
	}

	data.Baselines = baselines
	data.Timeouts = timeoutsValue

	tflog.Trace(ctx, "read a data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
