// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ztna_shared_gateways implements the
// jamfplatform_security_cloud_ztna_shared_gateways data source, a read-only view
// of the Jamf-managed shared ZTNA gateway catalogue.
//
// Shared gateways are Jamf-operated infrastructure available to every entitled
// tenant: one "Nearest Data Center" entry plus a shared IP pool per region. They
// cannot be created, changed or deleted, which is why this package holds only a
// plural data source — the API exposes no per-id endpoint to build a singular one
// on, and a list resource over a fixed catalogue nobody manages would be
// ceremony without a payoff.
//
// The reason it exists is discovery. A custom DNS zone's name servers reference a
// gateway by an opaque four-character ID, and a shared gateway's ID is only
// visible in the admin UI or through this data source (wire-verified 2026-08-27:
// a shared gateway ID is accepted as a zone's `gateway_id`). A shared gateway
// cannot, however, be a member of a grouped gateway — that is refused.
package ztna_shared_gateways

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

// defaultReadTimeout caps how long the shared gateways read will wait.
const defaultReadTimeout = 60 * time.Second

// dataSourceID is the fixed ID this data source reports. The catalogue is the same
// for every read, so there is nothing to derive an ID from.
const dataSourceID = "ztna_shared_gateways"

// SharedGatewaysDataSource implements the Terraform data source for the
// Jamf-managed shared ZTNA gateway catalogue.
type SharedGatewaysDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &SharedGatewaysDataSource{}

// NewSharedGatewaysDataSource returns a new instance of SharedGatewaysDataSource.
func NewSharedGatewaysDataSource() datasource.DataSource {
	return &SharedGatewaysDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *SharedGatewaysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_shared_gateways"
}

// Schema returns the data source schema.
func (d *SharedGatewaysDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the **\"Shared gateways\"** catalogue in the Jamf Security Cloud admin UI: " +
			"Jamf-operated gateways available to every entitled tenant, alongside any dedicated gateways of your " +
			"own. They cannot be modified or deleted, and no status is reported for them.\n\n" +
			"Use this to resolve the ID a custom DNS zone name server needs without hard-coding it. A shared " +
			"gateway cannot be a member of a grouped gateway." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"shared_gateways": schema.ListNestedAttribute{
				MarkdownDescription: "The shared gateways available to this tenant.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Shared gateway ID. This is the value a custom DNS zone name server's " +
								"`gateway_id` takes.",
							Computed: true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Shared gateway name, for example `Nearest Data Center` or " +
								"`Shared IP Pool: Europe - UK`.",
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *SharedGatewaysDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_shared_gateways")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the shared gateway catalogue and populates Terraform state.
func (d *SharedGatewaysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SharedGatewaysDataSourceModel
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

	gateways, err := d.client.ListZtnaSharedGatewaysV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud shared ZTNA gateways", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(dataSourceID)
	data.SharedGateways = make([]SharedGatewayResultModel, 0, len(gateways.Results))
	for _, g := range gateways.Results {
		data.SharedGateways = append(data.SharedGateways, SharedGatewayResultModel{
			ID:   types.StringValue(g.ID),
			Name: types.StringValue(g.Name),
		})
	}

	tflog.Trace(ctx, "read Jamf Security Cloud shared ZTNA gateways data source", map[string]any{"count": len(data.SharedGateways)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
