// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Changing any attribute replaces the connection, because Jamf Account has no
// working update endpoint.
//
// PUT /sso/v1/connections/{id} answers 500 UPSTREAM_ERROR for every request —
// wire-verified 2026-09-03 with the exact body a create accepts, with the stored
// name, with a fresh name, and at an identifier that does not exist, while POST
// and DELETE both work. The refused write applies nothing: a connection read back
// after a PUT changing three fields was byte-identical. So no provider-side
// handling can rescue an in-place change, and planning one would fail during
// apply. plan_modifiers.go turns every change into a replacement instead, and is
// the file to delete when Jamf fixes the endpoint.
//
// **A replacement interrupts sign-in.** Terraform destroys before it creates by
// default, so the connection is absent for the width of that gap and users on its
// domains cannot authenticate through it. Anyone editing a connection carrying
// real traffic wants `create_before_destroy` — Jamf allows two connections on one
// domain, so the new one can exist before the old one goes. The resource cannot
// set that itself; it is the practitioner's lifecycle block, and the attribute
// descriptions and the example both say so.
//
// Package sso_connection implements the jamfplatform_account_sso_connection
// resource, data sources and list resource backed by the Jamf Account SSO API.
//
// An SSO connection is one identity provider an organization signs people in
// through, pointed at the verified domains whose addresses it is authoritative
// for and enabled for the Jamf products those people should reach. Four provider
// families are supported, and the family is fixed for the life of the connection.
//
// Family: Jamf Account / organization-level. Configure goes through
// providerdata.ConfigureAccount, which gates on ScopeOrganization alone. The
// namespace is served only from the US gateway.
//
// # The update endpoint is broken upstream, and this package is written for the fix
//
// PUT /sso/v1/connections/{id} answers 500 UPSTREAM_ERROR for every request, on
// every region, for both organizations probed — including the verbatim body a
// create accepts, and including an identifier that does not exist. The body is
// still inspected: a PUT carrying `{}` is refused 400 FIELD_VALIDATION naming
// connectionType, enabledProducts, domains and connection, so the fault is in
// applying the change rather than in reaching the endpoint. Creates, reads,
// withdrawals and the sibling domain writes on the same namespace are all
// healthy, which localises the fault to the update path. Raised with Jamf; the
// identifier to quote is in the spike.
//
// A create's own 500 UPSTREAM_ERROR is a different animal and must not be read as
// the same fault: it is an overloaded catch-all standing in for several problems
// an operator can put right. Observed causes are a domain that is not claimed, a
// domain claimed but not verified, a required settings value missing, a
// connectionType disagreeing with the settings sent, a name carrying anything but
// letters and digits, and the organization sitting at the number of connections
// Jamf allows it — that last one pinned by repetition, since an identical body
// answered 201 at twenty-four connections, 500 at twenty-five, and 201 again after
// one withdrawal. helpers.go's create diagnostic lists them rather than blaming
// Jamf.
//
// Four consequences run through this package. Each behaviour that could only be
// settled by a successful update is marked "spec-derived, not wire-verified" at
// the point it is relied on, so the set is greppable when the fault clears:
//
//   - An update sends the complete settings, because the specification says an
//     omitted field is cleared rather than left alone — spec-derived, not
//     wire-verified.
//   - An omitted client secret is understood to leave the stored secret in
//     place, which is the one documented exception to that replacement —
//     spec-derived, not wire-verified.
//   - The provider family and the hosting region force a replacement, because
//     the specification says an update cannot move a connection to another of
//     either — spec-derived, not wire-verified.
//   - An empty collection is understood to clear rather than to leave alone, so
//     an emptied `domains` or `enabled_products` is sent as an empty collection
//     rather than omitted — spec-derived, not wire-verified. Only the create side
//     of it was probed, where Jamf refuses an empty `domains` and accepts an
//     empty `enabled_products`; what either does to an existing connection was
//     never observed.
//
// One thing the probe did settle: Jamf appends a uniquifying suffix to the name it
// is sent. `tfReviewMin` came back stored as
// `tfReviewMin-jqxld7tl4m454ed7s35647nmjssypo`, and eighteen of the twenty-two
// connections read carry such a suffix. That is why `name` and `internal_name` are
// two attributes rather than one: a single Optional attribute echoing the stored
// value back would give every suffixed connection a difference on every plan. So
// `name` holds what was configured and an ordinary refresh never overwrites it,
// while `internal_name` holds what Jamf stores whole.
//
// # Two connections that cannot be managed
//
// A connection built through the console's Microsoft admin-consent flow reads
// back cleanly but cannot be written again — it has no client of its own, and
// nothing in the payload can express the consent. Importing one would land a
// resource that no apply could reconcile, so Read and Update refuse it and the
// list resource drops it. Withdrawal is still allowed, so an operator holding one
// in state from an earlier provider version can destroy it rather than being
// stuck.
//
// A second class is a bookkeeping disagreement inside Jamf: at least one
// connection is returned by the collection read and answers 404 on its own
// identifier. It exists — the collection is Jamf's own store — so a Read that
// meets it must not treat the 404 as a withdrawal and drop the resource. It is
// reported instead, and the list resource drops it because it could never be
// imported.
//
// # What a read cannot tell you
//
// A complete refresh takes two calls. The read of one connection returns the
// per-provider settings but no products and no consent ticket; the collection
// read returns the products and the ticket but no per-provider settings.
//
// Neither returns the tenants or the platform environments a product is enabled
// for. So `enabled_products` and `enabled_environments` are
// configuration-authoritative: they are written, never read back, never
// refreshed, and carry no difference detection at all. `enabled_product_names`
// is the partial signal that does come back. `managed_account_id` inside those
// blocks is in the same position and worse: it is a write-only field of a
// write-only collection, so nothing this provider can read would reveal that a
// connection points at tenants a Jamf partner manages on a customer's behalf.
// That is stated in the attribute descriptions and in the guide because it cannot
// be enforced in code.
//
// # Attribute names
//
// Attribute names follow the Jamf Account console rather than the payload
// wherever the two diverge, per STYLE_GUIDE §Attribute names mirror the Jamf Pro
// admin UI. Because the guide also forbids comments inside function bodies, the
// mapping lives here:
//
//	Terraform attribute            Payload field
//	----------------------------   ------------------------------------------
//	name                           connection.name          (configured value)
//	internal_name                   name                     (stored value)
//	hosting_region                 region
//	auth_method                    tokenEndpointAuthMethod
//	pkce                           pkceAuthType
//	sync_attributes_at_login       syncUserProfileAttributesAtLogin
//	omit_login_hint                aliasLoginHintToIdp      (inverted)
//	attribute_map                  attributeMap
//	group_name_filter              groupNameFilter          (a document)
//	session_duration_minutes       sessionInfo.maxSessionTimeInMinutes
//	inactivity_timeout_minutes     sessionInfo.maxInactivityTimeInMinutes
//	enabled_products[].tenants     enabledProducts[].enabledTenants
//	enabled_environments[].environments
//	                               enabledEnvironments[].enabledEnvironments
//	enabled_product_names          enabledApplications
//	entra.get_user_groups          groups
//	entra.include_nested_groups    nestedGroups
//	google_workspace.get_user_groups
//	                               groups
//	google_workspace.extended_groups
//	                               extendedGroups
//	google_workspace.enable_users_api
//	                               apiEnableUsers
//
// Two of those are traps rather than preferences.
//
// `omit_login_hint` is the inverse of `aliasLoginHintToIdp`. The console renders
// a checkbox labelled "Omit login_hint IdP parameter", so a ticked box is
// `aliasLoginHintToIdp: false`. Carrying the payload's spelling would be faithful
// and actively misleading: an operator copying the console would set it true to
// mean "omit" and get the opposite behaviour, with nothing to catch it — Jamf
// validates none of it and the symptom is a quiet sign-in oddity rather than an
// error. The inversion happens in both builders and is pinned in both directions
// by unit tests, because a pair that inverts twice is indistinguishable from one
// that never inverts and no schema test would notice.
//
// `group_name_filter` is a structured document, `{"op":…,"groups":"<comma
// separated>"}`, and the console renders it as two controls. A single string
// attribute would push document assembly and comma-joining onto the operator for
// a value with a known, tiny shape. Note that an operator with no groups is the
// real-world "no filter" and is distinct from the field being absent, which is
// why the block's `groups` is required and an empty set does not collapse to
// nothing.
//
// # Value vocabularies
//
// `connection_type`, `auth_method` and `pkce` are renamed to the console's
// vocabulary; `hosting_region` and `product` keep Jamf's, and mappings.go says
// why for each. Every accepted set is derived from the SDK's own generated
// values, so a vocabulary Jamf changes fails a test rather than drifting.
//
// # One console value deliberately absent
//
// The console shows a callback address for each connection, and it is what an
// operator pastes into their provider. It appears nowhere in the Jamf Account
// data — not on a connection, not on any settings object — and is evidently
// derived from the hosting region. Deriving it here would mean publishing a value
// Jamf never returned, and a change to the pattern would silently break sign-in
// for anyone who trusted it. Read it from the console instead; the guide says so.
package sso_connection

