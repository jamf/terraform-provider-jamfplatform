# Repository Guidelines

## Overview

Terraform provider for Jamf Platform APIs, built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) v1.19.0 with Protocol v6. Go module path: `github.com/jamf/terraform-provider-jamfplatform`.

Five construct types: **resources** (CRUD), **data sources** (read-only lookups), **list resources** (RSQL-filtered streaming), **actions** (fire-and-forget device commands), and **functions** (offline provider-defined functions under the `jamfplatform::` namespace — no API client, no provider config).

The Jamf Platform API client is the external Go SDK `github.com/jamf/jamfplatform-go-sdk` (package `jamfplatform`). Not vendored.

## Companion Docs (authoritative on conflict)

- `STYLE_GUIDE.md` — index only. Code comments cite rules as `STYLE_GUIDE §<section name>`; the index maps each section to its file under `contributing/`:
  - `contributing/package-layout.md` — Go conventions, dependencies, the file split inside a resource or function package, naming patterns, shared abstractions, the plan-time name collision check.
  - `contributing/schema-design.md` — attribute naming, enums from the SDK, Sets vs Lists, secrets, `Optional+Computed`, nested blocks, cross-field validation.
  - `contributing/descriptions.md` — what schema descriptions and examples say, and how.
  - `contributing/write-semantics.md` — import hydration, IDs and import, singletons, full-replace / merge-patch / classic update semantics, create and delete, errors and retry helpers.
  - `contributing/configuration-profiles.md` — payload mask-and-compare and the import fidelity gate.
  - `contributing/scope.md` — the API integration scope gate and the Jamf Pro scope helper.
  - `contributing/families.md` — Jamf Account, Jamf Security Cloud and Jamf Pro rules, Configure helpers, the Jamf Pro version gate.
  - `contributing/endpoint-lifecycle.md` — endpoint adoption and migration, schema deprecation (`PRO-DEPRECATED` / `PLATFORM-DEPRECATED` markers, 90 days), the `crud.go` SDK-endpoints annotation.
- `templates/guides/versioning-and-support.md` — the user-facing policy (generated into `docs/guides/`): versioning (a minor release can break), the 90-day deprecation window, supported Jamf Pro and Terraform versions. Source of truth for what users are promised.
- `TESTING.md` — test categories, commands, build tags, required tests by shape, CI lanes.
- `CONTRIBUTING.md` — contribution workflow, Pro-resource workflow (incl. ProClassic SDK payload audit), fixing a defect in the SDK, release steps.
- `README.md` — user-facing provider usage.
- `spike/JAMF_PRO_INVENTORY.md` — gitignored Pro SDK namespace adoption tracker.
- `spike/PRO_ROLLOUT_PLAN.md` — gitignored Pro rollout planning doc.

## Project Structure

