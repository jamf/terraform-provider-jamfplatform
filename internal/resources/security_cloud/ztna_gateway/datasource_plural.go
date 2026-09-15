// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

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

// defaultPluralReadTimeout caps how long the plural gateways read will wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The gateway
// list endpoint takes no parameters, so every read returns the same collection and
// there is nothing to derive an ID from.
const pluralDataSourceID = "ztna_gateways"

// GatewaysDataSource implements the Terraform data source for listing every
// dedicated Jamf Security Cloud ZTNA gateway.
type GatewaysDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &GatewaysDataSource{}

// NewGatewaysDataSource returns a new instance of GatewaysDataSource.
func NewGatewaysDataSource() datasource.DataSource {
	return &GatewaysDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *GatewaysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_gateways"
}

// Schema returns the plural data source schema.
func (d *GatewaysDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every dedicated Jamf Security Cloud ZTNA gateway on the tenant. Jamf Security " +
			"Cloud exposes no query parameters for gateways, so this data source takes no search arguments. " +
			"Filter the result in Terraform. Shared gateways are a separate catalogue, read with " +
			"`jamfplatform_security_cloud_ztna_shared_gateways`." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"gateways": schema.ListNestedAttribute{
				MarkdownDescription: "The dedicated ZTNA gateways on the tenant.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            schema.StringAttribute{MarkdownDescription: "Gateway ID assigned by Jamf Security Cloud.", Computed: true},
						"name":          schema.StringAttribute{MarkdownDescription: "Gateway name.", Computed: true},
						"egress_region": schema.StringAttribute{MarkdownDescription: "Egress region this gateway is deployed to.", Computed: true},
						"contact": schema.SingleNestedAttribute{
							MarkdownDescription: "Operational contact for this gateway.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"name":  schema.StringAttribute{MarkdownDescription: "Contact name, or a team name.", Computed: true},
								"email": schema.StringAttribute{MarkdownDescription: "Contact email address.", Computed: true},
							},
						},
						"enabled": schema.BoolAttribute{MarkdownDescription: "Whether the deployment is active.", Computed: true},
						"tenant_ids": schema.ListAttribute{
							MarkdownDescription: "IDs of the tenants granted access to this gateway.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"ipsec_source_ip_addresses": schema.ListAttribute{
							MarkdownDescription: "Addresses IPsec traffic from Jamf Security Cloud originates from.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"dedicated_egress_ips_enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether this is a dedicated internet gateway.",
							Computed:            true,
						},
						"dedicated_egress_ip_addresses": schema.ListAttribute{
							MarkdownDescription: "The private egress IP addresses Jamf Security Cloud provisioned for a dedicated internet gateway.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"ipsec":  dsIPSecAttribute(),
						"status": dsStatusAttribute(),
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *GatewaysDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_gateways")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every gateway and populates Terraform state.
func (d *GatewaysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data GatewaysDataSourceModel
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

	gateways, err := d.client.ListZtnaGatewaysV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud ZTNA gateways", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	data.Gateways = make([]GatewaysDataSourceResultModel, 0, len(gateways.Results))
	for _, g := range gateways.Results {
		result, resultDiags := buildGatewaysResultModel(ctx, g)
		resp.Diagnostics.Append(resultDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Gateways = append(data.Gateways, result)
	}

	tflog.Trace(ctx, "read Jamf Security Cloud ZTNA gateways data source", map[string]any{"count": len(data.Gateways)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
