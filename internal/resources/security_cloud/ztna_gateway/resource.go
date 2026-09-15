// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ztna_gateway implements the jamfplatform_security_cloud_ztna_gateway
// resource, data sources and list resource backed by the Jamf Security Cloud
// ZTNA gateway API.
//
// A dedicated gateway is the tenant's own egress point into Jamf Security Cloud.
// It comes in exactly two forms, which the admin UI presents as two separate
// sections and the API distinguishes by which of two mutually exclusive fields is
// set:
//
//   - a **dedicated IPsec gateway**, which builds a tunnel to the customer's own
//     VPN concentrator, and
//   - a **dedicated internet gateway**, which routes to the internet through a
//     pair of private egress IPs Jamf provisions.
//
// The provider models the choice as the presence or absence of the `ipsec` block
// rather than as a separate discriminator attribute. That is not a shortcut: the
// API requires exactly one of `ipsec` or the dedicated-egress-IP flag
// ("Gateway must be configured as dedicated (dedicatedIps.enabled=true) or
// ipsec."), refuses both together, and there is no configuration in which the
// flag carries information the `ipsec` block does not already imply. Deriving it
// removes a whole class of config that could only ever be rejected.
//
// Attribute names follow the admin UI rather than the wire wherever the two
// diverge, per STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI. Because
// the guide also forbids comments inside function bodies, the wire mapping lives
// here rather than beside each attribute — this table is where to look when
// searching for a field name seen in an API response:
//
//	Terraform attribute                    Wire field
//	------------------------------------   --------------------------------
//	egress_region                          datacenter
//	ipsec_source_ip_addresses              availabilityZones
//	dedicated_egress_ip_addresses          dedicatedIps.ips
//	ipsec.key_exchange_protocol            ipsec.keyExchange
//	ipsec.phase_1                          ipsec.ike
//	ipsec.phase_2                          ipsec.esp
//	ipsec.phase_N.diffie_hellman_group     ipsec.{ike,esp}.dhGroups
//	ipsec.phase_N.sa_lifetime_seconds      ipsec.{ike,esp}.lifetimeInSec
//	ipsec.jamf_side                        ipsec.left
//	ipsec.customer_side                    ipsec.right
//	ipsec.*.ike_domain_id                  ipsec.{left,right}.id
//	ipsec.jamf_side.subnet                 ipsec.left.subnets (single element)
//	ipsec.jamf_side.authentication_secret  ipsec.left.secret
//
// Two of those are worth a word. `left`/`right` is strongSwan jargon that appears
// nowhere a Jamf administrator would see — the Encryption Domain step labels the
// two ends "Jamf Security Cloud side" and "Customer side". And `availabilityZones`
// is not merely different from its label but actively misleading: the SDK itself
// notes that despite the name, the values are IPv4 addresses rather than zone
// identifiers.
//
// The IPsec form this resource builds is Jamf's **Custom IPSec** gateway. The admin
// UI offers a second kind, **Quick Connect IPSec** — a Linux VM Jamf's documentation
// walks you through building — and it is not expressible here: its create form takes
// no cipher suites, no encryption-domain subnets and no IKE domain IDs, while this
// schema requires `phase_1`, `phase_2`, `jamf_side` and `customer_side`. Anyone
// looking for Quick Connect in Terraform is looking for something the API surface
// behind this resource does not offer.
//
// Enumerated attributes take the admin UI's labels too, translated to stored
// values at the boundary; see mappings.go for the tables and their provenance.
package ztna_gateway

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// GatewayResource implements the Terraform resource for Jamf Security Cloud ZTNA
// gateways.
type GatewayResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                     = &GatewayResource{}
	_ resource.ResourceWithImportState      = &GatewayResource{}
	_ resource.ResourceWithIdentity         = &GatewayResource{}
	_ resource.ResourceWithConfigValidators = &GatewayResource{}
)

// Default operation budgets.
//
// Create and update are ten minutes rather than the two the rest of the namespace
// uses, because both wait for the gateway to report itself operational and that
// wait dominates the call. The 2026-08-31 probe recorded 275 seconds to `UP` on a
// create and 295 on an egress-region change (see waitForGatewayState), so ten minutes
// is roughly twice the observed time and the probe's own twenty-minute cap was
// never approached. A region that provisions more slowly than the one measured is
// the case the `timeouts` block exists for — raise `create` or `update` there
// rather than assuming these defaults fit every region.
//
// One figure that must not be read into this: Jamf's "Creating a Dedicated Internet
// Gateway" page says "Provisioning takes up to two business days". In context that is
// the paid add-on being enabled for the account by the Jamf Account Team, not the
// per-gateway build — the same page has the gateway itself become ready "after a
// short period", which matches the 275 seconds measured. Ten minutes is sized against
// the per-gateway build. Nothing Terraform does can wait out an account entitlement,
// and stretching these defaults towards two days would only turn a missing
// entitlement into a two-day hang.
//
// Read and delete do not wait for anything and keep the namespace's defaults.
const (
	defaultCreateTimeout = 600 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 600 * time.Second
	defaultDeleteTimeout = 120 * time.Second
)

