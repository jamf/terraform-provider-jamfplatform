# Repository Guidelines

## Overview

Terraform provider for Jamf Platform APIs, built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) v1.19.0 with Protocol v6. Go module path: `github.com/jamf/terraform-provider-jamfplatform`.

Five construct types: **resources** (CRUD), **data sources** (read-only lookups), **list resources** (RSQL-filtered streaming), **actions** (fire-and-forget device commands), and **functions** (offline provider-defined functions under the `jamfplatform::` namespace — no API client, no provider config).

The Jamf Platform API client is the external Go SDK `github.com/jamf/jamfplatform-go-sdk` (package `jamfplatform`). Not vendored.

## Companion Docs (authoritative on conflict)

- `STYLE_GUIDE.md` — Go style, file conventions, schema rules, Jamf Pro naming/version/endpoint policies, scope helper, profile payload normalisation, ID/import conventions.
- `TESTING.md` — test categories, commands, build tags, CI.
- `CONTRIBUTING.md` — contribution workflow, Pro-resource workflow (incl. ProClassic SDK payload audit), release-versioning policy.
- `README.md` — user-facing provider usage.
- `spike/JAMF_PRO_INVENTORY.md` — gitignored Pro SDK namespace adoption tracker.
- `spike/PRO_ROLLOUT_PLAN.md` — gitignored Pro rollout planning doc.

## Project Structure

```
internal/
├── provider/          # Provider config, registration, logging
├── providerdata/      # Shared Data{} carrying SDK client + cached Pro version
├── resources/
│   ├── account/       # Jamf Account — organization-level, flat single tier   (Jamf Account)
│   │                  #   sso_domain/
│   │                  #   (folder name = Terraform slug minus `jamfplatform_account_`)
│   ├── ai_governance/ # policy/, tool/                                      (Jamf AI Governance)
│   │                  #   (folder name = Terraform slug minus `jamfplatform_ai_governance_`)
│   ├── blueprints/    # blueprint/, blueprints/, component/, components/    (Platform Services)
│   ├── cbengine/      # benchmark/, benchmarks/, baselines/, rules/         (Platform Services)
│   ├── device/        # Single device data source                            (Platform Services)
│   ├── device_group/  # Resource, data source, list resource                 (Platform Services)
│   ├── device_groups/ # Plural data source                                   (Platform Services)
│   ├── devices/       # Plural data source                                   (Platform Services)
│   ├── security_cloud/ # Jamf Security Cloud — flat single tier                (Security Cloud)
│   │                  #   device_group/, dns_zone/, uem_connect/, ztna_gateway/,
│   │                  #   ztna_grouped_gateway/, ztna_shared_gateways/
│   │                  #   (folder name = Terraform slug minus `jamfplatform_security_cloud_`)
│   └── pro/           # Jamf Pro resources — flat single tier: every leaf package sits directly under pro/
│                      #   (folder name = Terraform slug minus `jamfplatform_pro_`, snake_case). No domain tier.
│                      #   ~115 packages, fully flat (e.g. the five PKI constructs are pki_adcs/, pki_venafi/, pki_digicert/, … — no pki/ grouping dir).
├── actions/
│   ├── account/       # sso_domain/ (verify)                                 (Jamf Account)
│   ├── device/        # erase, restart, shutdown, unmanage                       (Platform Device Actions API)
│   └── pro/           # managed_software_updates (plan + abandon), maintenance/ (flush_policy_logs, redeploy_management_framework), mdm/ (send_blank_push, renew_mdm_profile, flush_mdm_commands), patch/ (retry_patch_policy_logs)   (Jamf Pro)
├── functions/         # Provider-defined functions (offline; no SDK client, no provider config)
│   ├── mobileconfig/       # mobileconfig(profile) — build a full .mobileconfig from HCL payloads; also holds the shared Assemble core
│   └── mcx_forced_payload/ # mcx_forced_payload(domain, prefs) — MCX "Custom Settings" envelope; thin wrapper over mobileconfig.Assemble
├── common/
│   ├── aischemas/     # Vendor JSON Schema fetch/cache/validate for AI Governance policy settings
│   ├── appledeclarations/ # Apple declarative device management schemas (generated table + plan-time declaration validation; see §Apple declaration schemas)
│   ├── appleprofiles/ # Apple configuration profile schemas (generated table + plan-time payload validation; see §Apple profile schemas)
│   ├── availabletitles/ # Shared patch available-titles lookup (patch_external_source, patch_internal_source)
│   ├── criteria/      # Shared smart-group / advanced-search criteria operator vocabulary (device_group, user_group, future searches)
│   ├── enumguard/     # Recurrence guard: no enum value or error code restated as a literal the SDK generates
│   ├── files/         # Shared upload-source plumbing for resources that upload file content
│   ├── filters/       # RSQL + classic filter schema/expression builder
│   ├── helpers/       # Type conversions, polling, timeout, state reconciliation, dynamic JSON, IDs, Pro version
│   ├── impact/        # Plan-time impact alerts — device counts for scope changes (see §Impact alerts)
│   ├── invitationcommon/ # Shared enrollment-invitation helpers (computer_invitation, mobile_device_invitation)
│   ├── jsonvalue/     # Rendering a decoded JSON value for a diagnostic (aischemas, appleprofiles)
│   ├── ldapgroups/    # Directory-service (LDAP / cloud-IdP) group resolution + scope preflight validation
│   ├── payloadhelpers/ # mobileconfig mask / compare / identifier injection (macos/mobile_device_configuration_profile)
│   ├── permissions/   # Required-Jamf-permission tables rendered into schema descriptions (see §Required permission tables)
│   ├── planmodifiers/ # Shared Terraform Plugin Framework plan modifiers
│   ├── plisthelpers/  # Generic plist (Apple property list) parsing / normalisation helpers
│   ├── scope/         # Classic scope sub-block factories + builders + validators (see STYLE_GUIDE §Scope helper)
│   └── validators/    # Shared Terraform Plugin Framework validators (unique-string-field across collections)
└── testhelpers/       # Acceptance fixtures (provider factories, real client, mock server)
tools/                 # go:generate entrypoint (copywrite, terraform fmt, tfplugindocs)
local-testing/         # Manual API request workflows for development (gitignored)
examples/{provider,resources,data-sources,list-resources,actions,functions}/
docs/                  # Auto-generated provider documentation — do not hand-edit
```