```
internal/
├── provider/          # Provider config, registration, logging
├── providerdata/      # Shared Data{} carrying SDK client + cached Pro version; scope gates (scope.go, scopes.go)
├── resources/         # Every family below is a flat single tier: folder name = Terraform slug minus the family prefix
│   ├── account/       # Jamf Account, organization-level (`jamfplatform_account_`)
│   │                  #   platform_environment/, sso_connection/, sso_domain/, tenants/ (plural DS only)
│   ├── ai_governance/ # Jamf AI Governance (`jamfplatform_ai_governance_`): policy/, tool/
│   ├── apple/         # app_store_app/ — data source over Apple's public App Store listing (not a Jamf API;
│   │                  #   SDK standalone `appstore` package, per-run cache in providerdata/appstore.go)
│   ├── blueprints/    # blueprint/, blueprints/, component/, components/    (Platform Services)
│   ├── cbengine/      # benchmark/, benchmarks/, baselines/, rules/         (Platform Services)
│   ├── device/        # Single device data source                            (Platform Services)
│   ├── device_group/  # Resource, data source, list resource                 (Platform Services)
│   ├── device_groups/ # Plural data source                                   (Platform Services)
│   ├── devices/       # Plural data source                                   (Platform Services)
│   ├── security_cloud/ # Jamf Security Cloud (`jamfplatform_security_cloud_`)
│   │                  #   activation_profile/, content_categories/, device_group/, dns_hostname_mappings/,
│   │                  #   dns_search_domain/, dns_zone/, uem_connect/, ztna_app/, ztna_gateway/,
│   │                  #   ztna_grouped_gateway/, ztna_predefined_apps/, ztna_shared_gateways/
│   └── pro/           # Jamf Pro (`jamfplatform_pro_`): one leaf package per construct, directly under pro/.
│                      #   No domain tier and no grouping dirs (the PKI constructs are pki_adcs/, pki_venafi/, pki_digicert/, …).
├── actions/
│   ├── account/       # platform_environment/ (assign_tenants), sso_domain/ (verify)   (Jamf Account)
│   ├── device/        # erase, restart, shutdown, unmanage                       (Platform Device Actions API)
│   ├── pro/           # app_installers/, jamf_protect/, maintenance/, managed_software_updates/, mdm/ (device MDM commands), patch/   (Jamf Pro)
│   └── security_cloud/ # uem_connect/ (deploy_activation_profile)                (Jamf Security Cloud)
├── functions/         # Provider-defined functions (offline; no SDK client, no provider config)
│   ├── mobileconfig/       # mobileconfig(profile) — build a full .mobileconfig from HCL payloads; also holds the shared Assemble core
│   └── mcx_forced_payload/ # mcx_forced_payload(domain, prefs) — MCX "Custom Settings" envelope; thin wrapper over mobileconfig.Assemble
├── common/
│   ├── accountprivileges/ # Privilege-grid model shared by pro account and account_group
│   ├── accounttenant/ # Tenant-list lookups shared by the platform environment resource and assign_tenants action
│   ├── actiontimeouts/ # Context deadline + `timeouts` attribute for every action
│   ├── aischemas/     # Vendor JSON Schema fetch/cache/validate for AI Governance policy settings
│   ├── availabletitles/ # Shared patch available-titles lookup (patch_external_source, patch_internal_source)
│   ├── collision/     # Plan-time name collision check (create or rename onto an existing object)
│   ├── computertarget/ # Resolves a Jamf Pro computer id from the serial / management-id / udid selector of device-targeting actions
│   ├── criteria/      # Shared smart-group / advanced-search criteria operator vocabulary
│   ├── deprecations/  # Single reader of the schema-deprecation removal markers (sweep + conformance test)
│   ├── egressip/      # The host's public source address, for Jamf-side IP allowlist diagnostics
│   ├── enumguard/     # Recurrence guard: no enum value or error code restated as a literal the SDK generates
│   ├── files/         # Shared upload-source plumbing for resources that upload file content
│   ├── filesources/   # Cloud storage and authenticated-https upload sources for package_file_source: scheme parsing, the S3 and Azure Blob drivers, host-scoped https credentials, the resolver the provider builds from file_sources
│   ├── filters/       # RSQL + classic filter schema/expression builder
│   ├── groupcollision/ # Name uniqueness namespaces for the Jamf Pro group constructs and jamfplatform_device_group
│   ├── helpers/       # Type conversions, polling, timeout, state reconciliation, dynamic JSON, IDs, Pro version
│   ├── impact/        # Plan-time impact alerts — device counts for scope changes (see §Impact alerts)
│   ├── invitationcommon/ # Shared enrollment-invitation helpers (computer_invitation, mobile_device_invitation)
│   ├── jsonvalue/     # Rendering a decoded JSON value for a diagnostic (aischemas)
│   ├── ldapgroups/    # Directory-service (LDAP / cloud-IdP) group resolution + scope preflight validation
│   ├── listskip/      # Reports items a list resource left out because they could not be hydrated
│   ├── listtimeouts/  # Context deadline + `timeouts` attribute for every list resource
│   ├── payloadhelpers/ # mobileconfig mask / compare / identifier injection / import gate (see its doc.go)
│   ├── permissions/   # Required-Jamf-permission tables rendered into schema descriptions (see §Required permission tables)
│   ├── planmodifiers/ # Shared Terraform Plugin Framework plan modifiers
│   ├── plisthelpers/  # Generic plist (Apple property list) parsing / normalisation helpers
│   ├── prestagealias/, prestageskip/ # Attribute-rename rules shared by the two PreStage resources
│   ├── progroups/     # Machinery shared by the four Jamf Pro device-group constructs
│   ├── scope/         # Classic scope sub-block factories + builders + validators (see contributing/scope.md)
│   ├── securitycloudtenant/ # Resolves the Security Cloud tenant ID a ZTNA gateway defaults `tenant_ids` to
│   └── validators/    # Shared Terraform Plugin Framework validators (unique-string-field across collections)
└── testhelpers/       # Acceptance fixtures (provider factories, real client, mock server)
tools/                 # go:generate entrypoint (copywrite, terraform fmt, tfplugindocs)
local-testing/         # Manual API request workflows for development (gitignored)
examples/{provider,resources,data-sources,list-resources,actions,functions}/
templates/guides/      # Source of the registry guides; `make generate` renders them into docs/guides/
docs/                  # Auto-generated provider documentation — do not hand-edit
contributing/          # The style guide, split by concern (see Companion Docs)
```

