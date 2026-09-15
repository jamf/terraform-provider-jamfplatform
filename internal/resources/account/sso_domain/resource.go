// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package sso_domain implements the jamfplatform_account_sso_domain resource,
// data sources and list resource backed by the Jamf Account SSO API.
//
// An SSO domain is a DNS domain an organization claims in Jamf Account so that
// an SSO connection can be pointed at the identities under it. Claiming is not
// owning: Jamf mints a verification token at claim time, and ownership is proved
// by publishing that token as a TXT record and then running the verification —
// which is a separate action, because it depends on DNS the provider did not
// create and is refused for five minutes after any change to the claim.
//
// Family: Jamf Account / organization-level. Configure goes through
// providerdata.ConfigureAccount, which gates on ScopeOrganization alone. The
// namespace is served only from the US gateway.
//
// # Shape
//
// Create, read and delete only. Wire-probed 2026-09-02: a read, a full update and
// a merge update on a single domain's path all answer 403 BAD_PERMISSIONS, which
// by this repository's own law (STYLE_GUIDE §Jamf Security Cloud Resource Naming)
// means the route is unmapped rather than unprivileged. Three consequences run
// through the whole package:
//
//   - There is no read by identifier. A single claim is found by scanning the
//     organization's domain collection and matching on the domain name.
//   - There is no update. Every attribute is RequiresReplace and Update issues no
//     write, refreshing the read-only attributes only.
//   - Only an owned claim is manageable. The collection returns shared domains
//     — owned by another organization, assignable to a connection, refused every
//     change and withdrawal — alongside the organization's own, and matching on
//     the name alone would let an import adopt one. Read, Update and the list
//     resource all gate on `sharedDomain`; the data sources do not, because
//     reading a shared domain is exactly what they are for.
//   - Import is by domain name. The identifier is assigned by Jamf, is never
//     shown to a practitioner, and is not even stable — withdrawing a claim and
//     making it again mints a new one, along with a new verification key. This
//     diverges from STYLE_GUIDE §Import format ("avoid name-based imports; use
//     IDs only"), which assumes an identifier that both addresses the object and
//     survives; here neither holds.
//
// # Attribute names
//
// Attribute names follow the Jamf Account console rather than the payload
// wherever the two diverge, per STYLE_GUIDE §Attribute names mirror the Jamf Pro
// admin UI. Because the guide also forbids comments inside function bodies, the
// mapping lives here:
//
//	Terraform attribute         Payload field
//	-------------------------   --------------------------
//	verification_status         domainStatus
//	verification_txt_record     (derived from verificationKey)
//	parent_domain_id            verifiedTldId
//	shared                      sharedDomain
//	created_by                  createdByName
//	created_at                  createdDate
//	last_modified_at            lastModifiedDate
//	last_verified_at            lastVerificationDate
//	verification_expires_at     verificationExpirationDate
//
// Three of those renames are worth their churn. `verifiedTldId` is cryptic and
// also wrong — the referent is the verified parent registrable domain a subdomain
// inherits from, not a top-level domain. `domainStatus` would stutter on a
// resource already named for a domain, and it reports verification state only,
// whereas the console's STATUS column is a composite that also reflects
// connection usage. `sharedDomain` stutters the same way.
//
// The status *values* are deliberately left in Jamf's spelling instead of being
// translated to console labels; mappings.go records why.
package sso_domain

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// maxDomainLength bounds the domain name at the length a fully-qualified DNS name
// can take, so an obviously impossible value is refused at plan time rather than
// producing an unattributed refusal mid-apply.
const maxDomainLength = 253

// DomainResource implements the Terraform resource for Jamf Account SSO domains.
type DomainResource struct {
	client *account.Client
}

