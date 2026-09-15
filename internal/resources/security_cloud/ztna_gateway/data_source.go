// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

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

// GatewayDataSource implements the Terraform data source for a single Jamf
// Security Cloud ZTNA gateway.
type GatewayDataSource struct {
	client *securitycloud.Client
}

var (
	_ datasource.DataSource                     = &GatewayDataSource{}
	_ datasource.DataSourceWithConfigValidators = &GatewayDataSource{}
)

// NewGatewayDataSource returns a new instance of GatewayDataSource.
func NewGatewayDataSource() datasource.DataSource {
	return &GatewayDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *GatewayDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_gateway"
}

// Schema returns the data source schema.
func (d *GatewayDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a dedicated Jamf Security Cloud ZTNA gateway by ID or by name. Use it to " +
			"resolve the gateway ID a custom DNS zone name server needs. The IPsec pre-shared key is never " +
			"reported, because Jamf Security Cloud does not return it." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Gateway ID to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Gateway name to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"egress_region": schema.StringAttribute{
				MarkdownDescription: "Egress region this gateway is deployed to.",
				Computed:            true,
			},
			"contact": schema.SingleNestedAttribute{
				MarkdownDescription: "Operational contact for this gateway.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"name":  schema.StringAttribute{MarkdownDescription: "Contact name, or a team name.", Computed: true},
					"email": schema.StringAttribute{MarkdownDescription: "Contact email address.", Computed: true},
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the deployment is active.",
				Computed:            true,
			},
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
				MarkdownDescription: "Whether this is a dedicated internet gateway, routing through private " +
					"egress IP addresses Jamf Security Cloud provisions. Mutually exclusive with an IPsec " +
					"configuration.",
				Computed: true,
			},
			"dedicated_egress_ip_addresses": schema.ListAttribute{
				MarkdownDescription: "The private egress IP addresses Jamf Security Cloud provisioned for a " +
					"dedicated internet gateway. Allocated within seconds of the gateway being created, well " +
					"before it finishes provisioning, so a populated list means the addresses are reserved " +
					"rather than that the gateway reports itself operational. Read `status` for that. Always " +
					"empty on an IPsec gateway.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"ipsec":    dsIPSecAttribute(),
			"status":   dsStatusAttribute(),
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// dsIPSecAttribute builds the read-only IPsec block for the data sources.
func dsIPSecAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "IPsec tunnel configuration. Null on a dedicated internet gateway.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"key_exchange_protocol": schema.StringAttribute{MarkdownDescription: "Key exchange protocol.", Computed: true},
			"phase_1":               dsCipherSuiteAttribute("Phase 1 cipher suite, protecting the key exchange."),
			"phase_2":               dsCipherSuiteAttribute("Phase 2 cipher suite, protecting the tunnelled traffic."),
			"jamf_side": schema.SingleNestedAttribute{
				MarkdownDescription: "The Jamf Security Cloud end of the tunnel.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"host":          schema.StringAttribute{MarkdownDescription: "Endpoint address.", Computed: true},
					"ike_domain_id": schema.StringAttribute{MarkdownDescription: "IKE identity Jamf Security Cloud presents.", Computed: true},
					"subnet":        schema.StringAttribute{MarkdownDescription: "Jamf-side encryption domain, in CIDR notation.", Computed: true},
					"auth_method":   schema.StringAttribute{MarkdownDescription: "Authentication method.", Computed: true},
				},
			},
			"customer_side": schema.SingleNestedAttribute{
				MarkdownDescription: "The customer end of the tunnel.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"host":          schema.StringAttribute{MarkdownDescription: "Your IPsec gateway address.", Computed: true},
					"ike_domain_id": schema.StringAttribute{MarkdownDescription: "IKE identity your concentrator presents.", Computed: true},
					"subnets": schema.ListAttribute{
						MarkdownDescription: "Subnets reachable through this gateway, in CIDR notation.",
						Computed:            true,
						ElementType:         types.StringType,
					},
					"vendor":      schema.StringAttribute{MarkdownDescription: "VPN vendor of your concentrator.", Computed: true},
					"auth_method": schema.StringAttribute{MarkdownDescription: "Authentication method.", Computed: true},
				},
			},
		},
	}
}

// dsCipherSuiteAttribute builds one read-only cipher-suite block.
func dsCipherSuiteAttribute(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"encryption":           schema.StringAttribute{MarkdownDescription: "Encryption algorithm.", Computed: true},
			"integrity":            schema.StringAttribute{MarkdownDescription: "Integrity algorithm.", Computed: true},
			"diffie_hellman_group": schema.StringAttribute{MarkdownDescription: "Diffie-Hellman group.", Computed: true},
			"sa_lifetime_seconds":  schema.Int64Attribute{MarkdownDescription: "Security association lifetime, in seconds.", Computed: true},
		},
	}
}

// dsStatusAttribute builds the read-only status block for the data sources.
func dsStatusAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Operational status Jamf Security Cloud reports for this gateway.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"state": schema.StringAttribute{
				MarkdownDescription: "Overall gateway state: `PENDING` while provisioning (**Pending** in the " +
					"Jamf Security Cloud admin UI), `UP` when the gateway reports itself operational " +
					"(**Active** in the admin UI), `DOWN` when unreachable or degraded, `DISABLED` when the " +
					"gateway is not enabled.",
				Computed: true,
			},
			"tunnel_state": schema.StringAttribute{MarkdownDescription: "IPsec tunnel health.", Computed: true},
		},
	}
}

// ConfigValidators enforces that exactly one of id or name is supplied.
func (d *GatewayDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *GatewayDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_gateway")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a gateway by ID or name and populates Terraform state.
func (d *GatewayDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data GatewayDataSourceModel
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
			"ZTNA gateway ID is empty",
			"`id` is set to an empty string, which ExactlyOneOf still counts as configured, so `name` cannot "+
				"be used instead. This usually means a variable or a reference resolved to \"\" — set `id` to "+
				"an ID, or remove it and set `name`.",
		)
		return
	}

	var gateway *securitycloud.Gateway
	var err error
	byName := data.ID.IsNull()
	if byName {
		gateway, err = d.client.ResolveZtnaGatewayV1ByName(readCtx, data.Name.ValueString())
	} else {
		gateway, err = d.client.GetZtnaGatewayV1(readCtx, data.ID.ValueString())
	}
	if err != nil {
		if ambiguous, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err); ok {
			resp.Diagnostics.AddError(
				"Multiple Jamf Security Cloud ZTNA gateways share that name",
				"Jamf Security Cloud does not require gateway names to be unique, and more than one gateway is "+
					"named "+data.Name.ValueString()+". Look the gateway up by `id` instead. Matching gateway "+
					"IDs: "+strings.Join(ambiguous.Matches, ", "),
			)
			return
		}
		if byName && helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Unable to find Jamf Security Cloud ZTNA gateway",
				"No ZTNA gateway on this tenant is named \""+data.Name.ValueString()+"\". Names are matched "+
					"exactly, so one differing only in capitalisation or surrounding whitespace will not be "+
					"found. Use the `jamfplatform_security_cloud_ztna_gateways` data source to list what exists.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to find Jamf Security Cloud ZTNA gateway", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignGatewayDataSourceModel(ctx, &data, gateway)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Security Cloud ZTNA gateway data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