Each leaf resource folder mirrors the file split in [STYLE_GUIDE.md §Resource Package File Conventions](STYLE_GUIDE.md#resource-package-file-conventions).

### Reference implementations (copy from these)

| Pattern | Reference |
|---|---|
| Complex CRUD with state upgrader + nested payload sub-package | `internal/resources/blueprints/blueprint/` |
| Simple Pro CRUD (+ list + data sources) | `internal/resources/pro/category/` |
| Pro CRUD with lossy-PUT canonicalisation + snake_case mapping | `internal/resources/pro/script/` |
| Pro singleton settings | `internal/resources/pro/self_service_plus_settings/` |
| ProClassic CRUD | `internal/resources/pro/site/` (and `network_segment/`) |
| Scope-bearing classic resource | `internal/resources/pro/policy/` |
| Classic configuration profile (mobileconfig payload diff suppression) | `internal/resources/pro/macos_configuration_profile/` |
| Plaintext secret with `WriteOnly + _wo_version` | `internal/resources/pro/directory_binding/` |
| Classic-CRUD resource with a v2 side-channel (extension-attribute accept) | `internal/resources/pro/patch_software_title/` |
| Positional id-less nested lists + opt-out sub-collections (omit=retain/`[]`=clear) + Computed nested collections as `types.List` | `internal/resources/pro/licensed_software/` |
| Create-only immutable upload (server rejects every PUT once the blob exists → all attrs RequiresReplace + no-PUT Update) | `internal/resources/pro/mobile_device_provisioning_profile/` |
| Classic XML merge PUT where empty clears (always-emit scalars / clear-by-omission) + Location/Purchasing blocks + read-only attachments (bearer-auth-refused upload) | `internal/resources/pro/mobile_device_enrollment_profile/` |
| Provider-defined function (offline; `types.Dynamic` decode + shared core) | `internal/functions/mobileconfig/` |
| Jamf Security Cloud CRUD (+ DS singular/plural + list; no tenant version gate) | `internal/resources/security_cloud/dns_zone/` |
| Two-form resource where the form is immutable and derived from a block's presence | `internal/resources/security_cloud/ztna_gateway/` |
| Read-only server catalogue, plural DS only (no per-id endpoint) | `internal/resources/security_cloud/ztna_shared_gateways/` |
| Free-form vendor JSON payload (JSON-string attribute + semantic equality + live schema validation) | `internal/resources/ai_governance/policy/` |

## Impact alerts — one-paragraph orientation

`internal/common/impact/` produces Jamf Pro's **impact alert notifications** during `terraform plan`: an advisory warning on each object whose scope or payload is changing, reporting how many computers or mobile devices the change reaches. Off by default behind the provider's `impact_alerts` attribute; a nil `*impact.Cache` means disabled, so resources need no flag check. Three channels. Two mirror Jamf's own split — **deployable** (policies, profiles, apps, blueprints, benchmarks) and **scopeable** (groups, classes) — because a resource's `ModifyPlan` cannot see a sibling's, so a plan editing both a group and something scoped to it needs an alert from each side. The third, **policy dependencies** (script, package, printer, Dock item, directory binding, disk encryption configuration), has no counterpart in Jamf Pro: these objects have no scope of their own, so their blast radius is the combined audience of the policies referencing them. Resources wire in via `impact.ReportPlan` (deployable), `impact.ReportMembership` (scopeable) or `impact.ReportDependencyPlan` (dependencies), plus a per-family adapter — a reducer to `impact.Scope` for the first two, and for a dependency only a reader for the object's id and name. The shared Jamf Pro scope block goes through `scope.BuildImpactScope`, which is the single place the narrows/broadens/counts classification lives. Dependencies need a whole-tenant policy sweep, since Jamf Pro has no reverse lookup ("which policies use this script"): lazy, at most once per configured provider instance, self-capped at 5 concurrent reads, and built by `impact.NewCacheWithPolicies` (`NewTenantCache` wires both sources). Alerts are advisory — a tenant that cannot be read yields one notice and never fails a plan. Two rules that are easy to get wrong: a numeric Jamf Pro group id is unique only **within an estate** (see [STYLE_GUIDE.md §Scope helper](STYLE_GUIDE.md#scope-helper)), and group membership is expressed in **device management identifiers**, so a Jamf Pro numeric device/building/department id must be resolved through the inventory before it can be compared with a group's members. User guidance, including the `terraform plan -json` caveat, is in `docs/guides/impact-alerts.md`.

## Required permission tables — one-paragraph orientation

`internal/common/permissions/` renders the **Required Jamf permissions** table into every
construct's schema description: for each SDK method a construct calls, the section and permission
name an operator ticks in Jamf Account, plus the `{capability}:{action}` identifier. The mapping
from capability slug to picker row exists in exactly one place — Jamf's [Jamf Pro permissions
map](https://developer.jamf.com/platform-api/reference/jamf-pro-permissions-map) article — because
the OpenAPI specs publish the slug and nothing else, and the two differ substantially
(`computer-inventory-collection-settings` is "Device inventory collection settings"). `catalogue.go`
transcribes that article, all 125 rows, deliberately complete rather than trimmed to what this
provider calls so it stays diffable against the next revision. **The article is the source of
truth**, and since it is published as markdown at the same URL with `.md` appended, that is now
enforced rather than trusted: `permissions-map.md` beside the catalogue is a verbatim snapshot,
`make permissions-map` refetches it, and `TestCatalogueMatchesThePublishedMap` asserts every row's
section and name against it — so a renamed or relocated permission fails a build instead of quietly
sending an operator to a checkbox that no longer exists. Two guards keep that honest: a parse
yielding under 100 capabilities fails outright, because a parser that silently finds nothing reports
perfect agreement; and only the article's own capability tables are read, bounded between two named
headings, since its narrative tables list *old* privilege names in the same column position.
`TestCatalogueCoversEverySDKCapability` covers the other direction (a capability the SDK requires
and the file has never heard of), and `catalogue.golden` pins the rendered triples so any edit is a
reviewable diff. `.github/workflows/permissions-map.yml` refetches monthly and opens a pull request
— red, with the failures in its body, when the catalogue no longer agrees. What nothing here can
prove is that the article matches what Jamf Account's picker actually prints; the same pattern lives
in `jamfplatform-go-sdk` (capability and actions only, as a privilege oracle) and in `jamfpro-cli`.

## Apple profile schemas — one-paragraph orientation

`internal/common/appleprofiles/` carries a generated table of Apple's configuration profile payload
keys, built from the `mdm/profiles` directory of apple/device-management by `make apple-schemas`
(never part of `make generate` — it needs a network clone; `make apple-profiles` survives as an
alias for muscle memory). The table is the **union** of Apple's `release` branch and its newest
`seed_OS_*` (pre-release) branch, because Jamf's generative-declarations service tracks seed and
offers seed-only keys in the UI immediately, so a release-only table would reject configurations
that work — which is why the target *fails* rather than quietly building from release alone when
seed discovery finds nothing. Jamf's blueprints service validates a stored legacy payload against
the same vocabulary, and wire probing established exactly how: an unknown **payload type** is
rejected and matched case-sensitively; an unknown **key** is silently discarded; a key differing
only in case is silently stored under Apple's spelling; a wrong value type or a missing required key
fails the write; enum and range constraints are **not** enforced (`AlertType: 99` stores fine), so
the table does not check them either. `appleprofiles.Validate` turns those rules into `Problem`
values and **every one of them is an error** — the advisory tier is gone, because a discarded key
produces a payload that reports success and never applies, and a warning left the operator with
exactly that. What survives of the old split is a *diagnostic* distinction rather than a severity
one: `Problem.StaleTableSuspect()` (formerly `Advisory()`) reports whether the finding could be
explained by the embedded table being older than the tenant — the name-based findings plus
`MissingRequiredKey`, since Apple relaxes `required` between revisions — and such a finding names
the snapshot's branches and commits and points at the escape hatch. That escape hatch is **per
block, not per payload**: `appendLegacyConfigProfile` folds every payload in a block into one
`com.jamf.ddm-configuration-profile` component, so escaping one payload means moving all of the
block's payloads to a single `raw_component`. Descent stops at a free-form (wildcard) dictionary:
everything under an MCX preference domain (`com.apple.ManagedClient.preferences`, the "Custom
Settings" envelope) is passthrough, and Jamf stops validating there too — but a wildcard declared
*alongside* named keys suppresses only the unknown-key finding, so the named keys are still
type-checked. The blueprint resource wires this in through `validators.go` for both
`component_blocks[].legacy_payloads` and the deprecated top-level `legacy_payloads`. Freshness is a
scheduled concern, not a plan-time one: `.github/workflows/apple-schemas.yml` regenerates **daily**
and opens a pull request, the same reviewable-PR pattern Dependabot uses here — daily rather than
monthly because an unrecognised name is now an error, so a stale table blocks a working
configuration instead of merely warning about one. User guidance is in
`docs/guides/apple-schema-validation.md`.

## Apple declaration schemas — one-paragraph orientation

`internal/common/appledeclarations/` is the same shape for **declarative device management**: a
generated table of Apple's declaration types, built from the `declarative` directory of
apple/device-management by the same `make apple-schemas` run, so a legacy payload and the
declaration wrapping it can never be validated against different upstream revisions. It is
deliberately a sibling of `appleprofiles/` rather than an extension of it, because the two services
behave *oppositely* on the case question and shared code would blur that. Jamf validates
declarations not at all: wire probing on 2026-09-10 found the blueprints service returns `201` for
an unknown declaration type, an invented key, a wrong-cased key, a wrong value type, an out-of-enum
value, an out-of-range integer and a missing required key alike, a deploy of the same blueprint
returns `202` and reports SUCCEEDED, and the Jamf Pro editor then renders the generated form with
the offending key blank while the device never receives it. So `terraform plan` is the only place
any of it is caught, and every finding is an **error** — there is no advisory tier and no provider
switch to soften one; a declaration that should not be checked belongs in `raw_component`. Four
rules came out of that probe and are easy to get wrong. Declaration key names are matched
**CASE-SENSITIVELY, and a wrong-cased key is discarded** — the exact opposite of a configuration
profile payload, where Jamf restores Apple's spelling, which is why the two packages must not share
case handling. A declaration type is likewise case-sensitive, and a wrong-cased one renders no card
at all. A value outside a declared `rangelist` is dropped, so an enum violation silently never
applies, while a value outside a declared `range` is stored and rendered unchanged — the *device* is
what rejects that one, which is why it is still an error rather than a note. And `kind` is accepted
in any pairing with `type`, so the pairing is derived from the type's reverse-domain prefix rather
than from a table; two of the four kinds it derives are a dated **gateway widening** recorded on
`KindForType` in the shape `internal/providerdata/scopes.go` uses, because
`blueprints.DeclarationKindValues()` declares only `CONFIGURATION` and `ASSET` while
`POST /blueprints/v1/blueprints` accepted `ACTIVATION` and `MANAGEMENT` and stored both verbatim on
the EU gateway, 2026-09-10 — and `enum_literals_test.go` is the tripwire for the day a spec ingest
catches up. The blueprint resource wires this in through `declaration_validators.go` for both
declaration-bearing components: `apple_declarations`, a list *of declarations* — the attribute is
the list, not an object wrapping one, so a finding lands on the exact element and `$PAYLOAD_n`
cross-references are range-checked — and `custom_declarations`, a set, so a finding names the
declaration type instead. That shape is why `apple_declarations` is assembled in the blueprint
package (`appendAppleDeclarations`, `flattenAppleDeclarations`) rather than in `components/`, the
same split `legacy_payloads` has and for the same reason: neither wire component is a typed object a
`ComponentConverter` can own, and both need what the components package cannot see — the blueprint's
name for one, the prior state for the other. The prior state is what lets `payload` be authored with
`file()`: an authored JSON string is kept whenever it is semantically equal to what Jamf
re-serialised, aligned **by position** since two declarations of one type in a block are legal.
What none of this can prove is that Jamf tracks the *same* seed
branch the table unions — that is inferred from `siri.settings.AllowSiriAI` appearing in the Jamf
picker while absent from release, so re-probe it first if false positives reappear for keys the UI
offers. User guidance is in `docs/guides/apple-schema-validation.md`.

## Blueprint legacy payload identifiers — one-paragraph orientation

A legacy payload's `payloadIdentifier` is the blueprints service's to assign, not the provider's: it
is absent from the Blueprints API specification, which requires only `payloadType` in a
`payloadContent` entry, and wire probing on 2026-09-16 established that a payload arriving without
one is accepted and stamped with a **fresh UUID on every write**, on POST and on merge-patch alike,
while one sent with an identifier keeps it verbatim and unvalidated across later writes. Because the
update is a whole-body merge-patch and Apple keys an *installed* payload on its identifier, letting
the service mint would re-identify every payload in the blueprint on each unrelated edit and
reinstall every profile on every scoped device — silently, since the field is masked out of state and
`deploymentState` reports `OUT_OF_DATE` after any write regardless. So a blueprint update is
**read-merge-write**, the same shape the configuration profile resources get from
`payloadhelpers.InjectTopLevelIdentifierValues`: `readStoredLegacyPayloadIdentifiers` fetches the
blueprint first and writes each stored identifier back, located by step and then by `payloadType`
with steps matched **by name ahead of position** so that inserting or reordering a block cannot shift
one block's identifiers onto another's payloads; create sends none, an authored value is discarded
either way, and a failed pre-read fails the apply rather than falling back to minting. One carve-out
to know before reaching for the escape hatch: the injection is wired into `legacy_payloads` alone, so
a legacy profile authored as `raw_component` goes through `collectBlockComponents`, which re-marshals
the author's own JSON untouched — that author must state a stable identifier themselves, which
`templates/guides/apple-schema-validation.md` and the resource example both say.

## Jamf Security Cloud resources — one-paragraph orientation

Terraform construct name format: `jamfplatform_security_cloud_<resource>`; Go package
`internal/resources/security_cloud/<resource>/`, flat single tier like `pro/`. All five
Security Cloud API namespaces (`jsc-categories`, `jsc-dns`, `jsc-ztna`,
`securitycloud-devices`, `uem-connect`) are generated into one SDK package,
`jamfplatform/securitycloud`, and every method routes through the unified
`/securitycloud` prefix — wire-verified in production EU on 2026-08-27 for the DNS
surface and 2026-08-29 for UEM Connect, both under a tenant-scoped integration. There is
no `/api` segment: the Platform API GA dropped it everywhere, and a request carrying it
gets the gateway's own bare `404 page not found` rather than a JSON error. Configure goes through
`providerdata.ConfigureSecurityCloud`, **not** `ConfigurePro`: Security Cloud is
continuously deployed with no customer-tenant version, and a tenant can hold it without
holding Jamf Pro, so a Pro version fetch would be both meaningless and fatal. Two things
differ from every other namespace and are easy to get wrong. First, **entitlement is not
authentication** — a valid integration can still be refused with `403 NOT_ENTITLED`, so
resources translate that code into a named diagnostic instead of surfacing the raw error.
Second, **cross-namespace references are server-enforced in both directions**: a DNS
zone's name servers each name a gateway by ID — a shared, dedicated or grouped gateway, all
three accepted — and a zone cannot be written before its gateway exists
(`422 GATEWAY_NOT_FOUND`), so that diagnostic points at `authoritative_name_servers` rather than
the zone; conversely a gateway that anything still references refuses to be deleted with a bare
`409 CONFLICT` naming nothing, which is a Terraform destroy-ordering trap and gets its own
diagnostic. That last behaviour is **per construct, not a namespace rule** — a device group
that a ZTNA app still names deletes cleanly and silently empties the app's assignment instead
— so probe the referenced delete for each new construct rather than inheriting either answer.
Three shapes recur across the namespace and are worth knowing before reading any of
it: a **cipher/algorithm field is an array the server accepts exactly one element in**, so it
is modelled as a single string and collapsed at the boundary; an **enum violation is
unattributed, but not identically so across services** — `jsc-dns` and `jsc-ztna` answer
`400 [INVALID_FIELD] Request body is missing or malformed.` with no field and no value,
while `uem-connect` answers `422 VALIDATION_FAILED` leaking Jackson's message, which does
name the accepted values; either way every enum is validated at plan time from the SDK's
own generated `*Values()` helper rather than a restated list, and the per-service
difference is one more reason not to write a diagnostic against a code another construct
observed; and an **unmapped route answers `403 BAD_PERMISSIONS`,
indistinguishable from a real privilege gap** (a bogus path returns the same body), so that
code is deliberately never translated into a diagnostic and a spec-advertised endpoint the
gateway 403s on is presumed unrouted rather than unprivileged. Acceptance tests gate on
`testhelpers.AccPreCheckSecurityCloud`, which requires the operator to *declare* that the
configured scope is a Security Cloud one (`JAMFPLATFORM_ACC_SECURITYCLOUD_{ENVIRONMENT,TENANT}_ID`,
matching the scope in use) and skips otherwise — a Pro-only acceptance tenant is a
legitimate environment, not a failure. Full rules:
[STYLE_GUIDE.md §Jamf Security Cloud Resource Naming](STYLE_GUIDE.md#jamf-security-cloud-resource-naming).

## Jamf Account — one-paragraph orientation

Terraform construct name format: `jamfplatform_account_<x>`; Go package
`internal/resources/account/<x>/`, flat single tier like `security_cloud/`. This is the provider's
**organization-level** family — what a practitioner manages at *account.jamf.com → Organization*,
above and across individual tenants. Today that means **SSO**: a `sso_domain` is a DNS domain the
organization has claimed and proved ownership of, and a `sso_connection` — shipped as a resource,
two data sources and a list resource — is an OIDC identity-provider connection that signs users in
for one or more of those domains, scoped to chosen Jamf Pro, School, Protect and Security Cloud
tenants. Family
facts and the naming rationale are in
[STYLE_GUIDE.md §Jamf Account Resource Naming](STYLE_GUIDE.md#jamf-account-resource-naming); five
things drive the design and are easy to get wrong. First, **this is the only family reachable under
organization scope, and the only scope that reaches it** — wire-probed 2026-09-02 on two
organization-scoped integrations, while a tenant-scoped one is refused `403 BAD_PERMISSIONS` on the
same URL in the same region (provably an authorization refusal rather than an unrouted path, because
another credential answers 200 there). Environment scope is *untested*, not ruled out: the namespace
exists only on the **US gateway**, so no US environment-scoped integration was available, and
widening `ConfigureAccount`'s gate is a fresh probe rather than a one-token edit. No organization ID
travels anywhere — the gateway resolves it from the access token, so there is no `organization_id`
attribute and no SDK `WithOrganizationID`. Second, **a domain is create/verify/delete only**: `GET`,
`PUT` and `PATCH` on `/sso/v1/domains/{id}` all answer `403 BAD_PERMISSIONS`, which by this repo's
law means unrouted — so every attribute is `RequiresReplace`, Read scans the list endpoint, and
import is by domain **name**. Third, **verification is an action, not resource state, and it is not
idempotent**: a *failed* verify returns `200` with `domainStatus` unchanged (so the status code says
nothing), yet still bumps `lastModifiedDate` and pushes `verificationExpirationDate` out 14 days —
and because the five-minute rate limit is measured from `lastModifiedDate`, which the claim itself
sets, the first verify after creating a domain is always refused. Fourth, two things about a connection, and note which
endpoint each belongs to, because getting that backwards is easy. **Its tenant allow-list is
write-only**: neither read shape echoes `enabledProducts` or `enabledEnvironments`, and no endpoint
anywhere lists an organization's tenants, so the ids can be neither discovered nor drift-checked —
documented rather than solved. And **changing one in place is impossible, though creating one is
not**: `POST` answers `201` for a valid body, and reading, importing, the data sources, the list
resource and deleting all work, but `PUT /sso/v1/connections/{id}` answers `500 UPSTREAM_ERROR` for
*every* request — including the verbatim body a create had just accepted — and wire-verified
2026-09-03 the refused write applies nothing, a connection read back after a PUT changing three
fields being byte-identical. So `sso_connection` carries a whole-resource `ModifyPlan` that plans a
**replacement** on any configured change, in one file rather than fifty `RequiresReplace` modifiers
so that reverting it is deleting a file; the `Update` method and its acceptance test are already
written and gated behind `skipUnlessConnectionUpdatesWork`, ready for the day Jamf fixes the
endpoint. Fifth,
**the server attributes almost nothing**: `errors[].field` is populated only for top-level required
fields, an invalid enum returns `MALFORMED_REQUEST_BODY` with `field: null` and never names the
value, and a `connectionType`-versus-payload mismatch is an unattributed `500` — so every enum is
taken from the SDK's own `*Values()` helper rather than a restated list, and the connection's
discriminator/payload pairing will be validated at plan time the same way once it ships.

## Jamf AI Governance — one-paragraph orientation

Terraform construct name format: `jamfplatform_ai_governance_<x>`; Go package
`internal/resources/ai_governance/<x>/`, flat single tier like `security_cloud/`. An **AI policy** is
the managed configuration for one AI tool — Claude Code, Claude Desktop or OpenAI Codex today — which
Jamf Pro then delivers to Macs through a blueprint's `com.jamf.ai-governance` component. Family is
**Platform Services**: hand-rolled Configure, no version gate, no SDK-endpoints annotation block —
but the scope gate is `ScopeEnvironment` **alone**, and both alternatives are wire-probed: a request
carrying no scope header is refused with `400 REQUEST_CONTEXT_NOT_PROVIDED`, and one carrying
`X-Tenant-Id` is refused with `403 BAD_PERMISSIONS` — while `GET /pro/v1/csa/tenant-id` under that
same header answers 200, so the header is accepted and the refusal belongs to this namespace. By this
repo's own law (§Jamf Security Cloud: `403 BAD_PERMISSIONS` is indistinguishable from a privilege
gap, so a spec-advertised route answering it is presumed unrouted) AI Governance is **not reachable**
under tenant scope, and widening the gate is a fresh probe rather than a one-token edit. (Note this
retires the Phase 11 epic's recorded blocker, which had the namespace down as organization-scope and
therefore unbuildable; and the path is `/ai/governance/policies/v1/...`, not `/api/ai-governance/...`.)
Four things drive the design and are easy to get wrong. First, **the settings body is the tool vendor's
own JSON**, declared by a JSON Schema the platform serves per tool per schema version, 142 top-level
properties for Claude Code and deeply nested for Codex — so it is one `settings_json` string
attribute with JSON semantic equality, never generated typed attributes (schema versions coexist per
policy, `anyOf` unions have no framework equivalent, and two Claude Code schema versions shipped in
three months). `internal/common/aischemas` fetches, caches and validates it at plan time; its
partiality is measured, not assumed, and its package doc says which keywords are skipped and why.
Second, **validation strength is a property of the vendor schema, not the service**: an undeclared key
is silently *stored and never applied* where the schema allows extras (Claude Code) and rejected with
`422` where it does not (Codex) — which is why an unrecognised key is a warning and everything else an
error, and why it is the one failure the service itself never reports. Third, **a policy has a draft
and a published history**: create publishes nothing, a blueprint pins a *version number*, the platform
diffs settings itself so an apply changing only the name mints no version — and `schemaVersion` is
part of neither diff, so moving a policy to a newer schema *without* touching the settings publishes
nothing and leaves blueprints delivering the old schema. Fourth, **archiving is not blocked by a
blueprint that references the policy**: the delete succeeds and leaves the blueprint pointing at a
version the platform will no longer serve, with no reverse lookup to warn from
(`GET /policies/{id}/deployment` returns an empty list even for a deployed referencing blueprint), so
that trap is documentation only — the opposite of the ZTNA gateway law, and one more reason to probe
referenced deletes per construct. User guidance is in `docs/guides/ai-governance-policies.md`.

## Jamf Pro resources — one-paragraph orientation

Terraform construct name format: `jamfplatform_pro_<resource>` regardless of whether the SDK source is `pro/` or `proclassic/`. One `jamfplatform.Client` built from `JAMFPLATFORM_*` credentials serves both Platform Services and Pro. Every Pro resource declares an unexported `const minJamfProVersion` and funnels Configure through `providerdata.ConfigurePro` (no hand-rolled boilerplate). Each Pro resource's `crud.go` opens with an SDK-endpoints annotation block (`Status: current. Last reviewed YYYY-MM-DD.`) — Pro / ProClassic only; Platform Services resources are exempt. Full rules: [STYLE_GUIDE.md §Jamf Pro Resource Naming](STYLE_GUIDE.md#jamf-pro-resource-naming), §Minimum Jamf Pro version check, §Endpoint adoption & migration policy. Workflow for adding a Pro resource (incl. SDK-comparison + ProClassic payload audit gate): [CONTRIBUTING.md §Adding a Jamf Pro Resource](CONTRIBUTING.md#adding-a-jamf-pro-resource).

## API integration scope — one-paragraph orientation

Jamf offers three scopes when an API integration is created, and the provider mirrors all three: **Platform environment** (a group of tenants across product types — the *preferred* scope, `environment_id` → `X-Environment-Id`), **Tenant** (a single Jamf Pro / School / Protect / Security Cloud tenant — Jamf's own words are "legacy method for targeting integrations without a platform environment", `tenant_id` → `X-Tenant-Id`), and **Organization management** (SSO and similar organization-level resources — no scope header at all; the gateway resolves the context from the access token; note AI Governance reads as organization-level and is *not*, it is environment-scoped). Since SDK v0.17.0 the scope travels in a header rather than the URL path, set by `WithEnvironmentID` / `WithTenantID`. So `environment_id` and `tenant_id` are **mutually exclusive and both optional** — an integration targets one, and supplying the other is refused with `403 OWNERSHIP_FORBIDDEN` even when both IDs belong to the same customer. `internal/provider/scope.go` resolves which is in play (config beats environment; both-at-once is an error either way; a shadowed env var warns) and selects the SDK option accordingly; `providerdata.New` then reads the scope back off the built client via `Client.Scope()` (SDK v0.18.0), so the gate can never disagree with the header the client actually sends. Whether a given construct can be reached under that scope is then enforced **per construct**, not once in provider Configure, via `providerdata.RequireScope` in `internal/providerdata/scope.go` — because the answer differs per API family and has already differed twice: Jamf Pro works under either scope (gated once inside `configureSub`, covering every `pro/` package and Pro action), Security Cloud likewise works under either and is gated once inside `ConfigureSecurityCloud`, while Blueprints, Compliance Benchmarks and AI Governance are environment-only. Call sites never write the kinds out; they pass a derived family set from `internal/providerdata/scopes.go`, resolved at initialisation from the SDK privilege registry's `MethodPrivileges.Scopes` (SDK v0.22.0) so that a spec ingest moving a family arrives as one changed value with a pinned expectation to agree with rather than going stale at 27 call sites. Two rules keep that honest. The intersection, not the union, is what a construct needs — `Scopes` is an alternatives set per method and a client carries exactly one scope. And a **spec-declared set is not always what the gateway serves**: the GA deleted `X-Tenant-Id` from six Platform specs while the gateway went on answering it, so `scopes.go` carries a `gatewayWidenings` table of family, scope and dated wire evidence (three entries — Platform devices, device groups, device actions, all 2026-09-04, two independent probes plus a green tenant-scope acceptance run on `device_group` and `devices`), whose entries are deleted rather than edited once either half of the justification goes; `scopes_test.go` fails on one the registry has caught up with, and on a family name that matches nothing and therefore widens nothing. Blueprints and Compliance Benchmarks are deliberately **not** widened, and note *why*: two different tenant credentials are refused `403 BAD_PERMISSIONS`, which classifies nothing on its own — Security Cloud answers identically and is reachable under both scopes — so the narrowing rests instead on Jamf Account's permission picker not offering the `blueprints` or `compliance-benchmarks` capabilities at all when the integration being created is tenant-scoped, observed in the picker 2026-09-04. That is falsified by a change to the picker rather than by a `200` from either route, and it is verified live to produce the named Configure diagnostic in 0.25s where the same run previously spent 134s applying before the 403. Scope is documented as well as enforced — `permissions.Section` opens each "Required Jamf permissions" block with the integration scope, from the same registry, reporting the spec's declaration and deliberately not the widenings. Organization scope is **not** a rejected special case: it is the only scope that reaches the `jamfplatform_account_*` family (Jamf Account SSO), gated once inside `ConfigureAccount`, and it is rejected everywhere else. Enforcement runs both ways for the same reason — an organization-scoped integration aimed at a Pro resource, or an environment-scoped one aimed at a Jamf Account resource, each turn an opaque gateway failure mid-apply into a named diagnostic at Configure.

## Tooling

- Go >= 1.26, Terraform >= 1.13.0.
- `GNUmakefile` is the canonical entrypoint. Default target: `fmt lint install generate`.
- Releases built with goreleaser (`goreleaser.yml`).
- `make generate` → `tools/tools.go`: copyright headers (`hashicorp/copywrite`), `terraform fmt -recursive ../examples/`, provider docs (`hashicorp/terraform-plugin-docs`).

| Target | Description |
|---|---|
| `build` | Build the provider |
| `install` | Build and install locally |
| `fmt` | `gofmt -s -w -e .` |
| `fix` | `go fix ./...` — rewrites deprecated API usages |
| `lint` | `golangci-lint run` |
| `generate` | Copyright headers + `terraform fmt examples/` + docs |
| `apple-schemas` | Regenerate both embedded Apple schema tables — `internal/common/appleprofiles/profiles.json` and `internal/common/appledeclarations/declarations.json` — from apple/device-management, unioning the `release` branch with the newest `seed_OS_*` branch (network; not part of `generate`; `apple-profiles` survives as an alias) |
| `permissions-map` | Refetch `internal/common/permissions/permissions-map.md` from Jamf's permissions map article (network; not part of `generate`) |
| `test` | Unit tests (excludes `acceptance` build tag) |
| `test-scripts` | Unit tests for `scripts/acctargets` (behind the `acctargets` build tag, so `go test ./...` misses it) |
| `testacc` | Acceptance tests (sets `TF_ACC=1`, requires tenant) |
| `testacc-run` | Targeted acc rerun (`RUN=<regex> PKG=<path>`) |

Before committing: `make fix fmt lint test`. Then `make generate` if any schema description or example changed.

## Environment Variables

- `JAMFPLATFORM_BASE_URL` — `https://us.api.jamfcloud.com` / `eu.api.jamfcloud.com` / `apac.api.jamfcloud.com`. The gateway root, host only: it serves `/auth/token` and every namespace at the root, so a `/api` path breaks authentication. The pre-GA beta `{region}.apigw.jamf.com` still resolves and still answers the token exchange, but serves no API namespace, so the provider refuses it at configure time.
- `JAMFPLATFORM_CLIENT_ID` / `JAMFPLATFORM_CLIENT_SECRET` — API client credentials.
- `JAMFPLATFORM_ENVIRONMENT_ID` — platform-environment scope, sent as `X-Environment-Id`. **Preferred.**
- `JAMFPLATFORM_TENANT_ID` — tenant scope, sent as `X-Tenant-Id`. **Legacy.** Mutually exclusive with the above; both are optional — see §API integration scope.
- `JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID` — **acceptance tests only**. Declares that the configured credentials are an organization-scoped integration, which is what the `jamfplatform_account_*` family requires. There is nothing to compare it against — an organization-scoped request carries no scope header, so no `JAMFPLATFORM_*` variable holds the organization — and that is exactly why the declaration is needed: without it a Pro-only credential set with both scope variables unset looks identical to a real organization integration. `testhelpers.AccPreCheckAccount` additionally requires **both** `JAMFPLATFORM_ENVIRONMENT_ID` and `JAMFPLATFORM_TENANT_ID` to be *unset*, and skips on a non-US base URL, since `/sso/v1` is served only from the US gateway.
- `JAMFPLATFORM_ACC_ORGANIZATION_SSO_VERIFIED_DOMAIN` / `JAMFPLATFORM_ACC_ORGANIZATION_SSO_UNVERIFIABLE_DOMAIN` — **acceptance tests only**, both optional, both naming a *claimed* Jamf Account SSO domain. The first must be a **real, already-verified** domain the operator owns; the second a throwaway `.example` one, which can never verify. They cover the two verification outcomes the suite cannot manufacture: re-checking a verified domain is refused `409 CONFLICT` (which the action reports as success), and checking an unverifiable one returns `200` with the status unchanged. The **first-time** unverified→verified transition is untestable by design — the verification key is reissued on every claim, so a DNS record published for an earlier claim is stale against a new one, and automating it would require the suite to write DNS.
- `JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID` — **acceptance tests only**. Declares that the configured environment holds Jamf AI Governance; must equal `JAMFPLATFORM_ENVIRONMENT_ID`. Unset or mismatched, every AI Governance acceptance test skips locally and **fails** in the `aigovernance` lane, which names this variable in its `require` token — it was referenced by no workflow at all before the lane pipeline, so all 18 tests skipped green on every run. Environment scope only — there is no tenant form, because the surface answers a request carrying no scope header with `REQUEST_CONTEXT_NOT_PROVIDED` and one carrying `X-Tenant-Id` with `403 BAD_PERMISSIONS`, which this repo reads as an unrouted namespace.
- `JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID` — **acceptance tests only**. Client ID (UUID) of a pre-existing Jamf Pro API client holding the AD CS certificate-job privileges. `jamfplatform_pro_pki_adcs` in `OUTBOUND` mode needs one and the provider can no longer create it: the API-client and API-role endpoints were withdrawn at the Platform API GA, so clients are created in Jamf Account. Unset and the two OUTBOUND tests skip.
- `JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID` / `JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID` — **acceptance tests only**. Declares that the configured scope belongs to a Jamf Security Cloud tenant; must equal the corresponding `JAMFPLATFORM_*` value. Unset or mismatched, every Security Cloud acceptance test skips locally and **fails** in the `securitycloud` lane, which names the environment form in its `require` token. The environment form is wired in CI (wire-verified 2026-09-03); the tenant form is not, and the ZTNA gateway tests require it: `tenantIds` is mandatory on a gateway and no API exposes an environment's tenants, so an environment-scoped run cannot supply one. See [TESTING.md](TESTING.md).
- `JAMFPLATFORM_ACC_REQUIRE` — **acceptance tests only**. A comma-separated list of lane `require` tokens (`platform`, `environment`, `organization`, `securitycloud`, `aigovernance`, `pro-tenant`). It promotes an unset-credential *skip* into a *failure* for the named sets, and `.github/workflows/acceptance.yml` sets it per lane from `matrix.require`. Unset it locally and everything skips as before, which is what lets a contributor with no estate run `make testacc`. See §Acceptance CI lanes.
- Every other acceptance-only variable is named `JAMFPLATFORM_ACC_<PRODUCT>_<FIELD>`, matching `jamfplatform-go-sdk` — verbatim where that repo already names the same secret, so one value serves both (hence `JAMFPLATFORM_ACC_PRO_DEP_TOKEN` and `JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_PRO_TENANT_ID`, neither of which is what this repo would have chosen alone). The full rename map, and the dual-read shim that keeps a stale local `.env` working, are in `internal/testhelpers/accrequire/env.go`. The variables listed *above* this line are the exception and must never be renamed: the provider schema reads them at Configure and they are documented for users, so the SDK alignment happens at the **secret** layer, with `acceptance.yml` mapping the aligned secret onto the provider's own variable.
- Acceptance tests additionally require `TF_ACC=1` (set automatically by `make testacc`), and one of the two scope variables.

## Acceptance CI lanes — one-paragraph orientation

`.github/workflows/acceptance.yml` runs acceptance as **product lanes**, not one serial suite.
`.github/acceptance-lanes.json` is the single source of truth, read by `scripts/acclanes` (which
builds the GitHub matrix) and by `internal/conformance/acc_lanes_test.go` (which proves the
partition is honest, untagged, in `make test`, with no credentials). A lane is a product space and
membership is a **package path prefix** — the divergence from the SDK, whose suite is one package
and so splits on a test-name regex and hands `go test -run` a 34 KB alternation; here the package
list goes straight to `go test`, 4.9 KB for the pro lane's 110 packages. Six lanes are active —
`account`, `securitycloud`, `aigovernance`, `platform-env`, `pro-tenant`, and `pro` as the default
that owns everything unclaimed — plus `protect`, `school` and `android` reserved with
`planned: true`, which must match zero packages and fail loudly the moment one arrives. Three
things drive the design. First, **only `pro` holds the estate lock**: the others authenticate with
a different credential set or a different entitlement declaration and share no fixtures with it, so
a 38-test Jamf Account run no longer queues behind a ~2.5 h Pro suite the way it did under the old
single `acceptance-tenant` group. Second, **a running suite is never cancelled** — there is
deliberately no workflow-level `concurrency`, because cancelling SIGKILLs `go test` and no
`CheckDestroy` or `t.Cleanup` handler runs, orphaning objects on a shared estate whose duplicate
names later collide; superseded runs queue, and a `Skip if superseded` step lets a stale one exit in
seconds. Third, **absence of a credential is fatal in a pipeline and a skip locally**, via
`JAMFPLATFORM_ACC_REQUIRE` — the failure this whole shape exists to close is the invisible one,
where a missing secret makes a suite self-skip and the package still prints `ok`. That was not
hypothetical: `acceptance.yml` referenced 48 secrets of which 13 existed, and the AI Governance
declaration was read by 18 tests and referenced nowhere. A lane that plans packages and executes
zero tests now fails with a warning banner, and one `acceptance-gate` job is the required check so
branch protection never has to name a lane. Full rules: [TESTING.md §CI scaling](TESTING.md#ci-scaling).

## Copyright headers

Every Go file carries `// Copyright Jamf Software LLC <year>` + `// SPDX-License-Identifier: MPL-2.0`. Managed by `copywrite` via `make generate`. 2026
