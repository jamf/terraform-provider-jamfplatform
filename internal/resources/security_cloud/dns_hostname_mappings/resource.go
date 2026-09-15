// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package dns_hostname_mappings implements the
// jamfplatform_security_cloud_dns_hostname_mappings resource and data source, backed
// by the Jamf Security Cloud custom DNS API.
//
// A hostname mapping points an internal host name at the IPv4 and IPv6 addresses it
// should resolve to, so that users reach internal resources while still being
// protected against mobile and network threats. Each mapping also chooses how its
// traffic is routed — through Zero Trust Network Access, through Secure DNS, or
// neither.
//
// Attribute names follow the admin UI rather than the wire, per STYLE_GUIDE
// §Attribute names mirror the Jamf Pro admin UI. Because the guide also forbids
// comments inside function bodies, the wire mapping lives here:
//
//	Terraform attribute                Wire field    UI label
//	--------------------------------   -----------   --------------------------
//	mappings                           (the body)    "Hostname mapping"
//	mappings[].hostname                hostname      "Insert hostname"
//	mappings[].ipv4_addresses          aRecords      "Insert IPv4" / "IPv4"
//	mappings[].ipv6_addresses          aaaaRecords   "Insert IPv6" / "IPv6"
//	mappings[].connect_to_ztna         ztna          "Connect to ZTNA"
//	mappings[].connect_to_secure_dns   secureDns     "Connect to Secure DNS"
//
// The two booleans stay flat rather than nested under the UI's "Traffic vectoring"
// heading: two scalars do not earn a SingleNestedAttribute, and the checkbox labels
// already carry the grouping.
//
// # Shape: one per tenant, full replace, with a real clear
//
// The endpoint holds the tenant's whole mapping set and there is no per-mapping
// route, so one resource owns the collection (STYLE_GUIDE §Full-replace endpoints).
// It follows §Singleton resources for its fixed helpers.SingletonID, identity schema,
// import validation and nil-client guards, and diverges on the same two points as
// the sibling search domain resource:
//
//  1. Delete is real. DELETE clears the set and answers 204, as does a PUT of an
//     empty array, so CheckDestroy asserts the set is empty rather than using the
//     inverted singleton contract.
//  2. Absence is observable — though differently from the search domain, which is a
//     reason not to copy either answer to the next construct. An empty mapping set
//     is a 200 carrying `{"totalCount":0,"results":[]}`, never a 404.
//
// # Wire truth, probed 2026-08-29 against production EU
//
// Several of these are not derivable from the SDK types, and one contradicts them:
//
//   - At least one of aRecords / aaaaRecords must be non-empty. The SDK types both
//     as *[]string with omitempty and documents them as nullable, but a mapping with
//     neither is refused — and the refusal always blames aRecords, even when
//     aaaaRecords is the list that was supplied. Hence EachMappingHasAnAddress,
//     whose diagnostic names both.
//   - Mapping order is not preserved. Sending z, a, m reads back m, a, z, stably
//     across repeated reads. Hence a Set.
//   - Duplicate addresses within one mapping are accepted and then silently deduped,
//     which is the second reason the address lists are Sets: a List would diff
//     forever against a configuration containing a duplicate.
//   - A duplicate hostname across two mappings answers 500 with an empty `errors`
//     array — nothing named, nothing to translate. Hence the plan-time uniqueness
//     check, which compares host names byte-exactly and must keep doing so: the
//     endpoint is case-sensitive, and `Case.example.com` alongside `case.example.com`
//     is accepted with both mappings stored. Folding the comparison would reject a
//     configuration the server takes.
//   - A trailing root dot is stripped on write. `Trail.Example.COM.` sent stores as
//     `Trail.Example.COM`; letter case is untouched, so the dot is the only transform.
//     The sibling search domain endpoint keeps the dot, so this is per construct
//     rather than a namespace rule. Because `mappings` is Required, Terraform compares
//     the configured value with the stored one and the stripped dot fails the apply
//     outright — hence NoTrailingRootDot refuses the form at plan time rather than the
//     provider normalising it, which would leave the plan and the state disagreeing
//     just the same.
//   - `dot.example.com.` alongside `dot.example.com` is the *same* name to this
//     endpoint and answers the same unattributable 500 as any other duplicate. That is
//     the second reason the dot-terminated form is refused at plan time: with it gone,
//     no configuration can reach that 500 through a difference the byte-exact
//     uniqueness check cannot see.
//   - Caps are 500 mappings, 10 IPv4 and 10 IPv6 addresses per mapping. The
//     per-mapping caps name their field; the 500 does not.
//   - Omitted address lists read back as `[]`, and omitted booleans as false. The
//     admin UI pre-checks "Connect to ZTNA" in its add dialog; the API does not.
//   - Host name letter case is stored exactly as written, not normalised.
//
// Absent and empty are the same thing for an address list, so the state builder
// writes null for an empty one and the schema refuses an explicitly empty set,
// pointing at omission instead. Accepting `[]` as distinct from absent would create a
// difference the wire cannot represent, and so a permanent diff.
//
// # Why both traffic-vectoring booleans are Required
//
// They started as Optional with booldefault.StaticBool(false), which is what the
// wire's own default suggests. That produces a permanent diff, and not for the reason
// one would guess: an attribute default inside a SetNestedAttribute overrides a value
// the configuration set explicitly. Verified against the live tenant on 2026-08-29
// with a single mapping — `connect_to_ztna = true` in the configuration planned as
// `false`, so every plan after the first proposed the same no-op change forever. It
// reproduces with one element, so it is not the set-element correlation problem that
// usually explains this shape.
//
// Making them Required removes the mechanism rather than working around it, and costs
// little: the admin UI shows both checkboxes on every mapping, so both always have a
// state to state. Do not reintroduce a Default here without re-verifying that plan
// against a real tenant — the unit tests pass either way.
//
// The two address lists are Optional and this is safe: an Optional attribute with
// neither Computed nor Default set is untouched by that mechanism, verified in the
// same session by a configuration that omits ipv6_addresses on some mappings and
// plans clean.
//
// # Create refuses to clobber, but adopts its own interrupted write
//
// A full replace reports no conflict, so nothing on the wire separates "creating the
// tenant's mappings" from "silently discarding the ones an administrator added by
// hand". Create reads first and refuses when the stored set differs from the planned
// one, pointing the operator at import — or, for the other cause the refusal has, at
// removing a second instance of this one-per-tenant resource.
//
// A stored set that already *equals* the plan is adopted rather than refused, because
// that is what an interrupted create leaves behind: Create writes and then reads the
// result back, and a failure of that confirming read leaves the tenant configured with
// no Terraform state to show it. See storedMappingsMatchPlan.
package dns_hostname_mappings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Collection bounds enforced by Jamf Security Cloud, wire-probed on 2026-08-29. The
// per-mapping caps are refused with 400 LIST_SIZE_EXCEEDED naming the field and the
// bound; the 500-mapping cap is refused with the same code and no field at all,
// which is the one that most needs catching at plan time.
const (
	minMappings  = 1
	maxMappings  = 500
	minAddresses = 1
	maxAddresses = 10
)