// NewGatewayResource returns a new instance of GatewayResource.
func NewGatewayResource() resource.Resource {
	return &GatewayResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *GatewayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_gateway"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *GatewayResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Gateway ID used to uniquely reference the gateway.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators enforces the cross-field rules the API applies to the IPsec
// block, at plan time rather than mid-apply.
func (r *GatewayResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		ipsecSourceAddressesValidator{},
	}
}

// Schema returns the Terraform schema for the ZTNA gateway resource.
func (r *GatewayResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a dedicated Jamf Security Cloud ZTNA gateway, the tenant's own egress " +
			"point into Jamf Security Cloud. A custom DNS zone's name servers and a ZTNA app's routing are " +
			"reachable through it.\n\nA gateway takes one of two forms, chosen by whether the `ipsec` block is " +
			"present:\n\n- **Dedicated IPsec gateway**: set `ipsec` to build a tunnel to your own VPN " +
			"concentrator.\n- **Dedicated internet gateway**: omit `ipsec` to route to the internet through a " +
			"pair of private egress IP addresses Jamf Security Cloud provisions, reported in " +
			"`dedicated_egress_ip_addresses`.\n\nThe form is fixed for the life of the gateway: Jamf Security " +
			"Cloud refuses to convert one into the other, so adding or removing `ipsec` replaces the gateway. " +
			"Deleting a gateway that a custom DNS zone or a grouped gateway still references is also refused; " +
			"drop the reference in a separate apply first." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Gateway ID assigned by Jamf Security Cloud.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Gateway name\"** in the Jamf Security Cloud admin UI.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"egress_region": egressRegionAttribute(),
			"contact": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Contact name\"** and **\"Contact email\"** in the Jamf Security Cloud " +
					"admin UI: who Jamf Security Cloud should reach about this gateway's operation.",
				Required: true,
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: "Contact name, or a team name.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"email": schema.StringAttribute{
						MarkdownDescription: "Contact email address.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the deployment is active. A disabled gateway reports its status as " +
					"`DISABLED` and carries no traffic. Defaults to `true`. Disabling a gateway reports " +
					"`PENDING` for a few seconds before it settles, so an apply that disables one waits for " +
					"`DISABLED` in the same way an apply that enables one waits for `UP`. Either way the " +
					"status recorded is the settled one, not the transient.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"tenant_ids": schema.SetAttribute{
				MarkdownDescription: "IDs of the tenants granted access to this gateway. At least one, and every " +
					"one must belong to the same organization as the credentials the provider is configured with; " +
					"a tenant outside it is refused.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"ipsec_source_ip_addresses": schema.SetAttribute{
				MarkdownDescription: "**\"Jamf Security Cloud IPsec source IP addresses\"** in the Jamf Security " +
					"Cloud admin UI: the addresses IPsec traffic from Jamf Security Cloud originates from, which " +
					"your firewall must allow. Supply both addresses your egress region offers for dynamic " +
					"addressing, or one to pin a single source address. Only valid on an IPsec gateway: a dedicated " +
					"internet gateway must leave this unset.\n\n" +
					"The accepted addresses are fixed per egress region and are the ones the admin UI lists when " +
					"you pick the region. The provider does not check them at plan time because the accepted set " +
					"is not published anywhere it can read.\n\n" +
					"Taking the attribute out of your configuration clears the addresses on the next update " +
					"rather than leaving them alone.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(commonvalidators.IPv4Address()),
				},
			},
			"dedicated_egress_ip_addresses": schema.ListAttribute{
				MarkdownDescription: "The private egress IP addresses Jamf Security Cloud provisions for a " +
					"dedicated internet gateway. Allocated within seconds of the gateway being created, roughly " +
					"four and a half minutes before it finishes provisioning, so a populated list means the " +
					"addresses are reserved rather than that the gateway reports itself operational. Read " +
					"`status` for that. Always empty on an IPsec gateway, confirmed against Jamf Security Cloud " +
					"on 2026-08-31. Read-only.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"ipsec":  ipsecAttribute(),
			"status": statusAttribute(),
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// egressRegionAttribute builds the egress-region attribute.
//
// The provider treats the region as updatable, which diverges from Jamf's own
// documentation. The "Creating a Dedicated Internet Gateway" page states flatly that
// "The egress region cannot be changed once the gateway is created". Wire-probed
// against production EU on 2026-08-31: a request changing only the region was
// accepted with a 204 and the gateway re-provisioned, taking new dedicated egress
// addresses in the new region. So the documented prohibition does not hold, and the
// operation works.
//
// Keeping it updatable is the lesser risk. Marking it RequiresReplace to match the
// documentation would destroy and recreate a paid gateway — surrendering its
// dedicated IP addresses back to the account's allotment, and refusing the destroy
// outright while anything still references it — to avoid an in-place change that has
// been observed to succeed. The description below therefore says what the provider
// does and what was observed, and does not claim Jamf supports it.
func egressRegionAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "**\"Egress region\"** in the Jamf Security Cloud admin UI: the region this " +
			"gateway is deployed to.\n\n" +
			"Changing it re-provisions the gateway in the new region: connectivity drops and the reported " +
			"status returns to `PENDING`. Any dedicated egress IP addresses are replaced in place rather than " +
			"cleared (measured at around 35 seconds after the change), so for a short window the list is " +
			"non-empty, entirely plausible and still the old region's. The apply waits for the gateway to " +
			"report itself operational again, which covers that window.\n\n" +
			"The published documentation states the egress region cannot be changed once a gateway is " +
			"created. The provider allows the change because it was observed to be accepted and to " +
			"re-provision the gateway, and because replacing the gateway instead would surrender its " +
			"dedicated IP addresses. Treat a region change as disruptive.\n\n" +
			"Valid values: " + markdownList(egressRegionValues()) + ".",
		Required: true,
		Validators: []validator.String{
			stringvalidator.OneOf(egressRegionValues()...),
		},
	}
}

