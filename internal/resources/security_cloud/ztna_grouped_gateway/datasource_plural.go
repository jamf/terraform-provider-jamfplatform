// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPluralReadTimeout caps how long the plural grouped gateways read will
// wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The endpoint
// takes no parameters, so every read returns the same collection and there is
// nothing to derive an ID from.
const pluralDataSourceID = "ztna_grouped_gateways"

// GroupedGatewaysDataSource implements the Terraform data source for listing every
// Jamf Security Cloud ZTNA grouped gateway.
type GroupedGatewaysDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &GroupedGatewaysDataSource{}

// NewGroupedGatewaysDataSource returns a new instance of
// GroupedGatewaysDataSource.
func NewGroupedGatewaysDataSource() datasource.DataSource {
	return &GroupedGatewaysDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *GroupedGatewaysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_grouped_gateways"
}

// Schema returns the plural data source schema.
func (d *GroupedGatewaysDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every Jamf Security Cloud ZTNA grouped gateway on the tenant. Jamf Security " +
			"Cloud exposes no query parameters for grouped gateways, so this data source takes no search " +
			"arguments. Filter the result in Terraform." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"grouped_gateways": schema.ListNestedAttribute{
				MarkdownDescription: "The ZTNA grouped gateways on the tenant.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{MarkdownDescription: "Grouped gateway ID assigned by Jamf Security Cloud.", Computed: true},
						"name": schema.StringAttribute{MarkdownDescription: "Grouped gateway name.", Computed: true},
						"gateway_ids": schema.ListAttribute{
							MarkdownDescription: "IDs of the member gateways, in priority order.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"routing_strategy": schema.StringAttribute{
							MarkdownDescription: "Which member a device uses: `Nearest`, `Random` or `First available`.",
							Computed:            true,
						},
						"required_gateway_stability": schema.StringAttribute{
							MarkdownDescription: "How long a recovered member must stay healthy before traffic returns to it.",
							Computed:            true,
						},
						"tenant_ids": schema.ListAttribute{
							MarkdownDescription: "IDs of the tenants granted access to this grouped gateway.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"created_at": schema.StringAttribute{MarkdownDescription: "When the grouped gateway was created.", Computed: true},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *GroupedGatewaysDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_grouped_gateways")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every grouped gateway and populates Terraform state.
func (d *GroupedGatewaysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data GroupedGatewaysDataSourceModel
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

	groups, err := d.client.ListZtnaGroupedGatewaysV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud ZTNA grouped gateways", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	data.GroupedGateways = make([]GroupedGatewaysDataSourceResultModel, 0, len(groups.Results))
	for _, g := range groups.Results {
		result, resultDiags := buildGroupedGatewaysResultModel(ctx, g)
		resp.Diagnostics.Append(resultDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.GroupedGateways = append(data.GroupedGateways, result)
	}

	tflog.Trace(ctx, "read Jamf Security Cloud ZTNA grouped gateways data source", map[string]any{"count": len(data.GroupedGateways)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
