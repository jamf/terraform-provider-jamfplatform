// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DNSZoneDataSource implements the Terraform data source for a single Jamf
// Security Cloud custom DNS zone.
type DNSZoneDataSource struct {
	client *securitycloud.Client
}

var (
	_ datasource.DataSource                     = &DNSZoneDataSource{}
	_ datasource.DataSourceWithConfigValidators = &DNSZoneDataSource{}
)

// NewDNSZoneDataSource returns a new instance of DNSZoneDataSource.
func NewDNSZoneDataSource() datasource.DataSource {
	return &DNSZoneDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DNSZoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_zone"
}

// Schema returns the data source schema.
func (d *DNSZoneDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Security Cloud custom DNS zone by ID or by name. Zone names are not " +
			"required to be unique, so a name matching more than one zone is an error. Use the ID in that case." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Zone ID to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Zone name to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"domains": schema.ListAttribute{
				MarkdownDescription: "Domains that match this zone.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"authoritative_name_servers": schema.ListNestedAttribute{
				MarkdownDescription: "Authoritative name servers that resolve hostnames for this zone's domains.",
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
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id or name is supplied.
func (d *DNSZoneDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *DNSZoneDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_zone")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a DNS zone by ID or name and populates Terraform state.
func (d *DNSZoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DNSZoneDataSourceModel
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

	if !data.ID.IsNull() && data.ID.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("id"),
			"DNS zone ID is empty",
			"`id` is set to an empty string, which ExactlyOneOf still counts as configured, so `name` cannot "+
				"be used instead. This usually means a variable or a reference resolved to \"\" — set `id` to "+
				"an ID, or remove it and set `name`.",
		)
		return
	}

	var zone *securitycloud.Zone
	var err error
	byName := data.ID.IsNull()
	if byName {
		zone, err = d.client.ResolveDnsZoneV1ByName(readCtx, data.Name.ValueString())
	} else {
		zone, err = d.client.GetDnsZoneV1(readCtx, data.ID.ValueString())
	}
	if err != nil {
		if ambiguous, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err); ok {
			resp.Diagnostics.AddError(
				"Multiple Jamf Security Cloud DNS zones share that name",
				"Jamf Security Cloud does not require zone names to be unique, and more than one zone is named "+
					data.Name.ValueString()+". Look the zone up by `id` instead. Matching zone IDs: "+
					strings.Join(ambiguous.Matches, ", "),
			)
			return
		}
		if byName && helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Unable to find Jamf Security Cloud DNS zone",
				"No DNS zone on this tenant is named \""+data.Name.ValueString()+"\". Names are matched "+
					"exactly, so one differing only in capitalisation or surrounding whitespace will not be "+
					"found. Use the `jamfplatform_security_cloud_dns_zones` data source to list what exists.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to find Jamf Security Cloud DNS zone", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignDNSZoneDataSourceModel(ctx, &data, zone)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Security Cloud DNS zone data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