// ipsecAttribute builds the IPsec tunnel block. Its presence selects the gateway
// form, and Jamf Security Cloud refuses to change the form of an existing gateway
// (`GATEWAY_TYPE_CHANGE_NOT_SUPPORTED`), so adding or removing the block has to
// replace the resource rather than attempt an update that cannot succeed.
func ipsecAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "IPsec tunnel configuration. Present on a dedicated IPsec gateway, absent on a " +
			"dedicated internet gateway. Adding or removing the whole block replaces the gateway.",
		Optional: true,
		PlanModifiers: []planmodifier.Object{
			objectplanmodifier.RequiresReplaceIf(
				requiresReplaceOnIpsecPresenceChange,
				"Adding or removing the `ipsec` block changes the gateway's form, which Jamf Security Cloud does not support in place.",
				"Adding or removing the `ipsec` block changes the gateway's form, which Jamf Security Cloud does not support in place.",
			),
		},
		Attributes: map[string]schema.Attribute{
			"key_exchange_protocol": schema.StringAttribute{
				MarkdownDescription: "**\"Key exchange protocol\"** in the Jamf Security Cloud admin UI. Valid " +
					"values: " + markdownList(keyExchangeValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(keyExchangeValues()...),
				},
			},
			"phase_1": cipherSuiteAttribute(
				"**\"Phase 1\"** in the Jamf Security Cloud admin UI: the cipher suite protecting the key exchange itself.",
			),
			"phase_2": cipherSuiteAttribute(
				"**\"Phase 2\"** in the Jamf Security Cloud admin UI: the cipher suite protecting the tunnelled traffic.",
			),
			"jamf_side":     jamfSideAttribute(),
			"customer_side": customerSideAttribute(),
		},
	}
}

// cipherSuiteAttribute builds one cipher-suite block. Each of the three
// algorithm fields is a single value, not a list: the wire shape is an array but
// the server rejects anything other than exactly one element, so a list would
// offer users only invalid extra room.
func cipherSuiteAttribute(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"encryption": schema.StringAttribute{
				MarkdownDescription: "**\"Encryption\"** in the Jamf Security Cloud admin UI. Valid values: " +
					markdownList(encryptionValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(encryptionValues()...),
				},
			},
			"integrity": schema.StringAttribute{
				MarkdownDescription: "**\"Integrity\"** in the Jamf Security Cloud admin UI. Valid values: " +
					markdownList(integrityValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(integrityValues()...),
				},
			},
			"diffie_hellman_group": schema.StringAttribute{
				MarkdownDescription: "**\"Diffie-Hellman Group\"** in the Jamf Security Cloud admin UI. Valid " +
					"values: " + markdownList(diffieHellmanGroupValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(diffieHellmanGroupValues()...),
				},
			},
			"sa_lifetime_seconds": schema.Int64Attribute{
				MarkdownDescription: "**\"Security Association (SA) Lifetime\"** in the Jamf Security Cloud admin " +
					"UI, in seconds.",
				Required: true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
		},
	}
}

