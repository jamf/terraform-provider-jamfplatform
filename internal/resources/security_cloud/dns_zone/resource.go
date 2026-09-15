// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package dns_zone implements the jamfplatform_security_cloud_dns_zone resource,
// data sources and list resource backed by the Jamf Security Cloud custom DNS
// API.
//
// A custom DNS zone is Jamf Security Cloud's split-brain DNS configuration:
// hostnames under the zone's domains resolve via authoritative name servers the
// customer nominates rather than the public resolvers, which is what makes
// private applications reachable over ZTNA.
//
// Attribute names follow the admin UI rather than the wire wherever the two
// diverge, per STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI. Because
// the guide also forbids comments inside function bodies, the wire mapping lives
// here:
//
//	Terraform attribute                        Wire field
//	----------------------------------------   -------------------------
//	authoritative_name_servers                 nameServers
//	authoritative_name_servers[].ip_address    nameServers[].ip
//	authoritative_name_servers[].gateway_id    nameServers[].gatewayId
//
// `gateway_id` is the one place the attribute does not take the UI's label. The UI
// calls the column "Reachable via" and fills it with gateway names, but that phrase
// describes a relationship rather than naming a field, and `reachable_via = "a7d2"`
// would say nothing about what the value is. The ID rather than the name is
// deliberate too: gateway names are not unique, and an ID is what a reference to a
// sibling gateway resource yields.
package dns_zone

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Collection bounds enforced by Jamf Security Cloud. Wire-probed against
// production EU on 2026-08-27: an empty or over-long collection is refused with
// 400 LIST_SIZE_EXCEEDED naming exactly these bounds, so they are checked at plan
// time rather than surfacing mid-apply.
const (
	minDomains        = 1
	maxDomains        = 100
	minNameServers    = 1
	maxNameServers    = 20
	maxZoneNameLength = 100
)

// DNSZoneResource implements the Terraform resource for Jamf Security Cloud
// custom DNS zones.
type DNSZoneResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                = &DNSZoneResource{}
	_ resource.ResourceWithImportState = &DNSZoneResource{}
	_ resource.ResourceWithIdentity    = &DNSZoneResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDNSZoneResource returns a new instance of DNSZoneResource.
func NewDNSZoneResource() resource.Resource {
	return &DNSZoneResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DNSZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_zone"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *DNSZoneResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "DNS zone ID used to uniquely reference the zone.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the DNS zone resource.
func (r *DNSZoneResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Security Cloud custom DNS zone. Hostnames belonging to the zone's domains " +
			"are resolved through authoritative name servers of your choice instead of public DNS. That arrangement is " +
			"\"split-brain DNS\", and a custom DNS zone is required before enterprise apps on internal private networks " +
			"become reachable over ZTNA.\n\n" +
			"Misconfiguring a zone can cut end users off from some or all of your private applications and workloads.\n\n" +
			"See the [Jamf Security Cloud guide](../guides/security-cloud) for the rules a name server address has " +
			"to satisfy, how to choose the gateway each one is reached through, and why the gateway has to exist " +
			"before the zone that names it." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Zone ID assigned by Jamf Security Cloud.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Zone name\"** in the Jamf Security Cloud admin UI. Up to 100 characters. " +
					"Zone names are not required to be unique, so prefer the zone ID when referencing a zone elsewhere.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxZoneNameLength),
				},
			},
			"domains": schema.SetAttribute{
				MarkdownDescription: "**\"Domains\"** in the Jamf Security Cloud admin UI: the domains that match this " +
					"zone. Subdomains take a wildcard, and the parent domain must be listed explicitly alongside it: " +
					"`company.com` covers only the parent domain and `*.company.com` covers only the subdomains. " +
					"Between 1 and 100 entries. A domain already claimed by another zone is rejected.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeBetween(minDomains, maxDomains),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"authoritative_name_servers": schema.SetNestedAttribute{
				MarkdownDescription: "**\"Authoritative name servers\"** in the Jamf Security Cloud admin UI: the name " +
					"servers that resolve hostnames for this zone's domains. Between 1 and 20 entries. Each name server " +
					"must be reachable via the gateway it is paired with.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip_address": schema.StringAttribute{
							MarkdownDescription: "**\"Name server IP address\"** in the Jamf Security Cloud admin UI. " +
								"An IPv4 address in dotted-quad form; IPv6 is not accepted. Each IP address may appear " +
								"only once in a zone, even when paired with different gateways. Jamf Security Cloud also " +
								"refuses reserved ranges such as private and loopback addresses.",
							Required: true,
							Validators: []validator.String{
								commonvalidators.IPv4Address(),
							},
						},
						"gateway_id": schema.StringAttribute{
							MarkdownDescription: "**\"Reachable via\"** in the Jamf Security Cloud admin UI: the ID of " +
								"the gateway this name server is reachable through. Accepts a Jamf-managed shared " +
								"gateway (\"Nearest Data Center\" or one of the shared IP pools) or one of your own " +
								"ZTNA gateways. The gateway must already exist: a zone referencing an unknown gateway " +
								"is refused.",
							Required: true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
				Validators: []validator.Set{
					setvalidator.SizeBetween(minNameServers, maxNameServers),
					commonvalidators.UniqueStringFieldSet("ip_address"),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *DNSZoneResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_zone")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Security Cloud DNS zone ID.
func (r *DNSZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
