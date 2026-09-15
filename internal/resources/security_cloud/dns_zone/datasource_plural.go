// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

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

// defaultPluralReadTimeout caps how long the plural DNS zones read will wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The zone
// list endpoint takes no filter, so every read of this data source returns the
// same collection and there is nothing to derive an ID from.
const pluralDataSourceID = "dns_zones"

// DNSZonesDataSource implements the Terraform data source for listing every Jamf
// Security Cloud custom DNS zone.
type DNSZonesDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &DNSZonesDataSource{}

// NewDNSZonesDataSource returns a new instance of DNSZonesDataSource.
func NewDNSZonesDataSource() datasource.DataSource {
	return &DNSZonesDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DNSZonesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_zones"
}

// Schema returns the plural data source schema.
func (d *DNSZonesDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every Jamf Security Cloud custom DNS zone on the tenant. Jamf Security Cloud " +
			"exposes no filter parameters for zones, so this data source takes no search arguments. Filter the " +
			"result in Terraform." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"dns_zones": schema.ListNestedAttribute{
				MarkdownDescription: "The custom DNS zones on the tenant, ordered by name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Zone ID assigned by Jamf Security Cloud.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Zone name.",
							Computed:            true,
						},
						"domains": schema.ListAttribute{
							MarkdownDescription: "Domains that match this zone.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"authoritative_name_servers": schema.ListNestedAttribute{
							MarkdownDescription: "Authoritative name servers for this zone.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"ip_address": schema.StringAttribute{
										MarkdownDescription: "Name server IPv4 address.",
										Computed:            true,
									},
									"gateway_id": schema.StringAttribute{
										MarkdownDescription: "ID of the gateway this name server is reachable through.",
										Computed:            true,
									},
								},
							},
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
func (d *DNSZonesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_zones")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every DNS zone and populates Terraform state.
func (d *DNSZonesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DNSZonesDataSourceModel
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

	zones, err := d.client.ListDnsZonesV1(readCtx, defaultZoneSort)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud DNS zones", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	data.DNSZones = make([]DNSZonesDataSourceResultModel, 0, len(zones.Results))
	for _, z := range zones.Results {
		result, resultDiags := buildDNSZonesResultModel(ctx, z)
		resp.Diagnostics.Append(resultDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.DNSZones = append(data.DNSZones, result)
	}

	tflog.Trace(ctx, "read Jamf Security Cloud DNS zones data source", map[string]any{"count": len(data.DNSZones)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