import (
	"context"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// maxSessionMinutes is the ceiling the Jamf Account console states for both
// session limits, a day in minutes. Whether Jamf enforces it was never probed, so
// this is a console-derived bound rather than a wire-verified one. It only ever
// refuses a configuration: both attributes are Optional rather than read-only, so
// a stored value above the ceiling is still adopted by a refresh.
const maxSessionMinutes = 1440

// nameMaxLength bounds the connection name generously, so that an obviously
// impossible value is refused at plan time rather than producing an
// unattributable refusal mid-apply. Jamf publishes no limit.
const nameMaxLength = 255

// nameAllowedPattern is the character set Jamf Account accepts in a connection
// name: letters and digits, nothing else.
//
// Wire-established 2026-09-02, and it is not in the specification or the SDK. A
// name carrying a hyphen is refused with an unattributed 500 UPSTREAM_ERROR
// naming no field, indistinguishable from the several other faults that share
// that response — so validating it here is the difference between a plan-time
// message and an opaque failure mid-apply.
//
// Note the suffix Jamf appends to the stored name contains a hyphen itself, so
// the constraint applies to what is sent, not to what comes back.
var nameAllowedPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// ConnectionResource implements the Terraform resource for Jamf Account SSO
// connections.
type ConnectionResource struct {
	client *account.Client
}

var (
	_ resource.Resource                = &ConnectionResource{}
	_ resource.ResourceWithImportState = &ConnectionResource{}
	_ resource.ResourceWithIdentity    = &ConnectionResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewConnectionResource returns a new instance of ConnectionResource.
// ConnectionResource forces replacement on any change while Jamf Account's update
// endpoint is unavailable. See plan_modifiers.go, which is temporary.
var _ resource.ResourceWithModifyPlan = &ConnectionResource{}

func NewConnectionResource() resource.Resource {
	return &ConnectionResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ConnectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_connection"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ConnectionResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Identifier used to uniquely reference the connection.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the SSO connection resource.
func (r *ConnectionResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an SSO connection for your Jamf Account organization: one identity " +
			"provider the people in your organization sign in through, and the Jamf products they reach with " +
			"it.\n\nPick the provider family with `connection_type` and supply the matching settings block: " +
			"exactly one of `generic_oidc`, `entra`, `okta` or `google_workspace`, and it has to be the one the " +
			"family names. Neither the family nor `hosting_region` can be changed afterwards. Both replace the " +
			"connection.\n\nEvery domain in `domains` must already be claimed *and* verified by your " +
			"organization; a connection cannot be created without at least one. Use " +
			"`jamfplatform_account_sso_domain` to claim a domain and the " +
			"`jamfplatform_account_sso_domain_verify` action to prove it, and depend on the verification so the " +
			"ordering is explicit.\n\n**Jamf Account cannot currently apply a change to an existing " +
			"connection.** Every attempt is refused with an internal failure, in every region, even when what is " +
			"sent is exactly what Jamf Account accepted when the connection was created, so it is not something " +
			"a configuration change can work around. That is why this resource replaces a connection rather than " +
			"editing one: any change you make destroys it and creates a new one, which interrupts sign-in, so " +
			"give a connection carrying real traffic a `create_before_destroy` lifecycle block. Creating, " +
			"reading, listing and destroying all work normally. The fault is in Jamf Account and has been " +
			"reported; until it clears, edit a connection in the Jamf Account console.\n\nTwo kinds of " +
			"connection cannot be managed here at all. One built with Microsoft's admin-consent flow in the " +
			"console has no client of its own and cannot be written back, so importing one is refused. And a " +
			"connection your organization's collection lists but which cannot be read on its own identifier is " +
			"reported rather than treated as gone: it exists, and Terraform must not drop it from " +
			"state.\n\n`enabled_products` and `enabled_environments` are configuration-authoritative. Nothing " +
			"Jamf Account returns echoes the tenants back, so Terraform cannot notice a change made outside it " +
			"and cannot recover them on import. `enabled_product_names` reports the products alone, which is the " +
			"only part that comes back.\n\nThe connection's callback address is not exposed: the console shows " +
			"one, Jamf Account's data does not carry it, and deriving it here would mean publishing a value that " +
			"could silently go wrong. Copy it from the console.\n\nNeeds an organization-scoped API integration, " +
			"created against neither a platform environment nor a tenant, and is reachable only in the United " +
			"States region." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier Jamf Account assigned to the connection. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Connection name\"** in the Jamf Account console: the name to give the " +
					"connection, and the name the console displays. Letters and digits only: Jamf Account " +
					"rejects anything else without saying which field was at fault, so this is checked before " +
					"the plan is applied.\n\nJamf Account does not require connection names to be unique, and " +
					"appends a suffix of its own to whichever name you choose. So two connections created with " +
					"the same name both exist, and the console shows both under that one name. Only " +
					"`internal_name` and `id` tell them apart. Give each connection a distinct name unless you " +
					"mean to have duplicates.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, nameMaxLength),
					stringvalidator.RegexMatches(nameAllowedPattern,
						"must contain only letters and digits; Jamf Account rejects a name with any other character"),
				},
			},
			"internal_name": schema.StringAttribute{
				MarkdownDescription: "Internal name Jamf Account stores for the connection. Jamf Account appends " +
					"a suffix of its own to the name you choose, and this is the result. The console does not " +
					"show it, listing `name` instead, so this is the only place two connections sharing a name " +
					"can be told apart. Read-only.",
				Computed: true,
			},
			"connection_type": schema.StringAttribute{
				MarkdownDescription: "**\"Connection type\"** in the Jamf Account console: the identity provider " +
					"family. One of " + markdownValueList(connectionTypeValues()) + ". Choose the family that " +
					"matches your provider; `generic_oidc` is for any OpenID Connect provider Jamf Account has " +
					"no purpose-built integration with. The matching settings block is required and no other may " +
					"be set. A connection cannot be moved to another family, so changing this replaces it.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(connectionTypeValues()...),
					ConnectionSettings(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosting_region": schema.StringAttribute{
				MarkdownDescription: "**\"Hosting region\"** in the Jamf Account console: the region your provider " +
					"details are held in and your sign-in traffic is routed through. One of " +
					markdownValueList(account.RegionValues()) + ". The console says this cannot be changed after " +
					"the connection is created, so changing it replaces the connection. This is unrelated to " +
					"the region of the `base_url` you configure the provider with.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(account.RegionValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"auth_method": schema.StringAttribute{
				MarkdownDescription: "**\"Connection auth method\"** in the Jamf Account console: how Jamf " +
					"Account proves itself to your provider when it redeems an authorization code. One of " +
					markdownValueList(authMethodValues()) + ". With `client_secret` you supply `client_secret`; " +
					"with `private_key_jwt` Jamf Account holds a key of its own and there is no shared secret to " +
					"supply, so `client_secret` is refused. The console offers no choice for `google_workspace`, " +
					"so this is refused there. Defaults to `client_secret`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(authMethodValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "**\"Client ID\"** in the Jamf Account console: the identifier of the " +
					"application you registered with your provider. Required for every family except an Entra " +
					"connection using `entra.use_common_endpoint`, where a multi-tenant application needs none.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "**\"Client secret\"** in the Jamf Account console: the secret of the " +
					"application you registered with your provider. Never held in Terraform state and never " +
					"returned by Jamf Account, so Terraform cannot tell whether the stored secret still matches " +
					"this value. Leave it out to keep the stored secret as it is; supply it to set or rotate it, " +
					"and bump `client_secret_wo_version` in the same change so Terraform knows a rotation was " +
					"asked for.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
			},
			"client_secret_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for `client_secret`. Because a write-only value is not held " +
					"in state, changing the secret alone produces no difference for Terraform to act on. Bump " +
					"this whole number to make it rotate.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("client_secret")),
				},
			},
			"scopes": schema.StringAttribute{
				MarkdownDescription: "**\"Scopes\"** in the Jamf Account console: the OAuth scopes Jamf Account " +
					"asks your provider for, separated by spaces exactly as the console shows them. `openid` is " +
					"required, and a `groups` scope is needed if you want group memberships passed through; the " +
					"console's default is `openid email profile`. Required for `generic_oidc` and `okta`, " +
					"optional for `google_workspace`, and refused for `entra`, which takes none.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"pkce": schema.StringAttribute{
				MarkdownDescription: "**\"PKCE configuration\"** in the Jamf Account console: the Proof Key for " +
					"Code Exchange method used with your provider. One of " + markdownValueList(pkceValues()) +
					", where `auto` lets Jamf Account pick. The console offers this only for `generic_oidc` and " +
					"`okta`, so it is refused for the other two families. Defaults to `disabled`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(pkceValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"send_nonce": schema.BoolAttribute{
				MarkdownDescription: "**\"Send nonce\"** in the Jamf Account console: whether a nonce is sent on " +
					"the authentication request. Leave it alone unless your provider requires one. Defaults to " +
					"`false`.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"sync_attributes_at_login": schema.BoolAttribute{
				MarkdownDescription: "**\"Sync at each login\"** in the Jamf Account console: whether a person's " +
					"profile details are refreshed from your provider every time they sign in. Defaults to " +
					"`true`, which is what every connection read carried.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"omit_login_hint": schema.BoolAttribute{
				MarkdownDescription: "**\"Omit `login_hint` IdP parameter\"** in the Jamf Account console: " +
					"whether the address someone typed at Jamf Account is withheld from your provider, so they " +
					"type it again there. Leave it `false` for the smoother sign-in; set it `true` if your " +
					"provider mishandles the hint. Defaults to `false`.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"custom_username_claim_name": schema.StringAttribute{
				MarkdownDescription: "**\"Custom username claim name\"** in the Jamf Account console: the claim to " +
					"read a username from when your provider does not place it in the standard one, so people are " +
					"matched by an identifier such as a user principal name instead of an email address. " +
					"Typically `upn` or `nickname`, and needs Jamf Pro 11.20.0 or later where Jamf Pro is one of " +
					"the enabled products.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"username_domain": schema.StringAttribute{
				MarkdownDescription: "Domain appended to a bare username from your provider to form the person's " +
					"email address. No console control was observed for this; leave it out unless your provider " +
					"returns usernames without a domain.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"attribute_map": schema.StringAttribute{
				MarkdownDescription: "How claims from your provider are mapped onto Jamf Account user details, " +
					"as a JSON object string. Author it with `jsonencode({ ... })`. Formatting and key order do " +
					"not force the connection to be replaced, because the replacement decision compares the " +
					"value as JSON. Reindenting it does still show as an in-place change, which Jamf Account's " +
					"update endpoint currently refuses.\n\nEvery connection read carried one, in one of three " +
					"shapes: `{\"mapping_mode\":\"bind_all\"}`, `{\"mapping_mode\":\"basic_profile\"}`, or " +
					"`{\"mapping_mode\":\"use_map\", \"userinfo_scope\":\"…\", \"attributes\":{…}}` whose values " +
					"are claim templates. There is no published schema for this and Jamf Account validates " +
					"nothing here, so a mode this provider does not recognise is a warning rather than an error. " +
					"A value that is not a JSON object is refused, though, because Jamf Account stores it and " +
					"quietly ignores it. Jamf Account populates a default when you leave it out.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					AttributeMap(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"group_name_filter": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"IdP group name filter\"** in the Jamf Account console: which of your " +
					"provider's groups are passed through to Jamf Account, for a directory holding more groups " +
					"than Jamf Account needs. Leave the whole block out to send no filter at all; supply it with " +
					"an empty `groups` set to send an empty filter, which is a different thing and the shape " +
					"most connections carry.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"operator": schema.StringAttribute{
						MarkdownDescription: "The console's AND/OR toggle beside the group list: `or` passes a " +
							"group matching any entry, `and` requires every entry. A group matches an entry when " +
							"its own name contains it.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.OneOf(filterOperatorValues()...),
						},
					},
					"groups": schema.SetAttribute{
						MarkdownDescription: "Group names to filter on. A group is passed through when its own " +
							"name **contains** one of these, so `Engineering` also passes " +
							"`Non-Engineering-Contractors`. Give the whole name if you mean an exact list. An " +
							"empty set is meaningful and means no filtering; a name may not contain a comma, " +
							"which is how Jamf Account separates them.",
						Required:    true,
						ElementType: types.StringType,
						Validators: []validator.Set{
							setvalidator.ValueStringsAre(
								stringvalidator.LengthAtLeast(1),
								FilterGroupName(),
							),
						},
					},
				},
			},
			"session_duration_minutes": schema.Int64Attribute{
				MarkdownDescription: "**\"Session duration (minutes)\"** in the Jamf Account console: how long a " +
					"session lasts before the person signs in again, however active they are. Leave it out to " +
					"use the Jamf Account default. Respected by Jamf Pro, Jamf Protect and Jamf Account.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.Between(1, maxSessionMinutes),
				},
			},
			"inactivity_timeout_minutes": schema.Int64Attribute{
				MarkdownDescription: "**\"Inactivity timeout (minutes)\"** in the Jamf Account console: how long " +
					"a session survives without activity. Leave it out to use the Jamf Account default.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.Between(1, maxSessionMinutes),
				},
			},
			"domains": schema.SetAttribute{
				MarkdownDescription: "**\"Associated Domains\"** in the Jamf Account console: the domain names " +
					"this connection signs people in for, such as `example.com`. At least one is required, and " +
					"every one must already be claimed and verified by your organization: an unverified or " +
					"unclaimed name is refused. A domain belongs to one organization and Jamf Account stores it " +
					"in lower case, so give lower-case names.\n\nDestroying a `jamfplatform_account_sso_domain` " +
					"also withdraws that domain from every connection naming it, quietly narrowing this set. The " +
					"`jamfplatform_account_sso_domain` data source reports which connections a domain is " +
					"assigned to.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(
						stringvalidator.LengthAtLeast(1),
						DomainName(),
					),
				},
			},
			"enabled_products": schema.SetNestedAttribute{
				MarkdownDescription: "**\"Applications\"** in the Jamf Account console: the Jamf products, and " +
					"the tenants of those products, this connection may be used to sign in to. Jamf Account " +
					"itself is always enabled and is not a choice, so it need not be listed. Leave the whole set " +
					"out, or give an empty one, to enable no tenant-scoped product.\n\nThis set is " +
					"configuration-authoritative and blind to outside change. Nothing Jamf Account returns " +
					"echoes the tenants back, so Terraform cannot notice a change made in the console, cannot " +
					"recover this on import, and will always plan whatever you configure. " +
					"`enabled_product_names` reports the products alone, which is the only part that does come " +
					"back. Nothing in this provider can list an organization's tenant identifiers either, so " +
					"they have to be copied from the Jamf Account console.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"product": schema.StringAttribute{
							MarkdownDescription: "The Jamf product. One of " + productDocs() + ".",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(productValues()...),
							},
						},
						"tenants": schema.SetAttribute{
							MarkdownDescription: "Identifiers of the product's tenants this connection applies to. " +
								"Leave it out or give an empty set for a product scoped to the whole organization " +
								"rather than to a tenant, such as `ACCOUNT`.",
							Optional:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
							},
						},
						"managed_account_id": schema.StringAttribute{
							MarkdownDescription: "Set this only when the tenants belong to an account you manage " +
								"on someone else's behalf as a Jamf partner, rather than to your own " +
								"organization. Nothing Jamf Account returns reveals it, so Terraform cannot tell " +
								"you that a connection points at a managed account. This attribute is write-only " +
								"in practice, like the block that holds it.",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
			},
			"enabled_environments": schema.SetNestedAttribute{
				MarkdownDescription: "Products scoped by platform environment rather than by tenant, and the " +
					"environments of them this connection applies to. Leave it out unless a product needs it; " +
					"there is no published list of which do. Configuration-authoritative and blind to outside " +
					"change, exactly as `enabled_products` is, and for the same reason.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"product": schema.StringAttribute{
							MarkdownDescription: "The Jamf product. One of " + productDocs() + ".",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(productValues()...),
							},
						},
						"environments": schema.SetAttribute{
							MarkdownDescription: "Identifiers of the product's platform environments this " +
								"connection applies to.",
							Required:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
								setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
							},
						},
						"managed_account_id": schema.StringAttribute{
							MarkdownDescription: "Set this only when the environments belong to an account you " +
								"manage on someone else's behalf as a Jamf partner. Nothing Jamf Account returns " +
								"reveals it.",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
			},
			"enabled_product_names": schema.SetAttribute{
				MarkdownDescription: "The Jamf products Jamf Account reports this connection as enabled for. This " +
					"is the only part of `enabled_products` that can be read back, never the tenants, so it is " +
					"a partial signal rather than a full picture. Read-only.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"ticket_url": schema.StringAttribute{
				MarkdownDescription: "Address of the Google Workspace administrator consent request for this " +
					"connection, where one is outstanding. Null for every other family and for a Google " +
					"connection needing no consent. Read-only.",
				Computed: true,
			},
			"consent_flow": schema.BoolAttribute{
				MarkdownDescription: "Whether the connection authenticates through Microsoft's admin-consent " +
					"flow rather than through a client you registered. Managed by Jamf Account and only ever set " +
					"up in its console: such a connection has no client of its own and cannot be written back, " +
					"so this resource refuses to manage one. Read-only.",
				Computed: true,
			},
			"easy_config": schema.BoolAttribute{
				MarkdownDescription: "Whether the connection was built by Jamf Account's guided setup rather than " +
					"configured directly. Managed by Jamf Account. Read-only.",
				Computed: true,
			},
			"generic_oidc": schema.SingleNestedAttribute{
				MarkdownDescription: "Settings for a connection to any OpenID Connect provider Jamf Account has " +
					"no purpose-built integration with. Required when `connection_type` is `generic_oidc`, and " +
					"refused otherwise.\n\nThe console fills the three addresses in for you by reading your " +
					"provider's discovery document and lets you adjust them afterwards. It does that in the " +
					"browser, so this provider cannot: supply all three yourself, copying them from your " +
					"provider's discovery document at `<issuer_url>/.well-known/openid-configuration`.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"issuer_url": schema.StringAttribute{
						MarkdownDescription: "**\"Issuer URL\"** in the Jamf Account console: your provider's " +
							"issuer, exactly as it appears in the tokens it signs.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"authorization_endpoint": schema.StringAttribute{
						MarkdownDescription: "Address people are redirected to in order to sign in. Taken from your " +
							"provider's discovery document as `authorization_endpoint`.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"token_endpoint": schema.StringAttribute{
						MarkdownDescription: "Address an authorization code is exchanged at. Taken from your " +
							"provider's discovery document as `token_endpoint`.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"jwks_uri": schema.StringAttribute{
						MarkdownDescription: "Address your provider publishes its signing keys at. Taken from your " +
							"provider's discovery document as `jwks_uri`.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"user_info_endpoint": schema.StringAttribute{
						MarkdownDescription: "Address profile details are read from. Leave it out when your " +
							"provider returns everything Jamf Account needs in the identity token itself.",
						Optional: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
				},
			},
			"entra": schema.SingleNestedAttribute{
				MarkdownDescription: "Settings for a Microsoft Entra connection. Required when `connection_type` " +
					"is `entra`, and refused otherwise.\n\n" +
					"This is the manually-configured form. Microsoft's admin-consent flow, the \"Connect with " +
					"Microsoft\" button in the Jamf Account console, is an interactive browser handshake with no " +
					"unattended equivalent and cannot be set up here.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"domain": schema.StringAttribute{
						MarkdownDescription: "**\"Microsoft Entra AD domain\"** in the Jamf Account console: the " +
							"primary domain of your Entra tenant. Real connections carry every shape here: an " +
							"`onmicrosoft.com` host, a plain company domain, a bare tenant identifier, and a full " +
							"Microsoft sign-in address including the tenant identifier. Nothing is checked beyond " +
							"it not being empty, deliberately, because anything stricter would refuse a working " +
							"value.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"tenant_domain": schema.StringAttribute{
						MarkdownDescription: "**\"Tenant domain\"** in the Jamf Account console: the domain " +
							"identifying the Entra tenant to authenticate against. Often the same as `domain` but " +
							"not reliably so, and it takes the same range of shapes, so it is not checked either.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"use_common_endpoint": schema.BoolAttribute{
						MarkdownDescription: "**\"Use common endpoint\"** in the Jamf Account console: whether " +
							"Microsoft's multi-tenant sign-in address is used instead of your tenant's own. Turn " +
							"it on for a multi-tenant application registration, in which case `client_id` may be " +
							"left out. Defaults to `false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"identity_api": schema.StringAttribute{
						MarkdownDescription: "**\"Identity API\"** in the Jamf Account console: the Microsoft " +
							"identity platform version the connection uses. One of " +
							markdownValueList(account.EntraIdentityApiValues()) + ". Defaults to " +
							"`MICROSOFT_IDENTITY_PLATFORM_V2`, which every Entra connection read carried.",
						Optional: true,
						Computed: true,
						Validators: []validator.String{
							stringvalidator.OneOf(account.EntraIdentityApiValues()...),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"max_groups": schema.Int64Attribute{
						MarkdownDescription: "**\"Max number of groups to retrieve\"** in the Jamf Account " +
							"console: the most groups read for one person. Entra truncates group claims on a large " +
							"directory, so raising this only helps as far as Entra will go. Defaults to `250`, " +
							"which every Entra connection read carried.",
						Optional: true,
						Computed: true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseNonNullStateForUnknown(),
						},
					},
					"set_emails_verified": schema.BoolAttribute{
						MarkdownDescription: "**\"Always set email verified to 'true'\"** in the Jamf Account " +
							"console: whether addresses from Entra are treated as already confirmed, so people " +
							"are not asked to confirm them again. Defaults to `true`, matching the console.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"enable_users_api": schema.BoolAttribute{
						MarkdownDescription: "**\"Enable users API\"** in the Jamf Account console: whether " +
							"Microsoft Graph is queried for details the token does not carry. Your application " +
							"registration has to hold the matching Graph permission. Defaults to `false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"use_wsfed": schema.BoolAttribute{
						MarkdownDescription: "Whether WS-Federation is used instead of OpenID Connect. Leave it " +
							"`false` unless your tenant requires it. Defaults to `false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"groups_scope": schema.StringAttribute{
						MarkdownDescription: "The Microsoft Graph permission groups are read with. One of " +
							markdownValueList(account.EntraGroupsScopeValues()) + ". `GROUP_READ_ALL` is the " +
							"narrower of the two and the right choice for a new connection; `DIRECTORY_READ_ALL` " +
							"is kept for existing ones. Only meaningful with `get_user_groups`, so it is refused " +
							"without it.",
						Optional: true,
						Validators: []validator.String{
							stringvalidator.OneOf(account.EntraGroupsScopeValues()...),
						},
					},
					"extended_profile": schema.BoolAttribute{
						MarkdownDescription: "**\"Extended profile\"** in the Jamf Account console: whether a " +
							"person's extended profile details are read from Entra when they sign in. Defaults to " +
							"`false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"get_user_groups": schema.BoolAttribute{
						MarkdownDescription: "**\"Get user groups\"** in the Jamf Account console: whether a " +
							"person's Entra group memberships are passed through to Jamf Account. Defaults to " +
							"`false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"include_nested_groups": schema.BoolAttribute{
						MarkdownDescription: "**\"Include all groups the user is a member of, including child " +
							"groups\"** in the Jamf Account console. Only meaningful with `get_user_groups`, so " +
							"setting it `true` without that is refused. Defaults to `false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"basic_profile": schema.BoolAttribute{
						MarkdownDescription: "**\"Basic profile\"** in the Jamf Account console: whether a " +
							"person's basic profile is read from Entra when they sign in. The console shows it " +
							"ticked and greyed out because it is always on, so it is reported here rather than " +
							"offered as a choice. Read-only.",
						Computed: true,
					},
				},
			},
			"okta": schema.SingleNestedAttribute{
				MarkdownDescription: "Settings for an Okta connection. Required when `connection_type` is " +
					"`okta`, and refused otherwise. Only the org domain is yours to set; Jamf Account works the " +
					"four addresses out from it.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"domain": schema.StringAttribute{
						MarkdownDescription: "**\"Okta domain\"** in the Jamf Account console: your Okta org " +
							"domain, without a scheme, such as `example.okta.com`.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
							BareHost(),
						},
					},
					"issuer_url": schema.StringAttribute{
						MarkdownDescription: "Issuer of your Okta authorization host, worked out from `domain`. " +
							"Read-only.",
						Computed: true,
					},
					"authorization_endpoint": schema.StringAttribute{
						MarkdownDescription: "Address people are redirected to in order to sign in, worked out from " +
							"`domain`. Read-only.",
						Computed: true,
					},
					"token_endpoint": schema.StringAttribute{
						MarkdownDescription: "Address an authorization code is exchanged at, worked out from " +
							"`domain`. Read-only.",
						Computed: true,
					},
					"jwks_uri": schema.StringAttribute{
						MarkdownDescription: "Address Okta publishes its signing keys at, worked out from " +
							"`domain`. Read-only.",
						Computed: true,
					},
				},
			},
			"google_workspace": schema.SingleNestedAttribute{
				MarkdownDescription: "Settings for a Google Workspace connection. Required when " +
					"`connection_type` is `google_workspace`, and refused otherwise.\n\nNo live Google Workspace " +
					"connection was available anywhere while this was built, so treat this block as provisional: " +
					"every field comes from the documented shape and from the console, and none of it has been " +
					"seen round-tripped.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"domain": schema.StringAttribute{
						MarkdownDescription: "**\"Google Workspace domain\"** in the Jamf Account console: the " +
							"primary domain of your Google Workspace account.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
							BareHost(),
						},
					},
					"get_user_groups": schema.BoolAttribute{
						MarkdownDescription: "**\"Groups\"** in the Jamf Account console: whether a person's " +
							"Google Workspace group memberships are passed through to Jamf Account. Defaults to " +
							"`false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"extended_groups": schema.BoolAttribute{
						MarkdownDescription: "Whether groups are read through the Google Directory rather than from " +
							"the token, which returns groups the token leaves out. Defaults to `false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"enable_users_api": schema.BoolAttribute{
						MarkdownDescription: "**\"Enable user API\"** in the Jamf Account console: whether the " +
							"Google Directory is queried for details the token does not carry. You have to turn " +
							"the Admin SDK on in the Google console and allow access for each Google Workspace " +
							"domain. Defaults to `false`.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
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

// Configure wires the Jamf Account client into the resource via the shared
// providerdata.ConfigureAccount helper.
func (r *ConnectionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_connection")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by connection identifier.
//
// Unlike the sibling SSO domain construct, which imports by name because its
// identifier is neither readable nor stable, a connection is read by identifier
// and keeps it — so this follows STYLE_GUIDE §Import format. The stored name
// would be the wrong key anyway: Jamf may hold a uniquified form of whatever was
// configured, so two connections asked for the same name would both answer to it.
func (r *ConnectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