// jamfSideAttribute builds the Jamf-side tunnel endpoint — the wire's `left`.
func jamfSideAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"Jamf Security Cloud side\"** in the Jamf Security Cloud admin UI: the endpoint " +
			"Jamf Security Cloud presents to your VPN concentrator.",
		Required: true,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Endpoint address, or `%any` to accept any address.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"ike_domain_id": schema.StringAttribute{
				MarkdownDescription: "**\"Jamf Security Cloud IKE domain ID\"** in the Jamf Security Cloud admin " +
					"UI: the IKE identity Jamf Security Cloud presents, for example `wpa.wandera.com`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"subnet": schema.StringAttribute{
				MarkdownDescription: "**\"Jamf Security Cloud subnet\"** in the Jamf Security Cloud admin UI: the " +
					"range all end-user traffic originates from through the tunnel, in CIDR notation. Must be a " +
					"private range: `10.0.0.0/8` with a `/8`–`/30` prefix, `172.16.0.0/12` with `/12`–`/30`, or " +
					"`192.168.0.0/16` with `/16`–`/30`. The range must not exist anywhere else on your network.",
				Required: true,
				Validators: []validator.String{
					privateCIDR(),
				},
			},
			"authentication_secret": schema.StringAttribute{
				MarkdownDescription: "**\"Authentication secret\"** in the Jamf Security Cloud admin UI: the " +
					"IPsec pre-shared key, applied to both ends of the tunnel. Write-only. It is sent to Jamf " +
					"Security Cloud on writes and never held in Terraform state, because Jamf Security Cloud " +
					"never returns it. Pair with `authentication_secret_wo_version` to rotate it. It can be " +
					"rotated but not cleared.",
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"authentication_secret_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the write-only `authentication_secret`. Bump this integer to " +
					"force an update that re-sends the secret. Set it to `1` on create. Leaving it unset or " +
					"unchanged means \"leave the stored key alone\", so the provider omits the secret from the " +
					"next update and Jamf Security Cloud retains the existing one.",
				Optional: true,
			},
			"auth_method": schema.StringAttribute{
				MarkdownDescription: "Authentication method Jamf Security Cloud reports for this endpoint. " +
					"Read-only.",
				Computed: true,
			},
		},
	}
}

// customerSideAttribute builds the remote-peer endpoint — the wire's `right`.
func customerSideAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "**\"Customer side\"** in the Jamf Security Cloud admin UI: your own VPN " +
			"concentrator, and the subnets reachable through it.",
		Required: true,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "**\"Your IPsec gateway IP address\"** in the Jamf Security Cloud admin UI.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"ike_domain_id": schema.StringAttribute{
				MarkdownDescription: "**\"Your IKE domain ID\"** in the Jamf Security Cloud admin UI: the IKE " +
					"identity your concentrator presents.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"subnets": schema.SetAttribute{
				MarkdownDescription: "**\"Customer subnets\"** in the Jamf Security Cloud admin UI: the subnets " +
					"reachable through this gateway, in CIDR notation, usually where your applications live. At " +
					"least one. `0.0.0.0/0` is accepted, and narrowing access on the firewall instead is the " +
					"documented approach for most firewalls.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(cidrBlock()),
				},
			},
			"vendor": schema.StringAttribute{
				MarkdownDescription: "**\"IPsec network vendor\"** in the Jamf Security Cloud admin UI: the VPN " +
					"vendor of your concentrator. Case-sensitive. Valid values: " + markdownList(vendorValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(vendorValues()...),
				},
			},
			"auth_method": schema.StringAttribute{
				MarkdownDescription: "Authentication method Jamf Security Cloud reports for this endpoint. " +
					"Read-only.",
				Computed: true,
			},
		},
	}
}