var (
	_ resource.Resource                = &DomainResource{}
	_ resource.ResourceWithImportState = &DomainResource{}
	_ resource.ResourceWithIdentity    = &DomainResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDomainResource returns a new instance of DomainResource.
func NewDomainResource() resource.Resource {
	return &DomainResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_domain"
}

// IdentitySchema defines the identifier used for import and list results.
//
// The domain name rather than the Jamf-assigned identifier: it is the only value
// a single claim can be looked up by, it is what a practitioner knows, and it
// survives a withdraw-and-reclaim that the identifier does not.
func (r *DomainResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"domain": identityschema.StringAttribute{
				Description:       "Domain name used to uniquely reference the claim.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the SSO domain resource.
func (r *DomainResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Claims a DNS domain for your Jamf Account organization, so that an SSO connection " +
			"can be pointed at the people who sign in with an address at that domain.\n\nClaiming a domain does " +
			"not prove you own it. Jamf Account mints a verification token when the claim is made, and the " +
			"domain stays unverified until a TXT record holding `verification_txt_record` is published at the " +
			"root of the domain and the check is run, either with the `jamfplatform_account_sso_domain_verify` " +
			"action or from the Jamf Account console. Jamf Account then re-checks a verified domain continuously " +
			"in the background, so leave the TXT record in place.\n\nA claim cannot be edited. Changing `domain` " +
			"replaces it, and the replacement is issued a fresh `verification_key`, so the TXT record has to be " +
			"republished before the new claim can verify.\n\nDestroying a claim also withdraws the domain from " +
			"every SSO connection that names it, which silently narrows those connections. The " +
			"`jamfplatform_account_sso_domain` data source reports which connections a domain is assigned " +
			"to.\n\nOnly a domain your own organization claimed can be managed here. A domain another Jamf " +
			"Account organization owns and shares with yours (`shared` is `true`) can be assigned to a " +
			"connection but cannot be changed or withdrawn, so this resource refuses it: an import of one is " +
			"rejected, and the `jamfplatform_account_sso_domain` data source is how a shared domain is " +
			"read.\n\nNeeds an organization-scoped API integration, created against neither a platform " +
			"environment nor a tenant, and is reachable only in the United States region." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier Jamf Account assigned to the claim. It is not stable, because " +
					"withdrawing a claim and making it again yields a different one, so reference a domain by " +
					"`domain` rather than by this.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "**\"Domain Name\"** in the Jamf Account console: the DNS domain to claim, " +
					"such as `example.com`. Lower case only, and a bare name: no scheme, no path, no port and no " +
					"user part. Changing it replaces the claim.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxDomainLength),
					DomainName(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"verification_status": schema.StringAttribute{
				MarkdownDescription: "Verification state of the claim: " + verificationStatusDocs() + ". The Jamf " +
					"Account console shows a composite status that also reflects whether a domain is in use by a " +
					"connection; this reports verification alone. Read-only.",
				Computed: true,
			},
			"verification_key": schema.StringAttribute{
				MarkdownDescription: "Token Jamf Account minted for this claim, published as the value of a TXT " +
					"record on the domain to prove ownership. Prefer `verification_txt_record`, which is the " +
					"complete record value. Read-only.",
				Computed: true,
			},
			"verification_txt_record": schema.StringAttribute{
				MarkdownDescription: "Complete TXT record value to publish at the root of the domain: the " +
					"`jamf-site-verification=` prefix followed by `verification_key`. This is what the Jamf Account " +
					"console offers behind its Copy button. Publish it with host `@` and a TTL of 86400, then run " +
					"the verification. Read-only.",
				Computed: true,
			},
			"parent_domain_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the verified parent domain a subdomain inherits its " +
					"verification from. Null for a domain verified in its own right. Inheritance is resolved by " +
					"Jamf Account and cannot be declared. Read-only.",
				Computed: true,
			},
			"shared": schema.BoolAttribute{
				MarkdownDescription: "Whether the domain is owned by another Jamf Account organization and shared " +
					"with yours. A shared domain can be assigned to a connection but cannot be changed or " +
					"withdrawn, so it cannot be managed by this resource. Importing one is refused, and it is " +
					"read with the `jamfplatform_account_sso_domain` data source instead. Always `false` for a " +
					"domain this resource manages. Read-only.",
				Computed: true,
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the Jamf account the domain belongs to. For a shared domain " +
					"this is the owning organization's account rather than yours. Read-only.",
				Computed: true,
			},
			"created_by": schema.StringAttribute{
				MarkdownDescription: "Name of the Jamf Account user who added the domain. Populated only for " +
					"domains added through the Jamf Account console. A claim Terraform makes has no user behind " +
					"it, so this is always null for a domain this provider creates. Read-only.",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the domain was claimed. Read-only.",
				Computed:            true,
			},
			"last_modified_at": schema.StringAttribute{
				MarkdownDescription: "When the claim last changed. Running the verification updates it whether " +
					"the check succeeded or not, and it is the point Jamf Account's five-minute wait between " +
					"verification attempts is measured from. Read-only.",
				Computed: true,
			},
			"last_verified_at": schema.StringAttribute{
				MarkdownDescription: "When ownership was last verified successfully. Null for a domain that has " +
					"never verified. Read-only.",
				Computed: true,
			},
			"verification_expires_at": schema.StringAttribute{
				MarkdownDescription: "When the current verification lapses: 14 days after the last successful " +
					"verification, or 14 days after the claim for a domain that has never verified. Running the " +
					"verification pushes it out again even when the check fails. Read-only.",
				Computed: true,
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

// Configure wires the Jamf Account client into the resource via the shared
// providerdata.ConfigureAccount helper.
func (r *DomainResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_domain")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by domain name.
//
// The Jamf-assigned identifier is deliberately not accepted: nothing reads a
// single claim by it, so an import carrying one would have to scan the collection
// for it anyway, and it changes whenever a claim is withdrawn and made again.
func (r *DomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}