Each leaf resource folder follows the file split in [contributing/package-layout.md §Resource Package File Conventions](contributing/package-layout.md#resource-package-file-conventions).

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
| Data source whose upstream is not Jamf (no scope gate, no permissions table, injected logger, per-instance cache + concurrency cap) | `internal/resources/apple/app_store_app/` |

## Impact alerts — orientation

`internal/common/impact/` produces Jamf Pro's **impact alert notifications** during `terraform plan`: an advisory warning on each object whose scope or payload is changing, reporting how many devices the change reaches. Off by default behind the provider's `impact_alerts` attribute; a nil `*impact.Cache` means disabled, so resources need no flag check, and an unreadable tenant yields one notice and never fails a plan. Three channels, each with one entry point: **deployable** objects (`impact.ReportPlan`), **scopeable** objects such as groups (`impact.ReportMembership`), and **policy dependencies** such as scripts and packages (`impact.ReportDependencyPlan`, which needs a lazy whole-tenant policy sweep capped at 5 concurrent reads). The shared Jamf Pro scope block goes through `scope.BuildImpactScope`, the single place the narrows/broadens/counts classification lives. Two rules that are easy to get wrong: a numeric Jamf Pro group id is unique only **within an estate**, and group membership is expressed in **device management identifiers**, so a numeric device, building or department id must be resolved through the inventory before it is compared with a group's members. The design is in the package's doc comments; user guidance, including the `terraform plan -json` caveat, is in `docs/guides/impact-alerts.md`.

## Required permission tables — orientation

`internal/common/permissions/` renders the **Required Jamf permissions** table into every construct's schema description: for each SDK method a construct calls, the section and permission name an operator ticks in Jamf Account, plus the `{capability}:{action}` identifier. Each package declares its calls in `permissions.go`, and `permissions_test.go` holds it to the SDK privilege registry. The capability-to-picker-row mapping exists only in Jamf's [Jamf Pro permissions map](https://developer.jamf.com/platform-api/reference/jamf-pro-permissions-map) article, so **the article is the source of truth**: `catalogue.go` transcribes it in full, `permissions-map.md` is a verbatim snapshot (`make permissions-map` refetches it), tests fail the build when the catalogue disagrees with the snapshot or the SDK requires a capability the catalogue lacks, and `.github/workflows/permissions-map.yml` refetches on a schedule and opens a pull request. How the guards work is in the doc comments of `catalogue.go` and `permissions_map_test.go`. What nothing here can prove is that the article matches what Jamf Account's picker actually prints. The same pattern lives in `jamfplatform-go-sdk` (capability and actions only, as a privilege oracle) and in `jamf-cli`.

## Apple profile schemas — orientation

The SDK package `appleschemas/profiles` (`github.com/jamf/jamfplatform-go-sdk`, moved there from this
repository in SDK v1.4.0, SDK issue #97) carries a generated table of Apple's configuration profile
payload keys, built from the `mdm/profiles` directory of apple/device-management by the SDK's
`make apple-schemas`. This repository holds no table, generator or refresh workflow; it only wires
the SDK's validator into the blueprint resource. The table is the **union** of Apple's `release` branch and its newest
`seed_OS_*` (pre-release) branch, because Jamf's generative-declarations service tracks seed and
offers seed-only keys in the UI immediately, so a release-only table would reject configurations
that work — which is why the target *fails* rather than quietly building from release alone when
seed discovery finds nothing. Jamf's blueprints service validates a stored legacy payload against
the same vocabulary, and wire probing established exactly how: an unknown **payload type** is
rejected and matched case-sensitively; an unknown **key** is silently discarded; a key differing
only in case is silently stored under Apple's spelling; a wrong value type or a missing required key
fails the write; enum and range constraints are **not** enforced (`AlertType: 99` stores fine), so
the table does not check them either. `profiles.Validate` turns those rules into `Problem`
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
type-checked. The blueprint resource wires this in through `validators.go` for
`component_blocks[].legacy_payloads`. Freshness is a
scheduled concern, not a plan-time one: the SDK's `.github/workflows/apple-schemas.yml` regenerates
**daily** and opens a pull request there — daily rather than monthly because an unrecognised name
is an error, so a stale table blocks a working configuration instead of merely warning about one.
A refresh reaches a user in three further steps, none of them automatic end to end: the SDK
maintainers tag a release (tags are pushed by hand), Dependabot opens the bump here (the root
`gomod` entry checks **daily** for that reason), and a provider release ships it — which is why
CONTRIBUTING.md §Release Versioning requires bumping the SDK to its latest tag before tagging.
`raw_component` covers the window. User guidance is in `docs/guides/apple-schema-validation.md`.

## Apple declaration schemas — orientation

The SDK package `appleschemas/declarations` is the same shape for **declarative device management**:
a generated table of Apple's declaration types, built from the `declarative` directory of
apple/device-management by the same SDK `make apple-schemas` run, so a legacy payload and the
declaration wrapping it can never be validated against different upstream revisions. It is
deliberately a sibling of `appleschemas/profiles` rather than an extension of it, because the two services
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
the EU gateway, 2026-09-10 — and `declaration_kind_test.go` in the blueprint package is the
tripwire for the day a spec ingest catches up (the SDK carries none). The blueprint resource wires this in through `declaration_validators.go` for both
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

## Blueprint legacy payload identifiers — orientation

A legacy payload's `payloadIdentifier` is the blueprints service's to assign, not the provider's. A payload sent without one is stamped with a fresh UUID on every write, and a merge-patch replaces `payloadContent` wholesale, so a blueprint update is **read-merge-write**: `readStoredLegacyPayloadIdentifiers` fetches the blueprint first and writes each stored identifier back, matched by step (by name ahead of position) and then by `payloadType`. Create sends none, an authored value is discarded, and a failed pre-read fails the apply. The reason is parity with the web app, **not** a device concern: identifiers rotating or repeating across steps or blueprints is inert on the device. The one fatal shape is two payloads in a **single step** sharing an identifier, which makes macOS refuse the whole profile while the blueprint reports success; `dropDuplicateIdentifiers` drops such an identifier and warns. The wire evidence for all of this is in the doc comments in `internal/resources/blueprints/blueprint/helpers.go`. Two things recorded only here: the injection is wired into `legacy_payloads` alone, so a legacy profile authored as `raw_component` has its identifiers rotated on every write; and the documentation deliberately says nothing about any of it, because the field is the service's and an operator has nothing to do with it.

## Jamf Security Cloud resources — orientation

Terraform construct name format: `jamfplatform_security_cloud_<resource>`; Go package `internal/resources/security_cloud/<resource>/`, flat single tier like `pro/`. All five Security Cloud API namespaces are generated into one SDK package, `jamfplatform/securitycloud`, routed through the unified `/securitycloud` prefix with no `/api` segment. Full rules and the shapes that recur: [contributing/families.md §Jamf Security Cloud Resource Naming](contributing/families.md#jamf-security-cloud-resource-naming); user guidance: `docs/guides/security-cloud.md`. The traps:

- **Configure goes through `providerdata.ConfigureSecurityCloud`, not `ConfigurePro`.** Security Cloud has no customer-tenant version, and a tenant can hold it without holding Jamf Pro.
- **Entitlement is not authentication.** A valid integration can be refused `403 NOT_ENTITLED`; resources translate that code into a named diagnostic.
- **`403 BAD_PERMISSIONS` is never translated into a diagnostic.** An unmapped route answers it too, so it is indistinguishable from a privilege gap, and a spec-advertised endpoint the gateway 403s on is presumed unrouted.
- **Cross-namespace references are server-enforced, and delete behaviour is per construct.** A DNS zone cannot be written before its gateway exists (`422 GATEWAY_NOT_FOUND`); a referenced gateway refuses delete with a bare `409 CONFLICT`; a referenced device group deletes cleanly and silently empties the referencing app's assignment. Probe the referenced delete for each new construct.
- **Enums are validated at plan time from the SDK's `*Values()` helpers**, because the server's enum errors are unattributed and differ per service (`400 INVALID_FIELD` from `jsc-dns` and `jsc-ztna`, `422 VALIDATION_FAILED` from `uem-connect`). Do not write a diagnostic against a code another construct observed.
- **Acceptance tests gate on `testhelpers.AccPreCheckSecurityCloud`**, which requires the scope to be declared as a Security Cloud one and skips otherwise (see TESTING.md).

## Jamf Account — orientation

Terraform construct name format: `jamfplatform_account_<x>`; Go package `internal/resources/account/<x>/`, flat single tier. This is the provider's **organization-level** family, what a practitioner manages at *account.jamf.com → Organization*: SSO domains (`sso_domain`) and OIDC connections (`sso_connection`), platform environments (`platform_environment`, plus the `assign_tenants` action), and a read-only list of the organization's tenants (`tenants`, plural data source only). Full rules: [contributing/families.md §Jamf Account Resource Naming](contributing/families.md#jamf-account-resource-naming); user guidance: `docs/guides/account-sso.md`; wire evidence: the package doc comments. The traps:

- **Organization scope only, and only this family is reachable under it.** Gated once in `providerdata.ConfigureAccount`. No organization ID travels anywhere; the gateway resolves it from the access token. The SSO namespace is served from the **US gateway** alone. Environment scope is untested, not ruled out, so widening the gate is a fresh probe.
- **An SSO domain is create/verify/delete only.** `GET`, `PUT` and `PATCH` by id are unrouted, so every attribute is `RequiresReplace`, Read scans the list endpoint, and import is by domain **name**.
- **Verification is an action, not resource state, and it is not idempotent.** A failed verify returns `200` with the status unchanged, yet bumps `lastModifiedDate` and pushes the expiry out. The five-minute rate limit is measured from `lastModifiedDate`, which the claim itself sets, so the first verify after creating a domain is always refused.
- **A connection's tenant allow-list is write-only.** Neither read shape echoes `enabledProducts` or `enabledEnvironments`, so it cannot be drift-checked.
- **A connection cannot be changed in place.** `PUT /sso/v1/connections/{id}` answers `500 UPSTREAM_ERROR` for every request and applies nothing, so `sso_connection` carries a whole-resource `ModifyPlan` that plans a replacement on any configured change. It is one file so that reverting it is deleting a file; `Update` and its acceptance test are already written behind `skipUnlessConnectionUpdatesWork`.
- **The server attributes almost nothing.** `errors[].field` is populated only for top-level required fields and an invalid enum never names the value, so every enum comes from the SDK's `*Values()` helper.

## Jamf AI Governance — orientation

Terraform construct name format: `jamfplatform_ai_governance_<x>`; Go package `internal/resources/ai_governance/<x>/`, flat single tier. An **AI policy** is the managed configuration for one AI tool (Claude Code, Claude Desktop or OpenAI Codex today), which Jamf Pro delivers to Macs through a blueprint's `com.jamf.ai-governance` component. Family is **Platform Services**: hand-rolled Configure, no version gate, no SDK-endpoints annotation block. User guidance: `docs/guides/ai-governance-policies.md`; validation design: the `internal/common/aischemas` package doc.

**Scope is `ScopeEnvironment` alone, and both alternatives are wire-probed.** No scope header is refused `400 REQUEST_CONTEXT_NOT_PROVIDED`; `X-Tenant-Id` is refused `403 BAD_PERMISSIONS`, while `GET /pro/v1/csa/tenant-id` under that same header answers 200, so the refusal belongs to this namespace. By this repo's law that reads as unrouted, and widening the gate is a fresh probe. This retires the Phase 11 epic's recorded blocker, which had the namespace down as organization-scope and unbuildable. The path is `/ai/governance/policies/v1/...`, not `/api/ai-governance/...`.

Four things drive the design:

- **The settings body is the tool vendor's own JSON**, declared by a JSON Schema the platform serves per tool per schema version (142 top-level properties for Claude Code, deeply nested for Codex). So it is one `settings_json` string with JSON semantic equality, never generated typed attributes: schema versions coexist per policy and `anyOf` unions have no framework equivalent. `aischemas` fetches, caches and validates it at plan time.
- **Validation strength is a property of the vendor schema, not the service.** An undeclared key is silently stored and never applied where the schema allows extras (Claude Code) and rejected with `422` where it does not (Codex). That is why an unrecognised key is a warning and everything else an error.
- **A policy has a draft and a published history.** Create publishes nothing, a blueprint pins a version number, and an apply changing only the name mints no version. `schemaVersion` is part of neither diff, so moving a policy to a newer schema without touching the settings publishes nothing and leaves blueprints delivering the old schema.
- **Archiving is not blocked by a blueprint that references the policy.** The delete succeeds and leaves the blueprint pointing at a version the platform will no longer serve, with no reverse lookup to warn from, so that trap is documentation only. It is the opposite of the ZTNA gateway behaviour: probe referenced deletes per construct.

## Jamf Pro resources — orientation

Terraform construct name format: `jamfplatform_pro_<resource>` regardless of whether the SDK source is `pro/` or `proclassic/`. One `jamfplatform.Client` built from `JAMFPLATFORM_*` credentials serves both Platform Services and Pro. Every Pro construct declares an unexported `const minJamfProVersion` (the per-construct floor; `""` unless an endpoint needs a newer version, and a tenant below it is a Configure error) and funnels Configure through `providerdata.ConfigurePro` or `ConfigureProClassic`, never hand-rolled. Separately, `ProviderMinJamfProVersion` is the provider's build target: it tracks the SDK's `jamfplatform.JamfProAPIVersion`, is not the maximum of the floors, and a tenant below it gets a warning. Each Pro resource's `crud.go` opens with an SDK-endpoints annotation block ending in a `Status:` line (`Status: current.`); Platform Services resources are exempt. Full rules: [contributing/families.md §Jamf Pro Resource Naming](contributing/families.md#jamf-pro-resource-naming) and §Minimum Jamf Pro version check; [contributing/endpoint-lifecycle.md](contributing/endpoint-lifecycle.md) for endpoint adoption, migration and schema deprecation. Workflow for adding a Pro resource (incl. SDK-comparison + ProClassic payload audit gate): [CONTRIBUTING.md §Adding a Jamf Pro Resource](CONTRIBUTING.md#adding-a-jamf-pro-resource).

## API integration scope — orientation

Jamf offers three scopes when an API integration is created, and the provider mirrors all three: **Platform environment** (preferred; `environment_id` → `X-Environment-Id`), **Tenant** (legacy; `tenant_id` → `X-Tenant-Id`), and **Organization management** (no scope header; the gateway resolves the context from the access token). `environment_id` and `tenant_id` are mutually exclusive and both optional; `internal/provider/scope.go` resolves which is in play, and `providerdata.New` reads the scope back off the built client so the gate cannot disagree with the header sent. Full rules: [contributing/scope.md §API integration scope](contributing/scope.md#api-integration-scope-every-construct-declares-which-scopes-reach-it); the evidence is on the tables in `internal/providerdata/scopes.go`. What to remember:

- **Reachability is enforced per construct, at Configure**, via `providerdata.RequireScope`, because the answer differs per family: Jamf Pro and Security Cloud work under environment or tenant scope (gated once in `configureSub` and `ConfigureSecurityCloud`); Blueprints, Compliance Benchmarks and AI Governance are environment-only (AI Governance reads as organization-level and is not); `jamfplatform_account_*` is organization-only, gated in `ConfigureAccount`, and organization scope is rejected everywhere else.
- **Call sites never write the scope kinds out.** They pass a derived family set from `scopes.go`, resolved from the SDK privilege registry's `MethodPrivileges.Scopes`. A construct needs the intersection across its methods, not the union.
- **A spec-declared set is not always what the gateway serves.** `gatewayWidenings` in `scopes.go` records each family, scope and dated wire evidence; entries are deleted, not edited, once the registry catches up, and `scopes_test.go` enforces that.
- **`403 BAD_PERMISSIONS` classifies nothing on its own.** Blueprints and Compliance Benchmarks are not widened because Jamf Account's permission picker does not offer those capabilities to a tenant-scoped integration, not because of the 403. That is falsified by a change to the picker, not by a `200`. It is verified live to produce the named Configure diagnostic in 0.25s where the same run previously spent 134s applying before the 403.
- **Scope is documented as well as enforced.** `permissions.Section` opens each "Required Jamf permissions" block with the integration scope from the same registry, reporting the spec's declaration and deliberately not the widenings.

## Tooling

- Go >= 1.27.4, Terraform >= 1.13.0.
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
| `deprecation-sweep` | List schema deprecations whose `remove-after` date has arrived (`scripts/deprecationsweep`; `TODAY=YYYY-MM-DD` looks ahead) |
| `permissions-map` | Refetch `internal/common/permissions/permissions-map.md` from Jamf's permissions map article (network; not part of `generate`) |
| `test` | Unit tests (excludes `acceptance` build tag) |
| `test-scripts` | Unit tests for `scripts/acctargets`, `scripts/acclanes`, `scripts/deprecationsweep` and `scripts/skipkeycanary` (each behind its own build tag, so `go test ./...` misses them) |
| `testacc` | Acceptance tests (sets `TF_ACC=1`, requires tenant) |
| `testacc-run` | Targeted acc rerun (`RUN=<regex> PKG=<path>`) |

Before committing: `make fix fmt lint test`. Then `make generate` if any schema description or example changed.

## Environment Variables

Read by the provider at Configure and documented for users. **Never rename these**; the SDK alignment happens at the secret layer, with `acceptance.yml` mapping the aligned secret onto the provider's own variable.

- `JAMFPLATFORM_BASE_URL` — `https://us.api.jamfcloud.com` / `eu.api.jamfcloud.com` / `apac.api.jamfcloud.com`. The gateway root, host only: it serves `/auth/token` and every namespace at the root, so a `/api` path breaks authentication. The pre-GA beta `{region}.apigw.jamf.com` still resolves and still answers the token exchange, but serves no API namespace, so the provider refuses it at configure time.
- `JAMFPLATFORM_CLIENT_ID` / `JAMFPLATFORM_CLIENT_SECRET` — API client credentials.
- `JAMFPLATFORM_ENVIRONMENT_ID` — platform-environment scope, sent as `X-Environment-Id`. **Preferred.**
- `JAMFPLATFORM_TENANT_ID` — tenant scope, sent as `X-Tenant-Id`. **Legacy.** Mutually exclusive with the above; both are optional — see §API integration scope.

Acceptance tests only. Requirements, skip rules and the CI wiring for each are in [TESTING.md](TESTING.md); acceptance also needs `TF_ACC=1` (set by `make testacc`).

- `JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID` — declares that the credentials are an organization-scoped integration, which `jamfplatform_account_*` requires. Nothing else can tell one from a Pro-only credential set with both scope variables unset. `AccPreCheckAccount` also requires both scope variables unset and skips on a non-US base URL.
- `JAMFPLATFORM_ACC_ORGANIZATION_SSO_VERIFIED_DOMAIN` / `JAMFPLATFORM_ACC_ORGANIZATION_SSO_UNVERIFIABLE_DOMAIN` — optional; a real, already-verified claimed domain and a throwaway `.example` one, covering the two verification outcomes the suite cannot manufacture. The first-time unverified→verified transition is untestable by design: the verification key is reissued on every claim, so automating it would require the suite to write DNS.
- `JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID` — declares that the environment holds Jamf AI Governance; must equal `JAMFPLATFORM_ENVIRONMENT_ID`. Environment scope only.
- `JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID` / `JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID` — declares that the configured scope belongs to a Security Cloud tenant; must equal the corresponding `JAMFPLATFORM_*` value. The environment form is wired in CI; the tenant form is not. The ZTNA gateway tests need no tenant fixture: they leave `tenant_ids` unset and the provider defaults it (`internal/common/securitycloudtenant`).
- `JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID` — client ID of a pre-existing Jamf Pro API client holding the AD CS certificate-job privileges, for `jamfplatform_pro_pki_adcs` in `OUTBOUND` mode. The provider cannot create one: the API-client endpoints were withdrawn at the Platform API GA. Unset and the two OUTBOUND tests skip.
- `JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_BUCKET` — names a fixture bucket, with AWS credentials on the default chain that can write to it, for the `jamfplatform_pro_package` tests that read `s3://` (`s3_source_acceptance_test.go`, `pro` lane). Unset and those tests skip, in CI too: no `require` token promotes it, because a bucket is infrastructure outside Jamf. Optional companions (`_S3_PREFIX`, `_S3_REGION`, `_S3_ENDPOINT`, `_S3_ASSUME_ROLE_ARN`, `_S3_PROFILE_A` / `_B`) are tabled in TESTING.md. `JAMFPLATFORM_ACC_CLOUDSTORAGE_AZURE_ACCOUNT` and `_AZURE_CONTAINER` do the same for the `azblob://` tests, which authenticate with the `ARM_*` variables or an Azure CLI login and are not wired in CI.
- `JAMFPLATFORM_ACC_REQUIRE` — comma-separated lane `require` tokens (`platform`, `environment`, `organization`, `securitycloud`, `aigovernance`, `pro-tenant`). Promotes an unset-credential skip into a failure for the named sets; `acceptance.yml` sets it per lane. Unset locally, everything skips as before.
- Every other acceptance-only variable is named `JAMFPLATFORM_ACC_<PRODUCT>_<FIELD>`, matching `jamfplatform-go-sdk` verbatim where that repo already names the same secret, so one value serves both. The rename map and the dual-read shim that keeps a stale local `.env` working are in `internal/testhelpers/accrequire/env.go`.

## Acceptance CI lanes — orientation

`.github/workflows/acceptance.yml` runs acceptance as **product lanes**, not one serial suite. `.github/acceptance-lanes.json` is the single source of truth, read by `scripts/acclanes` (which builds the GitHub matrix) and by `internal/conformance/acc_lanes_test.go` (which proves the partition is honest in `make test`, with no credentials). A lane is a product space and membership is a **package path prefix**; `pro` is the default lane that owns everything unclaimed, and lanes reserved with `planned: true` must match zero packages. Full rules: [TESTING.md §CI scaling](TESTING.md#ci-scaling). Three things drive the design:

- **Only `pro` holds the estate lock.** The other lanes use a different credential set or entitlement declaration and share no fixtures with it, so a short lane no longer queues behind the multi-hour Pro suite.
- **A running suite is never cancelled.** There is deliberately no workflow-level `concurrency`: cancelling SIGKILLs `go test`, no `CheckDestroy` or `t.Cleanup` runs, and orphans on a shared estate later collide by name. Superseded runs queue, and a `Skip if superseded` step lets a stale one exit in seconds.
- **Absence of a credential is fatal in a pipeline and a skip locally**, via `JAMFPLATFORM_ACC_REQUIRE`. The failure this closes is the invisible one, where a missing secret makes a suite self-skip and the package still prints `ok`. It was not hypothetical: `acceptance.yml` referenced 48 secrets of which 13 existed, and the AI Governance declaration was read by 18 tests and referenced nowhere. A lane that plans packages and executes zero tests now fails with a warning banner, and one `acceptance-gate` job is the required check so branch protection never has to name a lane.

## Copyright headers

Every Go file carries `// Copyright Jamf Software LLC <year>` + `// SPDX-License-Identifier: MPL-2.0`. Managed by `copywrite` via `make generate`. 2026