// statusAttribute builds the read-only status block.
//
// The wire also carries a `updatedAt` timestamp, which this deliberately does not
// expose. It advances every time Jamf Security Cloud re-evaluates the gateway,
// so surfacing it would make every single refresh report the object as changed
// outside Terraform — noise about a value no configuration can act on. `state`
// and `tunnel_state` do settle once provisioning finishes, and both are worth
// reading, so they stay.
//
// The values are surfaced as the wire spells them, which is a deliberate exception
// to STYLE_GUIDE §"Attribute names mirror the Jamf Pro admin UI" — the same rule
// that has mappings.go translate every other enumerated value on this resource into
// its admin-UI label.
//
// All four labels are known from reading the admin UI directly on 2026-08-31:
// "Pending", "Available", "Down" and "Disabled". Three of the four are the wire value
// in sentence case; `UP` is the sole genuine rename. The IPsec and internet sections
// of the gateway page use the same vocabulary, so these are not per-form labels.
// Note that Jamf's own "Creating a Dedicated Internet Gateway" page disagrees with
// the second — it says "after a short period, the status changes to Active". The
// admin UI is what an operator actually reads, so "Available" is what these
// descriptions name; the page is stale. This is the second documented-behaviour
// divergence on this resource, the other being egress-region immutability, so prefer
// an observation to that page wherever the two conflict.
//
// The values are still surfaced as the wire spells them rather than remapped through
// mappings.go, and now that the mapping is complete that is a decision rather than a
// gap. Only `UP` differs materially from its label, and this is a read-only status
// snapshot: an operator reading it in state may equally be comparing it against an API
// response or a support conversation, so the wire value earns its place, with the
// label named alongside. Remapping one value of four would also leave the enum half
// translated. Nothing here should be read as "the mapping was never finished".
func statusAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Operational status Jamf Security Cloud reports for this gateway. Read-only, and " +
			"live: a create or an egress-region change starts the gateway at `PENDING` (**Pending** in the " +
			"Jamf Security Cloud admin UI), and it settles to `UP`, shown as **Available**, once the " +
			"infrastructure is provisioned.\n\n" +
			"For a dedicated internet gateway the provider waits for the status to settle before finishing a " +
			"create or an update: for `UP` when the gateway is enabled, and for `DISABLED` when it is not. " +
			"State therefore records the settled status rather than the `PENDING` both transitions pass " +
			"through. An update that does not re-provision the gateway, such as a name or contact change, " +
			"finds it already settled and waits for nothing. If a wait runs out first the apply still " +
			"succeeds, with a warning naming the status reached, and the status settles on a later refresh.\n\n" +
			"Creating an IPsec gateway waits for `DOWN` as readily as `UP`, because `UP` is not yet within " +
			"reach: the tunnel's other end is configured from values this resource returns, so the " +
			"concentrator, its NAT rules and its firewall openings all come afterwards. `DOWN` is the settled " +
			"status until the tunnel is established, and it is what state records rather than the `PENDING` " +
			"the gateway passes through first. If you configured your side in advance the gateway may reach " +
			"`UP` directly, which is accepted just the same. Updating one waits for `UP`, since by then the " +
			"tunnel may be established; if it is not, the apply gives up once the gateway has reported `DOWN` " +
			"for a minute and warns, rather than holding for the whole budget.\n\n" +
			"`UP` means the gateway reports itself operational. It is a necessary condition for traffic to " +
			"flow, not a guarantee of it.",
		Computed: true,
		Attributes: map[string]schema.Attribute{
			"state": schema.StringAttribute{
				MarkdownDescription: "Overall gateway state: `PENDING` while provisioning (**Pending** in the " +
					"Jamf Security Cloud admin UI), `UP` when the gateway reports itself operational " +
					"(**Available** in the admin UI), `DOWN` when unreachable or degraded, `DISABLED` when " +
					"`enabled` is `false` (**Disabled** in the admin UI). `DOWN` is **Down** in the admin " +
					"UI. Every transition drifts through `PENDING` for a few seconds, which is why an apply " +
					"waits for the settled value rather than recording the first status it reads.",
				Computed: true,
			},
			"tunnel_state": schema.StringAttribute{
				MarkdownDescription: "IPsec tunnel health, `UP` or `DOWN`. Null on a dedicated internet gateway, " +
					"and on an IPsec gateway until the first tunnel report arrives.",
				Computed: true,
			},
		},
	}
}

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *GatewayResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_gateway")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Security Cloud gateway ID.
//
// The IPsec pre-shared key cannot come back on import — Jamf Security Cloud never
// returns it — so an imported IPsec gateway needs `ipsec.jamf_side.authentication_secret`
// supplied in configuration before the next apply will succeed.
func (r *GatewayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