// HostnameMappingsResource implements the Terraform resource for Jamf Security Cloud
// custom hostname mappings.
type HostnameMappingsResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                = &HostnameMappingsResource{}
	_ resource.ResourceWithImportState = &HostnameMappingsResource{}
	_ resource.ResourceWithIdentity    = &HostnameMappingsResource{}
)

const (
	defaultCreateTimeout = 120 * time.Second
	defaultReadTimeout   = 120 * time.Second
	defaultUpdateTimeout = 120 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewHostnameMappingsResource returns a new instance of HostnameMappingsResource.
func NewHostnameMappingsResource() resource.Resource {
	return &HostnameMappingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *HostnameMappingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_hostname_mappings"
}

// IdentitySchema defines the identifier used for import. The endpoint holds one
// mapping set per tenant, so the identifier is always helpers.SingletonID.
func (r *HostnameMappingsResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Always \"singleton\", since a tenant holds one set of hostname mappings.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the hostname mappings resource.
func (r *HostnameMappingsResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages **\"Hostname mapping\"** under Custom DNS in the Jamf Security Cloud admin " +
			"UI: custom IPv4 and IPv6 mappings for internal host names your organization uses, so that users " +
			"reach internal resources while staying protected from mobile and network threats.\n\n" +
			"This resource owns the tenant's **entire** set of hostname mappings: a mapping added elsewhere and " +
			"absent from this configuration is removed on the next apply. There is one set per tenant, so only " +
			"one instance of this resource should exist in your configuration, and destroying it removes every " +
			"mapping." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton`, since a tenant holds one set of hostname mappings.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"mappings": schema.SetNestedAttribute{
				MarkdownDescription: "The hostname mappings for this tenant. Between 1 and 500 entries. To " +
					"remove them all, destroy the resource rather than emptying this collection. Each host name " +
					"may appear only once.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"hostname": schema.StringAttribute{
							MarkdownDescription: "**\"Insert hostname\"** in the Jamf Security Cloud admin UI: " +
								"the fully qualified host name this mapping applies to. Up to 253 characters. " +
								"Wildcards are not accepted, and letter case is stored exactly as written. Write " +
								"the name without a trailing dot: Jamf Security Cloud stores the name without " +
								"one, so a configuration carrying one could never match the stored value.",
							Required: true,
							Validators: []validator.String{
								commonvalidators.DNSHostname(),
								NoTrailingRootDot(),
							},
						},
						"ipv4_addresses": schema.SetAttribute{
							MarkdownDescription: "**\"Insert IPv4\"** in the Jamf Security Cloud admin UI: the " +
								"IPv4 addresses the host name resolves to. Up to 10 entries. Omit the attribute " +
								"when there are none; an empty collection is not accepted. At least one of " +
								"`ipv4_addresses` or `ipv6_addresses` must be set.",
							Optional:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeBetween(minAddresses, maxAddresses),
								setvalidator.ValueStringsAre(commonvalidators.IPv4Address()),
							},
						},
						"ipv6_addresses": schema.SetAttribute{
							MarkdownDescription: "**\"Insert IPv6\"** in the Jamf Security Cloud admin UI: the " +
								"IPv6 addresses the host name resolves to. Up to 10 entries. Omit the attribute " +
								"when there are none; an empty collection is not accepted. At least one of " +
								"`ipv4_addresses` or `ipv6_addresses` must be set.",
							Optional:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeBetween(minAddresses, maxAddresses),
								setvalidator.ValueStringsAre(IPv6Address()),
							},
						},
						"connect_to_ztna": schema.BoolAttribute{
							MarkdownDescription: "**\"Connect to ZTNA\"** under Traffic vectoring in the Jamf " +
								"Security Cloud admin UI: whether this host name's traffic is routed through " +
								"Zero Trust Network Access. Set it explicitly on every mapping. The admin UI's " +
								"add dialog pre-selects this checkbox, so a mapping added there and one written " +
								"here do not start from the same value.",
							Required: true,
						},
						"connect_to_secure_dns": schema.BoolAttribute{
							MarkdownDescription: "**\"Connect to Secure DNS\"** under Traffic vectoring in the " +
								"Jamf Security Cloud admin UI: whether this host name's traffic is routed " +
								"through Secure DNS. Set it explicitly on every mapping.",
							Required: true,
						},
					},
				},
				Validators: []validator.Set{
					setvalidator.SizeBetween(minMappings, maxMappings),
					commonvalidators.UniqueStringFieldSet("hostname"),
					EachMappingHasAnAddress(),
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
func (r *HostnameMappingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_hostname_mappings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import of the tenant's single hostname mapping set.
//
// The identifier must be helpers.SingletonID. Normalising anything else silently
// would hide a mis-typed import behind a Read that happens to succeed, since the
// endpoint takes no identifier and would return the tenant's mappings whatever was
// typed.
//
// helpers.ImportSingletonState is what accepts the identifier from either import
// form: `terraform import` and an `id =` import block fill req.ID, while an
// `identity = { id = … }` block leaves it empty and puts the value in
// req.Identity. This resource advertises an IdentitySchema, so it has to honour
// both.
//
//	terraform import jamfplatform_security_cloud_dns_hostname_mappings.<name> singleton
func (r *HostnameMappingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_security_cloud_dns_hostname_mappings")
}
