// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// HostnameMappingsDataSource implements the Terraform data source for Jamf Security
// Cloud custom hostname mappings.
type HostnameMappingsDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &HostnameMappingsDataSource{}

// NewHostnameMappingsDataSource returns a new instance of HostnameMappingsDataSource.
func NewHostnameMappingsDataSource() datasource.DataSource {
	return &HostnameMappingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *HostnameMappingsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_hostname_mappings"
}

// Schema returns the data source schema. It takes no arguments: there is one mapping
// set per tenant and nothing to select it by.
func (d *HostnameMappingsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads **\"Hostname mapping\"** under Custom DNS in the Jamf Security Cloud admin " +
			"UI: the custom IPv4 and IPv6 mappings for internal host names your organization uses.\n\n" +
			"There is one set per tenant, so this data source takes no arguments. A tenant with no mappings " +
			"reads back an empty collection rather than an error." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton`, since a tenant holds one set of hostname mappings.",
				Computed:            true,
			},
			"mappings": schema.ListNestedAttribute{
				MarkdownDescription: "The tenant's hostname mappings, in the order Jamf Security Cloud returns " +
					"them, which is not the order they were written in.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"hostname": schema.StringAttribute{
							MarkdownDescription: "The host name this mapping applies to.",
							Computed:            true,
						},
						"ipv4_addresses": schema.ListAttribute{
							MarkdownDescription: "The IPv4 addresses the host name resolves to. Empty when none " +
								"are configured.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"ipv6_addresses": schema.ListAttribute{
							MarkdownDescription: "The IPv6 addresses the host name resolves to. Empty when none " +
								"are configured.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"connect_to_ztna": schema.BoolAttribute{
							MarkdownDescription: "Whether this host name's traffic is routed through Zero Trust " +
								"Network Access.",
							Computed: true,
						},
						"connect_to_secure_dns": schema.BoolAttribute{
							MarkdownDescription: "Whether this host name's traffic is routed through Secure DNS.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the shared
// providerdata.ConfigureSecurityCloud helper.
func (d *HostnameMappingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_hostname_mappings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the tenant's hostname mappings and populates Terraform state.
//
// An empty set is an empty collection here rather than an error, unlike the sibling
// search domain data source. The difference is the wire's: an unset search domain is a
// 404 and cannot be told apart from a failure without saying so, whereas an empty
// mapping set is an ordinary 200 that a for expression over the collection handles on
// its own.
func (d *HostnameMappingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var data HostnameMappingsDataSourceModel
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

	got, err := d.client.GetDnsCustomHostnameMappingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Security Cloud hostname mappings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignHostnameMappingsDataSourceModel(ctx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Security Cloud hostname mappings data source", map[string]any{"count": len(data.Mappings)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
