# Style Guide

Code style conventions for the Terraform Provider for Jamf Platform.

## Go Conventions

- Follow standard Go conventions and idiomatic patterns.
- Run `go fmt ./...` and `golangci-lint run` before committing.
- Use clear, descriptive names for variables, functions, and types.
- Every exported constant, function, variable set, and type must have a short comment describing its purpose.
- Do not add comments inside type definitions or function bodies.

## Dependencies

Allowed:

- Go standard library.
- `golang.org/x` packages.
- HashiCorp Terraform Plugin packages: `terraform-plugin-framework`, `terraform-plugin-framework-timeouts`, `terraform-plugin-framework-validators`, `terraform-plugin-go`, `terraform-plugin-log`, `terraform-plugin-testing`.
- `github.com/Jamf-Concepts/jamfplatform-go-sdk` — required for all Jamf Platform API access (auth, HTTP transport, request/response types).
- `howett.net/plist` (BSD-2-Clause) — Apple plist parser/serialiser. Required by `internal/resources/pro/macos_configuration_profile/` to compare user-supplied `.mobileconfig` payloads against the server-canonical form for diff suppression. Use is contained to the configuration-profile resource family; no other code path should import it.

Do not introduce other third-party dependencies without prior discussion.

## Resource Package File Conventions

Resource packages live under `internal/resources/<domain>/<resource>/` and use resource-agnostic filenames:

| File | Purpose |
|------|---------|
| `resource.go` | Schema definition and boilerplate |
| `crud.go` | Create, Read, Update, Delete, and ImportState |
| `model_types.go` | Terraform model structs |
| `schema_types.go` | Attribute type maps for `ObjectValue`/`ListValue` state |
| `mappings.go` | Lookup tables and name mappings |
| `input_builders.go` | Build API request payloads from Terraform model data |
| `state_builders.go` | Map API responses to Terraform state |
| `helpers.go` | Resource-specific helper functions |
| `state_upgrader.go` | Schema version upgraders (when bumping schema version) |
| `plan_modifiers.go` | Schema plan modifiers (if needed) |
| `validators.go` | Schema validators (if needed) |
| `list_resource.go` | List resource implementation |
| `data_source.go` | Singular (by-id / by-name) data source implementation |
| `datasource_plural.go` | Plural (list) data source implementation, when the resource also exposes one |

### Optional split-outs for complex resources

- `endpoints_builders.go` / `endpoints_state.go` — when endpoint logic dominates.
- `nested_builders.go` / `nested_state.go` — for large nested payloads.

### Data-source-only packages

Packages that only contain a data source use `model_types.go` for their model structs and `data_source.go` for the implementation.

### Plural (list) data sources

A plural data source (`jamfplatform_pro_categories`) lives **in the same package as its singular counterpart** (`category/`), not in a separate `categories/` package. Add `datasource_plural.go` for the implementation; its model structs join the package's existing `model_types.go` alongside every other construct's models. Reuse the singular package's exported schema helpers, state builders, and SDK wiring wherever the shapes overlap (see `app_installer/` and `app_installer_title/` for full reuse).

- The plural read timeout const is `defaultPluralReadTimeout` to avoid colliding with the singular's `defaultReadTimeout`; `minJamfProVersion` is shared (declared once in the package).
- Where the list endpoint returns a strict subset of the singular's fields, the plural carries its own result model (e.g. `UserGroupsDataSourceResultModel`) so the sparse shape is explicit in one package rather than implied across two directories.

### First-time hydration detection in Read

Every resource's `Read` must tell a first-time import hydration apart from an ordinary refresh, because the two need different handling for any optional, unmanaged attribute: hydration has to fill it from the wire, everything else must only reconcile it against what the config already declares. Getting the signal wrong is silent — the optional attribute stays null, goes unmanaged, and no later plan surfaces the gap, because a null value looks identical to one the config never claimed. Issue #391 closed the audit that finally bounded it: all 48 resource packages carrying an optional block or collection were read (not grepped — see below), and six more had shipped the bug, bringing the total to twenty-two.

`req.State.Raw.IsNull()` is not the signal, and checking it is exactly the mistake. It is true only on the identity-only refresh path, where Terraform supplies `req.Identity` and nothing else. It is false on the far more common `terraform import <addr> <id>` path: `ImportStatePassthroughID` seeds state with the id before Terraform ever calls `Read`, so on that post-import `Read`, `req.State.Raw` is a populated object with `id` set and every other attribute null — indistinguishable, by nullness of `Raw` alone, from a resource that simply isn't managing those attributes.

Gate on a schema-`Required` attribute instead. Convention names it `firstHydration`. Create, Update and ordinary refresh always set that attribute before `Read` runs; neither import path does, so `state.<RequiredAttr>.IsNull()` can only mean this call is doing first-time hydration. Sample it off the immutable `req.State`, never off the model `Read` is assembling, which a later assignment in the same function can overwrite. Copy a package that already computes it this way as `firstHydration := state.<Required>.IsNull()` (`account_group`, `mac_app_store_app`) rather than re-deriving it. Where no attribute is `Required`, an unconditionally-assigned `Computed` one serves the same purpose (`computer_prestage_enrollment` and `mobile_device_prestage_enrollment` use `site_id`), but prefer a `Required` field: the guarantee then comes from the schema rather than from every assignment site staying ungated.

**Releasing the outermost gate is not enough. Thread the signal into every flattener that carries an ownership gate of its own.** A nested flattener typically re-derives "did the config declare this" from a `prior` argument, and `prior` is the pre-assignment value of the parent block — nil on import, precisely because the parent gate has only just released. So the parent hydrates while the collection beneath it stays null, on that `Read` and every later one, and the parent's own fix hides the remaining hole. This is how `app_installer.self_service_settings.categories` and `mobile_device_app.self_service.{self_service_icon,self_service_categories}` survived the #279 sweep with their parents already corrected, and it is why the #391 audit read `Read` paths instead of grepping: four separate greps had each missed a different resource, because the gate is spelled `state.X != nil` in one package, `declared != nil` in another, a `bool` parameter in a third, and a stale `prior` argument in a fourth.

**Adopt an optional collection only when the server actually returns members.** Adopting an empty one stores `[]` where a create that never declared the attribute stores null, which breaks `ImportStateVerify`. Reference: `mobile_device_configuration_profile`'s `flattenSelfService`.

**Check every `ImportStateVerify: true` step on the affected packages before calling the fix complete.** Hydration is a state change, so a test whose config never declared the block previously compared null against null by coincidence and now legitimately differs. PR #279 broke six of twelve that way; #391 needed three more `ImportStateVerifyIgnore` entries and was able to *remove* two.

**A gated block holding a `WriteOnly` secret needs the same release, and for a sharper reason than a cosmetic diff.** `pki_adcs`, `pki_digicert`, `sso_settings` and `user_initiated_enrollment_settings` gate a certificate block whose `keystore_file` / `password` / `wo_version` Read never sees. Their rotation predicates return "rotate" whenever the prior block is nil — always true after an import — so the first post-import apply re-uploads the certificate *whether or not the practitioner touched the rotation trigger*, and when the trigger is left undeclared there is no diff in the plan to say so.

Releasing the gate fixes that, and no change to the predicate is wanted. `wo_version` is a regular `Optional` attribute held in state, so once the block hydrates the predicate falls through to comparing it: a config declaring `wo_version = 1` plans a visible `+ wo_version = 1` against the null in state and re-sends, which is exactly what the trigger is documented to mean; a config omitting it compares null against null and sends nothing. Measured on `ldap_server` (2026-09-07), whose predicate has the same shape.

Do **not** "fix" this by treating a null prior `wo_version` as "no rotation signal". That either records a version that was never sent — permanent, because Read cannot correct it — or leaves a perpetual diff, and it contradicts HashiCorp's trigger semantics. Allocate such a block **only when the server actually reports a certificate**, and document the `wo_version` diff in the resource's Import section.

Two limits on that, both found by doing it (#399, #403). **Hydration only prevents a re-send where the rotation trigger is not obliged to be declared.** Where a validator pairs `wo_version` with the secret it guards — `pki_adcs`, `sso_settings` in `UPLOADED` mode, both `user_initiated_enrollment_settings` blocks — the trigger has no server echo to hydrate from and the configuration must supply one, so the post-import re-send is unavoidable and hydration buys legibility alone. The pattern that actually removes it is the one `sso_settings` already uses for `GENERATED`: **read the wire for prior state** (`currentCertSetupType`) rather than trusting Terraform state, and compare against the metadata the server does return.

And **hydration is unsafe wherever what it writes into state drives a destructive write.** Two shapes have shown up so far, and both are the same mistake: state that records the server rather than the configuration, read back later as if the practitioner had asked for it.

*Where omitting the block deletes the secret.* `sso_settings` guards that with `plan.SigningCertificate == nil && state.SigningCertificate == nil`, so populating state on import stops the guard firing and a practitioner who imports without declaring the block hits the transition table's `GENERATED → nil = DELETE`. Check what omission does on the wire **before** hydrating any write-only block, and pin the answer in a test — `TestAssignSigningCertificateState_NeverHydratesAnUnauthoredBlock` exists because a later uniform sweep would otherwise reintroduce that delete.

*Where prior state is the diff that decides what to clear.* `patch_software_title.version_packages` is a managed-subset map over a full-replacement `packages` array, so `Update` folds the plan over the live server set and reads a **prior-state key missing from the plan** as an explicit unassign. That is correct only while every key in state is a key the configuration declared. Hydrating the server's assignments broke it: the settle plan's apply unassigned every version the configuration did not mention, wiping the packages a patch policy deploys (#403, regressed from this very sweep). So the resource hydrates nothing there and wears the cosmetic addition diff instead, pinned by `TestManagedVersionPackages_NeverHydratesUndeclaredAssignments`. Before hydrating any collection, ask what else reads it: if a write path diffs it against the plan to decide what to remove, hydration turns "the server has this" into "the practitioner asked to delete this".

The concept currently ships under three names: `firstHydration` in those thirteen, `importing` in `security_cloud/uem_connect`, and a shared `importHydration` predicate feeding a `hydrating` local in `device_group` and `pro/account`. Converging them onto one helper in `internal/common/helpers`, with an AST guard in `internal/conformance` to keep a new resource off `req.State.Raw.IsNull()`, is open work. Until it lands, prefer `firstHydration` in anything new.

## Provider Function File Conventions

Provider-defined functions live under `internal/functions/<function>/`. They are **offline**: called before provider configuration is evaluated, so they take no SDK client and no credentials (the framework does not expose provider config to a function). Each package uses function-agnostic filenames:

| File | Purpose |
|------|---------|
| `function.go` | The `function.Function` implementation: `Metadata`, `Definition` (parameters + return), and `Run`. Also the argument decode + `Profile`/input mapping. |
| `<core>.go` | The offline core that does the actual work (e.g. `assemble.go`, `render.go`) — pure Go, no framework types, independently unit-testable. |
| `function_test.go` | Unit tests for `Definition`/`Metadata` and the input mapping. |
| `<core>_test.go` | Unit tests for the core, including golden output where determinism matters. |
| `function_acceptance_test.go` | `//go:build acceptance` — drives the function through real Terraform (see below). |

Conventions:

- **Decode `types.Dynamic` via `helpers.TerraformDynamicToJSON`.** Declare each argument as a `function.DynamicParameter` and decode with `req.Arguments.Get` → `helpers.TerraformDynamicToJSON` → a `map[string]any`. `DynamicParameter` (not a typed `ObjectParameter`/`ListParameter`) is deliberate: it lets a heterogeneous input (e.g. a `payloads` list whose objects have different key sets) decode as a cty tuple instead of being rejected for non-uniform element types. Guard the top-level type-assert and return `function.NewArgumentFuncError(argIndex, …)` on mismatch.
- **Keep the core framework-free.** The `Run` method should decode, delegate to the offline core, and set the result — nothing else. The core returns `([]byte, error)` / a plain value so it can be unit-tested without the framework.
- **Share a core between related functions** rather than duplicating the logic. Today `mcx_forced_payload` imports the `mobileconfig` package for `Assemble`/`Profile`, because `mcx` is conceptually a specialisation of `mobileconfig`. If a third function needs the same core, lift the neutral assembler into its own package (e.g. `internal/functions/profileassembler`) that each function package imports, rather than importing one function's package from another.
- **Register** in `internal/provider/provider.go` via the `Functions()` method (add it if this is the first function).

### Testing a provider function

Cover both sides of the framework seam:

- **Core unit tests** feed Go values directly to the core (`Assemble`, `render…`) — fast, exhaustive, and where golden-output pinning lives.
- **`Run` seam tests** build a real `types.Dynamic` and call `Run` through `function.NewArgumentsData([]attr.Value{…})`, asserting both the happy path (result set, no error) and at least one argument-error path (e.g. a non-object argument yields a `FuncError`). This is the path Terraform actually invokes; core tests bypass it.
- **Acceptance test** (`//go:build acceptance`) invokes the function from a real Terraform config through an `output`, and asserts the rendered string with `statecheck.ExpectKnownOutputValue` + `knownvalue.StringRegexp`. Because functions are offline, the function acceptance test **must not** call `testhelpers.AccPreCheck` (that gates on tenant credentials and would skip) — call `testhelpers.AccPreCheckOffline(t)` instead, which sets `TF_ACC` without the credential gate so the test runs (rather than silently skipping) under a raw `go test -tags=acceptance`. Use `ProtoV6ProviderFactories` directly and gate only on Terraform version with `tfversion.SkipBelow(tfversion.Version1_8_0)` (provider functions are GA from Terraform 1.8). Reference: `internal/functions/mobileconfig/`.

## Test File Conventions

| File | Purpose |
|------|---------|
| `schema_test.go` | Schema validation (every package) |
| `helpers_test.go` | Helper function tests |
| `input_builders_test.go` | Input builder tests |
| `state_builders_test.go` | State builder tests |
| `state_upgrader_test.go` | State upgrader tests (where present) |
| `resource_acceptance_test.go` | Resource acceptance tests (`//go:build acceptance`) |
| `datasource_acceptance_test.go` | Data-source-only package acceptance tests (`//go:build acceptance`) |
| `schema_plural_test.go` | Plural data source schema validation |
| `datasource_plural_acceptance_test.go` | Plural data source acceptance tests (`//go:build acceptance`) |

## Client Conventions

The Jamf Platform API client lives in the external SDK `github.com/Jamf-Concepts/jamfplatform-go-sdk` (package `jamfplatform`). This repository imports it; it is not vendored. To consume new endpoints, bump the SDK dep in `go.mod` (or coordinate changes upstream first).

The SDK follows the conventions below — match them when contributing upstream.

### Versioned naming

Client types and functions use explicit version suffixes to support multiple API versions:

```go
// V1 functions
func (c *Client) CreateDeviceGroupV1(ctx context.Context, req *DeviceGroupCreateRepresentationV1) (*DeviceGroupCreateResponseV1, error)

// V2 functions added alongside V1
func (c *Client) CreateCBEngineBenchmarkV2(ctx context.Context, req *CBEngineBenchmarkRequestV2) (*CBEngineBenchmarkResponseV2, error)
```

When endpoints are upgraded, add new versioned types/functions alongside existing ones. Resources migrate at their own pace.

### Type naming

Request and response types include the version suffix and follow the pattern `<Domain><Entity><Purpose><Version>`:

```go
type DeviceGroupCreateRepresentationV1 struct { ... }
type CBEngineBenchmarkResponseV2 struct { ... }
```

## Schema Guidelines

- Keep schemas inline and as flat as possible.
- Favor nested attributes (`SingleNestedAttribute`, `SetNestedAttribute`, `ListNestedAttribute`) over blocks.
- **Terraform attribute names are always snake_case**, irrespective of how the upstream API formats the underlying JSON field. Translate at the boundary in input/state builders. Examples: API `categoryId` → TF `category_id`, API `osRequirements` → TF `os_requirements`, API `parameter4` → TF `parameter_4`. The only namespace exempt from this rule is **RSQL filter selectors** (`filters.FilterModel.Selector`), which pass through to the API verbatim and therefore retain their API-native spelling.

### Attribute names mirror the Jamf Pro admin UI when the wire name is cryptic

Jamf Pro's wire payloads frequently use short, internal names that do not match what administrators see in the admin UI — `cache_last_user` is labelled "Create Mobile Account" in the AD binding screen; `mount_style` is labelled "Network Protocol"; `uid` is labelled "Map UID to attribute"; and so on. A TF user reading the provider docs should not have to translate from the wire spelling back to the UI label they know.

**Rule**: when the wire name is cryptic or differs materially from the admin-UI label, **rename the Terraform attribute to mirror the UI label** (snake_case, of course). Translate at the boundary in input/state builders — the wire name stays inside the SDK call. Examples from `jamfplatform_pro_directory_binding`:

| UI label | TF attribute | Wire element |
|----------|--------------|--------------|
| Create Mobile Account | `create_mobile_account` | `cache_last_user` |
| Network Protocol | `network_protocol` | `mount_style` |
| Map UID to attribute | `uid_attribute_mapping` | `uid` |
| Force local home directory on startup disk | `force_local_home_directory` | `local_home` (bool, AD type) |
| Home Location | `home_location` | `local_home` (string, ADmitMac type) |
| Allow administration by | `admin_group` / `admin_groups` | `admin_group` / `admin_groups` |

When the wire name is already a reasonable match for the UI label (e.g. `encrypt_using_ssl`, `use_unc_path`, `workstation_mode`), leave it alone — gratuitous renames produce churn without payoff.

**Every renamed attribute's `MarkdownDescription`** must lead with the exact UI label in bold quoted form (e.g. `**"Create Mobile Account"** in the Jamf Pro admin UI.`) so a `terraform plan` reviewer can match against the admin screen. The wire name should **not** appear in user-facing text — record it instead in a Go comment immediately above the attribute, where future maintainers searching for the wire name can find it without exposing API plumbing to end users.

```go
// Wire element: cache_last_user — renamed for UI alignment.
"create_mobile_account": optBool("**\"Create Mobile Account\"** in the Jamf Pro admin UI. …"),
```

The rename rule applies to all current and future Jamf Pro **and Jamf Security Cloud** resources — the reasoning is about the gap between a wire payload and the screen an administrator reads, which is not specific to one product. Where an existing shipped resource has cryptic wire-name attributes, do not retrofit in a feature PR — schedule a dedicated rename PR (the change is breaking for users).

**The wire name cannot go in a comment beside the attribute**, despite the snippet above: [§Go Conventions](#go-conventions) forbids comments inside function bodies, and a schema is built inside one. Put the whole wire mapping in the package doc comment instead, as a table — `internal/resources/security_cloud/ztna_gateway/resource.go` is the reference. That reads better anyway: one place to scan when chasing a field name seen in an API response, rather than a dozen scattered one-liners.

### User-facing descriptions are UI-aligned, not wire-aligned

`MarkdownDescription` / `Description` strings on every `pro_` schema attribute, resource, data source, and list resource are **user-facing Terraform Registry documentation**. They render in the registry and in the IDE schema browser, side-by-side with the Jamf Pro admin UI. The provider exists to abstract Jamf Pro's API plumbing — descriptions should reflect that abstraction, not betray it.

**Strip from user-facing descriptions** (move to Go comments above the attribute if the maintainer-side info is still useful):

- `<xml_tag>` references and any other wire-shape literals (e.g. `<level>`, `<jss_users>`, `<security><password>`).
- "Wire field …" / "wire field …" / "wire emits …" / "wire-symmetric" / "on the wire" / "wire shape" / "wire-canonical" framing.
- "classic API" / "classic endpoint" mentions, and endpoint paths (`/api/proclassic/...`, `/api/pro/v1/...`, `/JSSResource/...`).
- HTTP method names (`POST`, `PUT`, `GET`, `DELETE`) — rephrase as `create`/`update`/`read`/`delete` if the action matters to the user.
- SDK package names, SDK function names, Go type names, internal helper names.
- "Server-derived" — replace with `Returned by Jamf Pro; not user-settable` or `Read-only`.

**Keep — but rephrase without wire jargon**:

- Write-only / read-back quirks (e.g. *"Jamf Pro does not echo this value back after it is set, so the provider treats it as write-only."*).
- Field conflicts and validator notes (*"conflicts with X if Y"*).
- Special-case behaviour (`-1 means none`, `0 disables`, valid value lists).
- Cross-field relationships (*"Pair with X to set Y"*).
- Mode-dependent or singleton-resource notes (e.g. SSO OIDC vs SAML, tenant-wide singletons).

**Endpoint references and SDK annotations** belong in maintainer-side surfaces only:

- The `crud.go` `// SDK endpoints:` annotation block at the top of every Pro/ProClassic resource (see `#### Tracking — in-code annotation` below).
- Go file/function comments.
- Never in `MarkdownDescription` strings.

The reference implementations for the rewritten tone are `internal/resources/pro/macos_configuration_profile/resource.go` and `internal/resources/pro/mobile_device_configuration_profile/resource.go` — copy that voice for new resources.

#### Translating UI labels/presets to wire values

When the UI exposes a control as a fixed dropdown of presets but the wire stores a raw value (e.g. LAPS `rotation_interval` "30 days" ↔ `2592000` seconds), model the attribute as a `OneOf` enum of the **UI labels** and own the label↔value translation in the input/state builders (a `map[label]value` + its programmatic inverse). Reference: `internal/resources/pro/local_admin_password_settings/`.

Two rules for these tables:

- **Derive the documented value list from the same slice the `OneOf` validator uses** (a small `markdownValueList`-style helper), so the `MarkdownDescription` and the validator cannot drift apart — single source of truth, mirroring the version-const interpolation policy.
- **Acceptance cannot verify the table.** Writing through the map and reading through its inverse round-trips *by construction*, so a wrong-but-consistent entry (`"30 days"` mapped to the wrong number) passes every unit and acceptance test silently. Anchor at least one entry to the live wire during the build, and **wire-probe the table by driving the actual admin UI** (set each preset in Jamf, GET the stored value) — the round-trip test is not a substitute. Flag any unverified entries in the PR.

### Description and example prose: the terse-reference voice

The section above governs *what* a description may say. This one governs *how* it reads. It
applies to every `Description` / `MarkdownDescription` string in a schema — resource, data
source, list resource, action, function, provider configuration — and equally to the `#`
comments in `examples/**/*.tf`, which a user reads side by side with them on the Registry page.

The voice is terse reference: short declarative sentences, the fact first and each quirk in a
sentence of its own. Vary sentence length, so the text does not read as a template somebody
filled in.

Six habits account for most of the prose this provider has had to go back and rewrite. Do not
start them again.

- **One em dash per description at most, and only for a genuine interruption.** An em dash
  gluing an appositive or a second clause onto a sentence is the strongest single signal that a
  description was generated rather than written. `Manages Jamf Pro SMTP settings (Settings →
  System → SMTP Server) — the outbound relay …` is a comma or a full stop wanting to happen.
  Two in one description is one too many, and three is never right.
- **No bolded run-in label opening a paragraph.** `**Omit = preserve** — each toggle you omit
  keeps its current value` says no more than `A toggle you omit keeps its current Jamf Pro
  value.`, in more words and in a textbook voice. Where a long resource description really does
  change topic, use a `###` heading or a plain lead sentence. Two exceptions, both required
  elsewhere in this guide: the bold quoted admin-UI label a renamed attribute must open with,
  and `Requires **X API** access.`
- **Do not repeat a stock clause verbatim across packages.** `Singleton — one record per
  tenant.` stood identically in more than a hundred descriptions before a cleanup sweep.
  `One record per tenant.` carries it, and the next resource need not phrase it the same way.
- **En dash for numeric ranges** (`1–100`, `0–65535`); hyphen for everything else.
- **No throat-clearing.** Delete `Note that`, `Note:`, `NOTE:`, `IMPORTANT:`, `simply`, `just`.
  A description is already the place for the fact, so state it.
- **No padding verbs.** `is used to select` is `selects`. `used for` either names the use or goes.

Vocabulary that never earns its place in a description: *leverage, robust, seamless,
comprehensive, ensure, facilitate, streamline, delve, crucial, foster, showcase, underscore,
pivotal,* and *landscape* used figuratively. Nor do the rhetorical shapes that travel with them:
`not X, but Y`, a reflexive stack of three adjectives, or a closing sentence that restates the
description.

**Write as Jamf, not about Jamf.** This provider is Jamf's own, so its user-facing text does
not refer to Jamf in the third person, as a vendor the provider merely integrates with. Where a
sentence names the actor in an exchange, name the product or the service: *Jamf Pro refuses the
write*, *Jamf Account stores a claimed domain in lower case*, *Jamf Security Cloud enforces the
reference on delete*, *the platform compares the settings*. A bare "Jamf" as the actor is both
distancing and imprecise across five namespaces, and "Jamf's own", "the vendor" and "the API" are
the same tic in plainer form. Three things the rule does not touch: **the server** where the
server is one the reader runs (an LDAP directory, an SMTP relay, a distribution point, an external
patch source), **the API integration**, which is the reader's own, and a deliberate
product-versus-provider contrast, because "the deprecation is Jamf Pro's, not the provider's" is
the distinction a reader needs rather than a distancing tic. Contributor-facing text — Go
comments, this guide, `CONTRIBUTING.md` — is exempt.

**Pre-PR grep gate.** Same spirit as the jargon grep above — run it over the diff, not the tree:

```sh
git diff -U0 main -- 'internal/*' 'examples/*' | grep -E '^\+' | \
  grep -nE '—[^—]*—|\*\*[^*]+\*\*[[:space:]]*—|Singleton —|[0-9]-[0-9]|[Nn]ote that|NOTE:|IMPORTANT:| simply | just |is used to|leverage|robust|seamless|comprehensive|ensure|facilitate|Jamf.s |the API|the vendor|upstream'
```

A hit is not automatically a defect: one em dash carrying real emphasis, a hyphenated
identifier, or `ensure` inside a quoted UI label will all match. Every hit does need a reason.

### Enum values and error codes come from the SDK, not from literals

**If the SDK generates a constant for a value, use the constant.** This applies provider-wide,
to every namespace, and to two surfaces that are easy to treat as unrelated:

- **Schema value enums.** Build *both* the `OneOf` validator and the documented value list from
  the SDK's generated `*Values()` helper, so neither can drift from the other or from the API.
- **Machine-readable error codes** used to translate a failure into a diagnostic. Alias the SDK
  constant (`codeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled`), never restate the
  string.

Two shapes are legitimate and must not be "fixed" into the rule above:

- **A deliberate subset.** Where a generated set is a superset of what one caller accepts — the
  SDK says as much for `EmailMappingTypeValues()`, which spans every UEM vendor — keep the
  hand-curated list, because the curation is the point. But its *elements* are still SDK
  constants, not literals: the set is yours, the spellings are the SDK's.
- **A genuinely absent constant.** Where the SDK documents a set but generates no helper, or
  carries no constant for a code the wire sends, restate it in the package's `mappings.go` and
  say *why* in a comment.

That last exemption is where this rule keeps failing, so state the reason precisely. A comment
claiming the SDK "carries none of these codes" must be true of **every** code beneath it. Jamf
Security Cloud's `ApiErrorItemCode` is the DNS namespace's error schema and carries no
group-specific code — but it does carry the generic `INVALID_FIELD` and `NOT_ENTITLED`, so a
package restating those alongside its own codes is wrong in a way its own comment conceals.
That exact defect shipped twice and was caught by review both times.

**So do not rely on review for this — pin it with a test.** Every package that names an enum
value or an error code carries an `enum_literals_test.go` calling `internal/common/enumguard`,
which parses the package's own source and reports any string literal the package *declares* that
the SDK's `*Values()` helpers already cover. Parsing the source rather than restating a list is
what makes it self-maintaining: it fires both on a new literal that should have been an alias,
and on a literal that a future SDK release promotes into the enum. References:
`internal/resources/pro/ebook/enum_literals_test.go` (the plain shape) and
`internal/resources/pro/macos_configuration_profile/enum_literals_test.go` (exemptions with
per-value reasons).

The walker covers package-level `const` **and** `var` declarations — including the elements of a
slice or map composite literal and its keys — func-body short (`:=`) declarations, and the
vocabulary inlined into a `stringvalidator.OneOf` / `OneOfCaseInsensitive` /
`stringdefault.StaticString` call. A `const`-only parse is not enough, and that is not a
hypothetical: almost every restated enum in this provider is a `var validFoos = []string{…}`
feeding a `OneOf` validator, or is spelled out in the `OneOf` call itself, and a builder naming
the one wire value it is about to send writes it as `v := "install"`. What the guard deliberately
does **not** cover is a literal that is only *used* — compared against, returned, or passed as an
argument to anything but those two framework calls. Reaching those would mean treating every
string in the package as an enum reference, which fires on prose and on unrelated vocabularies
that share a spelling; the line is declaration versus use. Auditing the use sites is a review's
job, not the guard's.

Two exemption maps, and the difference between them is load-bearing — a rollout defect turned on
exactly this. `Absent` asserts the SDK carries **no** constant for the value, so it *is* checked
against `Covered`: an SDK release that starts generating the value fails the test and tells you to
alias it. `Ignore` asserts the value belongs to a **different** vocabulary that merely shares the
spelling, which says nothing about the sets in `Covered`, so it is *not* checked. Filing a
cross-vocabulary collision under `Absent` therefore reports a spurious promotion. Both maps take
a per-value reason, for the same reason the comment rule above demands one. An entry no literal in
the package uses is also a failure, as is a guard that asserts nothing (`Findings.Vacuous`) —
whether because its whole `Covered` set is exempted, or because `Covered` is empty, which is what
a package is left with when the SDK withdraws the only vocabulary it had. Both read as coverage
while asserting nothing.

### Sets vs Lists

- **Sets** for user-supplied unordered collections where deduplication and order-independent comparison matter (e.g. `members`, `raw_component`).
- **Lists** for (a) user-supplied collections whose **order is semantically meaningful**, and (b) computed API results that are read-only. Sets require element hashing which adds overhead with no benefit when the user doesn't control the values.

Data source attributes returning API data should always use lists.

**A server that re-sorts a collection forces a Set even when the UI looks ordered — and the same logical field can differ across the Pro/classic boundary.** Wire-probe round-trip order before choosing. Extension-attribute pop-up choices are the cautionary case: the admin UI presents an ordered add-list, but the **Pro JSON** `/v1/computer-extension-attributes` (and mobile) endpoint returns the choices **sorted alphabetically**, so a `List` perma-fails "produced inconsistent result after apply" — model it as a `Set`. The **ProClassic** `/userextensionattributes` endpoint for the *same* concept **preserves** submitted order, so the user EA models it as a `List`. Don't assume the wire matches the UI's apparent ordering, and don't assume two SDK packages treat one concept identically — probe each (`POST` `["Zebra","Apple"]`, `GET`, compare). See `internal/resources/pro/{computer,mobile_device,user}_extension_attribute/`.

**Never model ordered data as a Set.** Smart-group `criteria` is the cautionary case: the `order` field, parentheses, and `and_or` joins are all positional, yet it was first modelled as a `SetNestedAttribute`. Two failures resulted: (1) Terraform delivers Set elements to the provider in arbitrary hash order, so an index-derived `order` came out scrambled; (2) Set elements correlate plan↔state by **value**, so a positional reconcile (`prev = current[i]`) paired each server element with an unrelated hash-ordered prior-state element and emitted values matching no planned element — surfacing as `planned set element … does not correlate with any element in actual`. The fix is a `ListNestedAttribute` (the data source already modelled it as a list): lists correlate positionally so `prev[i]` lines up with `server[i]`, and the null-preserving `Reconcile*` helpers keep user-omitted fields null. See `internal/resources/device_group/`. No state upgrader is needed for a Set→List swap — both serialise to a JSON array in state — but an in-place upgrade yields a one-time, self-healing reorder plan.

### Plaintext secrets — `WriteOnly` with `_wo_version` rotation companion

New Pro resources exposing a user-supplied plaintext secret (passwords, API tokens, shared keys) **MUST** model it as `Optional + Sensitive + WriteOnly`. The plaintext is sent to Jamf on writes but never persisted in Terraform state — the framework strips it. Storing plaintext in state leaks credentials to anyone with state-file read access; "we mark it Sensitive in the schema" is not enough — Sensitive only redacts CLI output, the raw plaintext still lives in the state file.

**`WriteOnly` is also the right model for a non-secret *imperative write-instruction* — and that case does NOT take a `_wo_version`.** A field that tells the API *how to perform a write* rather than describing a persistent property of the object (e.g. `jamfplatform_pro_computer_extension_attribute.manage_existing_data` — `DELETE`/`RETAIN`, how Jamf handles already-collected inventory data when a SCRIPT EA is updated) is never returned on `GET` and is not a real attribute of the resource. Model it `Optional + WriteOnly` (not `Sensitive`, not `Computed`, no `Default`), read it from `req.Config` in Create/Update, and supply any required wire default in the input builder when config is null. **Omit the `_wo_version` companion**: there is nothing to *rotate* — the instruction simply rides along with whatever update is already happening, and "change only this value" correctly producing no diff (no update) is the right behaviour (re-issuing the instruction with no other change is a no-op). Persisting such a field as a plain `Optional` instead would store a non-attribute in state and mislead readers into thinking it describes the object. Reference: `manage_existing_data` in `computer_extension_attribute`.

Every WriteOnly **secret** **MUST** carry a sibling `<attr>_wo_version` rotation trigger:

```go
"password": schema.StringAttribute{
    MarkdownDescription: "...",
    Optional:            true,
    Sensitive:           true,
    WriteOnly:           true,
},
"password_wo_version": schema.Int64Attribute{
    MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump to force a re-PUT.",
    Optional:            true,
},
```

Why: WriteOnly values are excluded from Terraform's drift detection, so the user has no way to force a re-PUT by changing the password value alone — Terraform sees no diff. Bumping the Int64 companion is the only signal the provider has that "the user wants to rotate". Without it, the only way to rotate a stored password is `terraform destroy` + recreate. Pattern matches HashiCorp's documented `_wo_version` convention.

CRUD wiring:

- Create + Update **MUST** call `req.Config.Get(ctx, &cfg)` — the WriteOnly plaintext lives in `cfg`, not `plan` (the framework nullifies WriteOnly attrs in `plan`).
- Update **MUST** also call `req.State.Get(ctx, &state)` to compare `plan.<attr>_wo_version` against `state.<attr>_wo_version`; include the plaintext on the wire only when they differ (or thread it unconditionally if the resource's server semantics require the field on every write — e.g. `jamfplatform_pro_policy.account_maintenance`, where omitting the password erases it server-side and breaks the next client run).
- State builders **MUST NOT** preserve the plaintext across reads — the framework strips it from state regardless. The `_wo_version` companion is a regular Optional Int64 and **MUST** round-trip from prior state (the wire never echoes it).

`*_sha256` and similar server-redaction sentinels are **forbidden**:

- The classic API returns the literal string `********************` (20 asterisks) regardless of stored password content — it carries no drift-detection signal.
- Surfacing it as a Computed sibling encourages users to treat it as a real hash and write false-positive drift assertions.
- New Pro resources MUST NOT add a `*_sha256` Computed attribute alongside a `WriteOnly` plaintext.

`SetNestedAttribute` cannot contain `WriteOnly` children — the framework refuses to load the schema. If the wire shape carries a Set of nested objects with a plaintext secret (e.g. `account_maintenance.accounts[].password` on the classic policy), the surrounding attribute **MUST** be a `ListNestedAttribute`. Reorder wire entries by a stable natural key (username, name) when flattening so positional identity round-trips through unordered server responses — see `internal/resources/pro/policy/state_builders.go` `flattenPolicyAccountMaintenance` for the reference pattern.

### Server-minted secrets — `Computed + Sensitive` with a rotation trigger

`WriteOnly` is for **user-supplied** secrets. A secret that **Jamf Pro generates and returns once** (e.g. the Jamf-side public key minted by `jamfplatform_pro_pki_venafi`) is the mirror case: it is an *output*, not an input, so WriteOnly does not apply, and an **ephemeral resource is the wrong tool** — ephemerals re-run `Open()` every apply, and a "generate credentials" endpoint that mints a fresh secret on every call (with no idempotent read-back) would therefore rotate the secret on *every* apply. There is no stable value to anchor an ephemeral to.

**Pattern (reference: `internal/resources/pro/pki_venafi/`):**

- Model the secret as **`Computed + Sensitive`** (never `Optional`, never user-set) with `UseStateForUnknown` so it is sticky across plans. The value **is** written to the (Sensitive) state file — that is the unavoidable cost of making a server-minted secret consumable by downstream resources/outputs, and it is acceptable *only* because there is no read-back. Mirrors `aws_iam_access_key`, the VPP `serviceToken`, etc.
- Drive generation **opt-in** via an `Optional` string trigger (e.g. `credential_rotation`, the same idiom as `_wo_version`). Generate on Create only when the trigger is set; re-mint on Update only when its value **changes** (compare `plan` vs `state`; a `null`/removed trigger leaves the stored secret alone). Keep the compare logic in a small testable helper.
- **`ModifyPlan` must predict any server-side clearing of the secret.** If a sibling attribute revokes it server-side — e.g. an `enabled=false` that also discards the stored credential — then on `Read` you null the secret when the server reports it gone, **and** in `ModifyPlan` you must `resp.Plan.SetAttribute(<secret>, types.StringNull())` whenever that condition is planned (here: `enabled` planned `false`). Otherwise the sticky `UseStateForUnknown` value is planned non-null but applied null → `Provider produced inconsistent result after apply`. Force the secret `Unknown` in `ModifyPlan` when the rotation trigger changes so Update re-mints it (attribute plan modifiers run *before* resource `ModifyPlan`, so the override wins).
- Any precondition the generation endpoint enforces (e.g. rotation requiring the integration be enabled) gets a **cross-field plan-time validator** — assert only when the gating attribute is *known* (skip when unknown on create; the create-path guard + the server's own error cover that case).
- Data sources / list resources **MUST NOT** expose the secret (the API never returns it). `ImportStateVerifyIgnore` must list the secret **and** the rotation trigger.

### Server-derived computed fields & `Optional+Computed` attributes

Pro endpoints commonly return **server-derived** values for fields the user did not set — a "no category" sentinel like `categoryId="-1"` / `categoryName="NONE"`, a default `priority="AFTER"`, etc. These show up in three places, each with its own pitfall, and all three must line up or Terraform errors with `Provider produced inconsistent result after apply`.

**Clear sentinels are per-field, not universal — probe each one.** The `-1` "none" sentinel above is a *convention*, not a guarantee. Wire-probe the actual clear value for every nullable reference field, because it can differ **between fields on the same endpoint**. On classic `/patchsoftwaretitles`, `<site>` clears with `id=-1` (the norm) but `<category>` clears with `id=0` — sending `id=-1` to `<category>` is a silent no-op that retains the prior value (and an empty `<category></category>` returns HTTP 500); a never-set category echoes `-1` while a cleared one echoes `0`. When the wire sentinel diverges from the user-facing one, **honor the user-facing contract by translating at the wire layer** rather than leaking the quirk into the schema/docs/tests: map the user's sentinel to the wire clear value in the input builder, and collapse every server "none" echo back to the single user-facing sentinel in the state builder. Reference: `internal/resources/pro/patch_software_title/` (`buildPatchSoftwareTitleUpdateInput` maps category `≤0`→wire `0`; `categoryValues` collapses `-1`/`0`→`"-1"`).

**A classic clear sentinel can silently corrupt the shared v2 representation.** Classic and modern (v2) endpoints for the same entity write to one underlying record, but validate differently. On patch software titles the classic `<category><id>0</id>` clear leaves the v2 `categoryId` as the literal `"0"` — which `/v2/patch-software-title-configurations` then **rejects on every write** (`id field must be string of positive numeric value or -1`). So a hybrid resource that does classic CRUD plus a v2 side-call (e.g. extension-attribute accept) will see the v2 call 400 whenever the title has no category — even though the v2 body never mentions `categoryId`, because the server re-validates the whole stored object. Fix: **re-assert the user-facing value (always v2-valid) in the same v2 write**, repairing the divergence. Reference: `internal/resources/pro/patch_software_title/` `acceptPendingExtensionAttributes` re-sends `categoryId` alongside the EA merge-patch.

**1. Schema shape.** Any optional attribute the server fills in when omitted must be `Optional+Computed` so the framework knows the value can come from the server.

```go
"category_id": schema.StringAttribute{
    Optional: true,
    Computed: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
},
```

A purely **read-only** server-derived field whose value does **not** depend on another user-settable attribute (a standalone server identity — an `id`, `uuid`, created-date, etc.) is `Computed`-only with a `UseStateForUnknown` modifier — the user cannot set it, and its value carries across plans rather than going Unknown every refresh.

**But a read-only field *derived from a mutable sibling* (e.g. `category_name`, derived from `category_id`; `site_name` from `site_id`) MUST be *plain* `Computed` with NO `UseStateForUnknown`** — see [§Referencing a server-managed catalog by name](#referencing-a-server-managed-catalog-by-name). `UseStateForUnknown` pins the prior name into the plan even when the user changes `category_id`, but apply then reads back the *new* name → Terraform Core aborts with `Provider produced inconsistent result after apply` (`.category_name: was "Old", but now "New"`). Dropping the modifier lets the derived field go Unknown when the source changes, so the recomputed value is accepted. The cost — the field shows `(known after apply)` on any plan that touches the source — is correct, not noise. Wire-proven: `internal/resources/pro/script/` `TestAccResource_ProScript_CategorySwap` fails with the modifier present and passes once it is removed.

**The sentinel collapse applies to the derived *name*, not just the id — null it, don't echo it.** The id-side rule above ("collapse every server 'none' echo back to the single user-facing sentinel") has a name-side twin that is easy to miss: the classic GET **nondeterministically** either echoes `<name>NONE</name>` or omits the `<name>` element entirely for the unassigned sentinel (`<site>`/`<category>` with `id ≤ 0`). A state builder that trusts the echo (`StringPointerValueOrNull(s.Name)`) therefore flips the derived `*_name` between `"NONE"` and `null` across refreshes of the *same* object — surfacing as an `ImportStateVerify` diff (`- "site_name": "NONE"`) or `Provider produced inconsistent result after apply`, and only on the NONE/unassigned path so it reads as a flake. **For the sentinel branch (`id` nil or `≤ 0`), the derived name MUST be a deterministic constant (null), never the server echo.** Use `helpers.DerivedRefName(id, name)` — it returns null on the sentinel and the echoed name only for a real positive id — for every Computed `*_name` derived from a reference id (and `scope.FlattenSiteObject` for classic scope blocks, which bakes the same rule in). Reference: `internal/resources/pro/patch_software_title/` (`siteValues`/`categoryValues`).

**Server-*forced* fields (one attribute coerced to equal another) — and XML field-order sensitivity.** Some classic objects don't merely default a field, they overwrite it with another field's value. Jamf Pro forces `mobile_device_provisioning_profile.display_name` to equal `name`, and the collapse is **serialisation-order sensitive**: the server adopts whichever of the two elements appears *last* on the wire, and the generated SDK marshals struct fields **alphabetically** (`<display_name>` before `<name>`), so both land on `name`. Modelling `display_name` as `Optional` and sending the user's value yields `Provider produced inconsistent result after apply` (state shows the name, not what they set). Model such a field `Computed`-only and never send it. Wire-probe order-sensitivity directly — `POST` two *distinct* values and compare the `GET` (a probe that sends the same value for both, as is tempting, hides the collapse). Reference: `internal/resources/pro/mobile_device_provisioning_profile/`.

**1a. Nested-list elements: use `UseNonNullStateForUnknown`.** `Optional+Computed` scalars inside a `ListNestedAttribute` or `SetNestedAttribute` MUST use `stringplanmodifier.UseNonNullStateForUnknown()` (and bool/int siblings) rather than `UseStateForUnknown`. `UseStateForUnknown` copies the prior `StateValue` into the plan — including `Null` — and for an appended list element the prior state at the new index is `Null`. When the server then returns a value for that field on the new element, the framework consistency check trips with `Provider produced inconsistent result after apply`. If a `Sensitive` sibling (e.g. a `WriteOnly` password) lives on the same nested element, the error path is redacted up to the nearest non-sensitive ancestor, masking the real attribute and producing the misleading `.<parent>: inconsistent values for sensitive attribute`. `UseNonNullStateForUnknown` skips the copy when prior state is `Null`, leaving the plan `Unknown` so any post-apply value is accepted. Behavior is identical for the non-Null case (singletons, already-set values), so prefer this modifier uniformly within nested-collection element schemas. Reference: `internal/resources/pro/policy/resource.go` `optComputedString` / `optComputedBool` / `optComputedInt` helpers.

**2. Input builder must treat Unknown as nil.** When the user omits an Optional+Computed attribute, the plan value is **Unknown**, not Null. `types.String.ValueStringPointer()` returns a pointer to `""` for Unknown — Jamf Pro often rejects `categoryId: ""` with HTTP 500. Use the shared helper that nils both Null *and* Unknown:

```go
input := &pro.Script{
    Name:       plan.Name.ValueString(),
    CategoryID: helpers.OptionalStringPointer(plan.CategoryID),
    Priority:   helpers.OptionalStringPointer(plan.Priority),
    // ...
}
```

Apply `helpers.OptionalStringPointer` uniformly to every Optional and Optional+Computed string payload field. Do **not** call `types.String.ValueStringPointer()` directly on Optional+Computed attributes. Add bool/int variants only when a resource actually needs them — match the SDK pointer type at the helper signature so call sites don't need casts.

**3. Always refresh state via GET after Create AND Update.** Pro PUT responses are routinely **lossy** — `UpdateScriptV1` returns a `Script` without `categoryName` even though `GetScriptV1` does. If state is populated straight from the PUT response, server-derived fields go null while `UseStateForUnknown` carried the prior value into the plan — instant inconsistency error on Step 2 of any acceptance test that updates the resource.

```go
if _, err := r.client.UpdateScriptV1(updateCtx, plan.ID.ValueString(), buildScriptInput(plan)); err != nil {
    resp.Diagnostics.AddError("Error updating Jamf Pro script", err.Error())
    return
}

// PUT responses on Pro endpoints are lossy for server-derived fields
// (`categoryName` is omitted from PUT but present on GET). Refresh via GET
// so state is sourced from the canonical representation.
got, err := r.client.GetScriptV1(updateCtx, plan.ID.ValueString())
if err != nil {
    resp.Diagnostics.AddError("Error reading updated Jamf Pro script", err.Error())
    return
}
assignScriptResourceModel(&plan, got)
```

Create already follows this pattern (POST → HrefResponse → GET); Update must mirror it. Discard the PUT response's body — keep only the error.

**4. State builders must use `Reconcile*Pointer` for every Optional and Optional+Computed string field.** A bare `helpers.StringPointerValueOrNull(s.X)` overwrites Terraform's null/empty distinction every refresh; `helpers.ReconcileOptionalStringPointer(s.X, state.X)` preserves the user's prior value (including an explicit empty string) when the API returns nothing. Apply uniformly — do not mix `StringPointerValueOrNull` and `ReconcileOptionalStringPointer` across the same model. Only fully `Computed`-only fields (no `Optional`) should use `StringPointerValueOrNull` because there is no user value to reconcile.

**Reference implementation**: `internal/resources/pro/script/` (resource, crud, input_builders, state_builders).

**4a. Use `helpers.PreserveStringWhenWireEmpty` for user-authored strings the Classic API may echo as empty.** Some Jamf Pro Classic endpoints (observed on configuration-profile self-service blocks — `self_service_description`, `notification_subject`, `notification_message`, `authorization_password`) return the field as `<elem></elem>` on read even after a successful write of a non-empty value. `ReconcileOptionalStringPointer` treats an empty echo as "user did not set it" and collapses state to Null. When the plan held a non-empty user-authored string, the Null state then trips Terraform Core's `"Provider produced inconsistent result after apply"` check. **Watch out**: when the affected attribute lives in a block that *also* contains a `Sensitive` sibling, Terraform Core masks the error path to the parent block (e.g. `.self_service: inconsistent values for sensitive attribute` instead of `.self_service.self_service_description: ...`) — the wording falsely implicates the Sensitive attribute and sends debugging in the wrong direction. Default rule: any user-authored string field under a block that carries a Sensitive sibling should use `PreserveStringWhenWireEmpty` rather than `ReconcileOptionalStringPointer`. Sentinel-test for this pattern with a unit test exercising a non-empty configured value plus an empty-string wire echo.

### `SingleNestedAttribute` blocks: Optional-only when the model uses typed-pointer

Nested blocks modelled as `*StructModel` (typed pointer to a struct with `tfsdk:` tags on every field) cannot be `Optional+Computed`. The Plugin Framework decodes an absent-but-Computed block as **Unknown**, and `*StructModel` has no representation for Unknown — apply fails with:

> `Received unknown value, however the target type cannot handle unknown values. Use the corresponding`types`package type or a custom type that handles unknown values.`

Two ways out:

1. **Keep the block `Optional`-only.** Inner fields can still be `Optional+Computed` so the server may populate defaults the user omitted. This is the right default — typed-pointer models are easier to read and write than `types.Object`-shaped ones. Document for users: supply the block (even empty: `<type>_block = {}`) to take management of the per-type configuration.

   **The flatten MUST gate block (re)population on the model being WRITTEN to state — the mutated target — never a separate prior-state reference.** The server returns a populated block in every `GET`; populating it unconditionally violates the framework's consistency contract whenever the plan said null. The target pointer is the planned value Terraform Core compares the apply result against:
   - **Create / Read:** the state-builder mutates a single model; gating on that model is correct.
   - **Update:** the target is the **new plan**. If the state-builder takes a separate prior-`state` param and gates on `state.X != nil`, then *removing* a previously-managed block crashes the apply: the plan pointer is nil but the prior state still holds the populated block, so the flatten repopulates from the wire and trips `Provider produced inconsistent result after apply: …: was null, but now cty.ObjectVal(…)`. (When the block contains a `Sensitive`/`WriteOnly` field the same crash surfaces as the less obvious `inconsistent values for sensitive attribute`.) Pass the target model itself, or capture the target's pointer presence (`manageX := plan.X != nil`) before reassigning, and gate on that. Unit tests, lint, and `make generate` never exercise this — it only fails under `make testacc` (or a real `apply` that removes a block), so cover the remove-on-update transition in acceptance.

   Reference for the correct shape: `internal/resources/pro/policy/` passes a single model to `assignPolicyResourceModel` and gates on it (which *is* the new plan on Update). The `*_prestage_enrollment` resources capture `manageX := plan.X != nil` up front.
2. **Switch the model field to `types.Object`** with an `attrTypes` map. Only worth it when the resource genuinely needs the framework to represent the block as Unknown — most don't.

Reference: `internal/resources/pro/directory_binding/` — five per-type nested blocks, all Optional-only, inner fields Optional+Computed.

**The same trap applies to `Computed` nested *collections*** (`ListNestedAttribute` / `SetNestedAttribute`, or a nested object), not just `SingleNestedAttribute`. A **Computed** collection is **Unknown at plan time**, and a Go typed slice (`[]FooModel`) / `*StructModel` cannot carry Unknown — the identical "target type cannot handle unknown values" error fires, but only under acceptance `apply` (unit tests, lint, vet, and `make generate` never exercise plan/apply, so it is invisible until the first `make testacc`). Model Computed collections as `types.List` / `types.Set` with an `attr.Type` ObjectType, and build them with `types.ListValueFrom(ctx, objType, slice)` returning a **known** (possibly empty) value so the attribute resolves from Unknown at apply. The split is by **`Computed`**, not by nesting depth: user-authored `Optional`/`Required` collections are never Unknown, so Go typed slices are fine for them; and Computed *scalars* (`String`/`Bool`/`Int64`) inside a typed-slice element are fine — only nested collections/objects need the `types.X` treatment. Reference: `internal/resources/pro/licensed_software/` — Optional `software_definitions`/`licenses` are `[]Model`, while Computed `computers` and `licenses[].attachments` are `types.List`.

### Asymmetric server normalisation on `type`-style discriminator fields

Some classic endpoints accept a **legacy product name** on write but normalise to the **modern product name** on read. Example: PowerBroker directory bindings — the `/directorybindings` create path rejects `type="PowerBroker Identity Services"` with HTTP 409 "Problem with directory binding type" and only accepts `type="Likewise"` (the pre-acquisition name); the read path always returns `type="PowerBroker Identity Services"`. Pass the alias through to TF state and users get gratuitous drift; pass the modern name through to the server and Create silently fails.

Pattern: translate one-way at the input boundary so the wire-canonical name is what users see in state, and the legacy alias is an implementation detail. Centralise the mapping in `helpers.go`:

```go
const typePowerBrokerCreateAlias = "Likewise"

func mapType(tfType string) string {
    if tfType == typePowerBroker {
        return typePowerBrokerCreateAlias
    }
    return tfType
}
```

Inside `input_builders.go`, route the `type` field through the mapper before emitting the SDK payload. Cover the mapping with a unit test (input X → wire Y) so future maintainers don't quietly delete it.

Reference: `internal/resources/pro/directory_binding/helpers.go` (`mapType` + `typePowerBrokerCreateAlias`).

#### Plan-modifier rewrites are NOT a valid option for `Required` attributes

The "translate at the input boundary" pattern above only works because the user-facing TF value and the wire value are **identical** in directory_binding — the alias is swapped inside the input builder, not in the schema. If you instead try to canonicalise via a `planmodifier.String` that rewrites `req.ConfigValue` (e.g. lowercasing `Individual And Institutional` → `Individual and Institutional` at plan time), Terraform's plugin framework rejects the apply with:

```
Error: Provider produced invalid plan

Provider planned an invalid value for <attribute>:
planned value cty.StringVal("…") does not match config value cty.StringVal("…").

This is a bug in the provider, which should be reported in the provider's own issue tracker.
```

The framework enforces `plan == config` for any attribute that is **not** `Computed`. Required attributes can't be rewritten by plan modifiers. The two valid options are:

1. **Strict wire-form acceptance** — schema validator (`stringvalidator.OneOf`) accepts only the canonical wire spellings; user must supply them verbatim. Drift surfaces at plan time as a validator error, not at apply time as a framework violation. **Prefer this** for simple discriminator fields.
2. **Optional + Computed with plan-modifier rewrite** — works mechanically, but turns the attribute into a "computed override" and complicates required-input semantics (the user can omit it). Only worth it when the canonicalisation is more than a case-fold (e.g. an alias swap that genuinely round-trips).

The mistake to avoid: pairing `Required: true` with `OneOfCaseInsensitive` + a canonicalising plan modifier. The validator passes, but the framework will reject the plan at apply time. Caught during `disk_encryption_configuration` build, 2026-05-23.

References: `internal/resources/pro/directory_binding/resource.go` (Required + OneOf, no plan modifier — preferred pattern); `internal/resources/pro/disk_encryption_configuration/resource.go` (`key_type` accepts only `Individual and Institutional` with lowercase `and`).

### Working around server-broken classic endpoints

Some Jamf Pro classic endpoints are genuinely broken — the `/directorybindings/name/{name}` endpoint returns HTTP 500 for every name lookup as of 2026-05-23, even when the binding exists and is reachable by ID. List + ID paths work; only name lookup is dead.

When the SDK call backed by a broken endpoint is on a non-critical path (data source name resolver, optional fallback), route around it inside the resource package rather than waiting on the upstream fix:

- Implement a private helper in the resource package (e.g. `lookupByName` in `data_source.go`) that does **List + client-side name match + GetByID**.
- Doc-comment the helper with the bug summary, the date observed, and the upstream fix that would let the helper be deleted.
- Open an SDK issue so every consumer benefits once the SDK adopts the fallback (or once the server is fixed).

Reference: `internal/resources/pro/directory_binding/data_source.go` (`lookupByName`).

### SDK type-decode bugs — fix by generator override, not provider hand-decode

The previous section is about a broken *endpoint* (the server misbehaves). This one is about a broken *SDK type* — the endpoint works, but the generated Go struct can't decode the server's response. The two are different and have different fixes.

**The recurring shape.** Jamf's OpenAPI spec declares a field `type:string, format:date-time` (often with a `Z`-suffixed example), so the SDK generator maps it to `*time.Time`. But the live server emits a **timezone-less** ISO-8601 value (e.g. `"2028-05-15T13:41:46"`, `"2026-05-28T13:59:26.491"`). Go's RFC3339 decoder rejects it, and because `json.Unmarshal` aborts the **whole body** on the first field error, the SDK call returns `nil, err` — the entire response is unrecoverable, not partially populated. The spec is self-inconsistent (the server violates its own declared `date-time` format); the SDK faithfully generated the wrong type. The same trap exists for any spec/server type mismatch (int vs string, object vs map, etc.), not just dates.

**The rule: fix it in the SDK via a generator override. Never hand-decode in the provider.** Adding a bespoke `UnmarshalJSON`, a regex pre-parse, or an error-tolerant fallback inside a resource package is a code-review reject — it hides the defect from every other SDK consumer, rots silently, and duplicates per resource. The SDK is the single correct place to fix a wire-shape bug.

**How to fix (SDK side):**

1. Add one entry to the SDK's `tools/generate/config.json` → the `pro` (or relevant package) block's `fieldTypeOverrides` map. Key is `<snake_case_schema_name>.<jsonFieldName>`; value is the correct Go type. For timezone-less dates that's `"*string"` — the raw wire value round-trips and callers parse it themselves if needed. Example: `"cloud_ldap_keystore.expirationDate": "*string"`.
2. Regenerate so `types.go` picks up the override (the file is `DO NOT EDIT` — never patch it by hand; the override is what survives regen).
3. Add/extend an SDK test with a **realistic** body. The bug usually hides because the existing SDK test returns an empty object (`map[string]any{}`) that never reaches the parser — close that coverage hole.

**Precedent in the same spec:** `SsoKeystoreDetails.expiration` is declared plain `type:string` (no `date-time`) and decodes fine — that's the target shape for timezone-less Jamf datetimes.

**Provider-side consequence:** model the field as a Computed echo string and read the SDK's `*string` straight through. No workaround to remove later.

**Workflow:** write a fix-prompt doc at the provider repo root (gitignored), mirroring `SDK_PRESTAGE_SCOPE_ASSIGNMENT_DATE_FIX_PROMPT.md` / `SDK_CLOUD_LDAP_KEYSTORE_DATE_FIX_PROMPT.md`: state the bug, the affected SDK functions, the wire evidence, the spec quote, the exact override entry, and the test gap. Hand it to a session in the SDK repo; land the SDK PR; bump the provider `go.mod`. If a resource is blocked on the fix, record it in that resource's spike/memory and pin the dependency rather than hand-decoding to unblock.

### Field-order-sensitive classic write payloads (rare)

The SDK generator emits struct fields **alphabetically**, so a marshaled classic `PUT`/`POST` body lists child elements in alphabetical order, not the order the OpenAPI spec / admin UI use. **Nearly every classic endpoint tolerates this** — the large majority of shipped classic resources marshal alphabetically and pass acceptance. Do **not** treat field order as a systemic risk or reorder structs pre-emptively.

A **rare** endpoint is order-sensitive on write and rejects the alphabetical body. Known case: ProClassic `/vppinvitations` `<general>` — `auto_register_managed_users` (alphabetically first) marshaled before the core fields returns **HTTP 500**; the same body in the GET wire order (`name`, `vpp_account`, `distribution_method`, …, `auto_register_managed_users` last) returns 201.

**Debugging cue:** a classic create/update that 500s with no obvious payload error (all fields present, names/types correct) — compare the marshaled child order against a live GET. If they differ and reordering to wire order fixes it, it's this.

**Fix in the SDK generator, per-schema — never globally.** A blanket "preserve spec order" switch would reorder every existing struct and risk regressing the resources that work today. Add a targeted per-schema field-order override (mirror the `fieldTypeOverrides` fix-prompt workflow above) so only the order-sensitive schema is reordered to wire order. Reference: `jamfplatform_pro_vpp_invitation` (SDK `a37279c`).

### Form-decoded classic input fields (encode on write)

A few classic text fields are **form-URL-decoded by the server on write** (`%XX` → byte, `+` → space). A literal `%` not followed by two hex digits is then a malformed escape and the write **500s**. Known case: `/vppinvitations` `<message>` (the invitation email body, which legitimately contains the `%@` registration-URL placeholder).

When a classic string field form-decodes input, **`url.QueryEscape` the value on write** in the input builder; the server decodes it back, so it stores and GETs the verbatim original (placeholders, literal `%`, embedded newlines all round-trip). **Read needs no transform** — the GET already returns the decoded value, which matches config. This also sidesteps newline-normalisation diffs for free. Reference: `encodedMessagePointer` in `internal/resources/pro/vpp_invitation/input_builders.go`.

### Cross-field validation

**Required pattern:** every cross-field rule MUST be expressed as an **attribute-level validator** (`Validators: []validator.Bool{...}` on the schema attribute itself), not as a resource-level `resource.ResourceWithConfigValidators`. Errors attach to the offending attribute, the rule is co-located with the schema, the diagnostic is field-named, and the convention matches every resource in the provider.

**Decision matrix — which validator to reach for:**

| Rule shape | Use |
|---|---|
| "A and B are paired — supplying either requires the other" (any value triggers) | Off-the-shelf `boolvalidator.AlsoRequires` / `stringvalidator.AlsoRequires` / `int64validator.AlsoRequires` |
| "A and B are mutually exclusive" (any value triggers) | Off-the-shelf `boolvalidator.ConflictsWith` / `stringvalidator.ConflictsWith` / `int64validator.ConflictsWith` |
| "Enum membership" | Off-the-shelf `stringvalidator.OneOf(values...)` |
| "Required only when this bool is `true`" or "required only when this string equals `\"X\"`" — **value-specific** | Custom `validator.Bool` / `validator.String` in the package's `validators.go` |
| "Exactly one of X / Y / Z" spanning several siblings | Framework `resourcevalidator.ExactlyOneOf` / `AtLeastOneOf` / `Conflicting` |

**Rule of thumb:** the off-the-shelf `AlsoRequires` / `ConflictsWith` fire whenever the validated attribute is *known* — true OR false, `"foo"` OR `""`. They do NOT inspect the value. **If the rule only applies for a specific value, the off-the-shelf semantics are wrong and a custom validator is the right tool — but only then.** Writing a custom validator when an off-the-shelf one fits is a code-review reject.

**Path syntax:** `path.MatchRoot("other_attr")` for a root-level sibling; `path.MatchRelative().AtParent().AtName("other_attr")` for a sibling under the same nested object.

**Off-the-shelf references:**

- `internal/resources/blueprints/blueprint/components/math_settings.go` — paired toggles with `boolvalidator.AlsoRequires`.
- `internal/resources/blueprints/blueprint/components/software_update.go` — mix of `AlsoRequires`, `ConflictsWith`, `RegexMatches`, `Between`.
- `internal/resources/pro/network_segment/resource.go` — `override_buildings` / `override_departments` pair with their companion string via `AlsoRequires`.
- `internal/actions/device/erase.go` — toggle + companion via `AlsoRequires`.

**Custom validator references (value-specific only):**

- `internal/common/scope/validators.go` — `AllFlagConflictsWith` is a `validator.Bool` that fires only when the bool is `true`; off-the-shelf `ConflictsWith` would incorrectly fire when the bool is `false` too.
- `internal/resources/pro/sso_settings/validators.go` — value-discriminated validators for `configuration_type ∈ {SAML, OIDC, OIDC_WITH_SAML}`, `metadata_source ∈ {URL, FILE}`, `setup_type = "UPLOADED"`, etc., each requiring different companion sets.
- `internal/resources/pro/computer_inventory_collection_settings/validators.go` — `requiresAccountCollection` is a `validator.Bool` on two child sub-options that fires only when the sub-option is `true` and its parent toggle (`collect_local_user_accounts`) is `false` — the combination the server coerces (silently forcing the sub-option off). **A plan modifier is the wrong tool here:** the framework rejects a `ModifyPlan` that overrides an explicitly-configured value ("planned value … does not match config value"), and a sub-option left unset already resolves to the server's `false` harmlessly — so config-time validation, attached to the offending sub-option, is the only correct guard.

**Custom validator authoring rules:**

- Implement `Validate{Bool,String,Int64,...}` on a struct in `validators.go`.
- Skip when `req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown()` — apply-time references must not false-positive.
- Skip when the value does not match the discriminator (e.g. `req.ConfigValue.ValueBool() == false` for a "true-only" rule).
- Read companions via `req.Config.GetAttribute(ctx, path.Root("…") / path.MatchRelative()..., &target)`.
- Attach errors to the **companion's** path (not the discriminator's), via `resp.Diagnostics.AddAttributeError(companionPath, summary, detail)` — that's where the user needs to look.

##### Config-time validators MUST defer on unknown values — for EVERY attribute they read, not just the discriminator

Config validation (`ValidateConfig` / any `ConfigValidator` / schema `Validate{String,…}`) runs with **unknown** values for anything sourced from a variable, `for_each`, `count`, or another resource. `Unknown` means "not resolvable yet", **not** "missing". A validator that errors on unknown makes the resource unusable from anything but hard-coded literals — it breaks `terraform validate`/`plan` for every reusable module. (Caught provider-wide 2026-05; cf. deploymenttheory/terraform-provider-jamfpro #1111.)

- **Skip on unknown before treating absence as an error.** A "required-when" check must `return` (defer) when the *dependent* attribute is unknown, and error only when it is genuinely `null`. The discriminator is not enough — guard the companion too.
- **Never let a Go decode hide unknown.** `req.Config.Get(ctx, &model)` collapses `unknown` to a zero value: a nested block becomes a `nil` pointer, a list/set becomes `len() == 0`, a custom "is-set" helper returns false. So `if block == nil`, `if len(slice) == 0`, `if !isSet(x)` all **false-error on unknown**. Read the dependent attribute as its **typed value** (`types.Object` / `types.List` / `types.String`) via `GetAttribute` and branch on `IsUnknown()` / `IsNull()` explicitly — do not infer presence from the decoded Go value.
- **Forbidden-when checks are safe** (they fire on *presence*, so unknown-treated-as-absent just defers) — but required-when checks are the trap.
- **Regression-test with unknowns, not just literals.** Acceptance tests use literal HCL (always known) and therefore CANNOT catch this. Add a unit test that feeds `tftypes.UnknownValue` to the dependent attribute and asserts no diagnostic. References: `internal/resources/pro/user_initiated_enrollment_settings/validators_test.go` (`TestCertInvariant_SilentWhenCertUnknown`), `…/inventory/disk_encryption_configuration/validators_test.go`, `…/users/user_group/validators_test.go`. `inventory/ibeacon/validators.go` is the reference body (validates only when both values are known).

Avoid `resource.ResourceWithConfigValidators` even for multi-attribute rules unless the framework's `resourcevalidator` package genuinely cannot express them. Bespoke whole-config validators are a last resort.

### Configuration profile payload diff suppression (mask-and-compare)

Jamf classic configuration-profile endpoints (`osxconfigurationprofiles`, `mobiledeviceconfigurationprofiles`) re-serialise the user-supplied `.mobileconfig` plist server-side: whitespace stripping, version normalisation (`1.0` → `1`), server-stamped defaults, top-level UUID/Identifier rewrites, per-payload display-name defaults keyed on `PayloadType`. Trying to *predict* every mutation produces a brittle map of `PayloadType` → server-default lookups that drifts the moment Jamf changes a default. The provider uses **mask-and-compare** instead.

**Strategy.** The plan modifier parses both sides (user input + server-canonical state) and runs the same `payloadhelpers.MaskPayload` mask across both before comparing. The mask is symmetric and content-blind — the provider never needs to know that `com.apple.notificationsettings` defaults to `"Notifications Payload"`; both sides drop the key, both sides agree.

Mask drops (or empty-normalises) from **both** sides:

- **Top-level always-clobbered**: `PayloadDisplayName` (set from `general.name`), `PayloadIdentifier`, `PayloadUUID` (Jamf assigns lowercase UUIDs).
- **Top-level server-added defaults**: `PayloadEnabled`, `PayloadDescription`, `PayloadRemovalDisallowed`.
- **`PayloadContent[i]` server-augmented**: `PayloadDisplayName`, `PayloadUUID`, `PayloadIdentifier`, `PayloadOrganization`, `PayloadEnabled`, `PayloadDescription`, `AllowUserOverrides`, `VendorConfig`.
- **String values**: recursive leading/trailing whitespace trim (Jamf strips whitespace inside e.g. `Rules[].Comment`).
- **Empty-string normalisation**: `""` on either side compares equal to "absent".
- **Line endings inside string values**: `\r\n` and lone `\r` normalise to `\n` (`normalizeLineEndings`). See below.

**Line-break tolerance (paired with the SDK's CR-reference handling).** CR is the only whitespace character Jamf Pro keeps inside a string value — literal LF and TAB are **deleted outright** (words merge) in the payload types stored verbatim and in every slot outside `PayloadContent` — and it can only be authored as a `&#13;` reference, since XML 1.0 §2.11 rewrites a literal CR to LF in transit. Hence Jamf's own admin UI emits `&#13;`, and the SDK transmits those references bare (`escapeAmpPreservingCRRefs`) rather than decoding them. It always reads back as LF though: MCX and mobile fragments normalise on store, and a verbatim-stored CR returns as a raw CR byte our own parse normalises. **Both halves are required** — this normalisation must ship with (or before) any SDK that preserves CR references, or every `&#13;` fails the fidelity gate.

Cost: CR-vs-LF drift fidelity everywhere. Accepted over a per-`PayloadType` model of the server's line-break handling, which is empirical and drifts each release. U+2028/U+2029/U+0085 are deliberately **not** normalised — they round-trip byte-exact, render as line breaks on-device, and keep full drift detection.

**`PayloadContent` entry order is not preserved on store — canonicalise it, do not compare it.** Jamf Pro **stably partitions** the top-level `PayloadContent` array into the entries it stores *verbatim* followed by the entries it *re-renders* (the same two categories as §"Payload storage categories" below), keeping relative order within each block. Wire-probed 2026-08-11 against Jamf Pro 11.30.x across 18 profile shapes, no exceptions:

| Authored order | Stored order |
|---|---|
| `[MCX, certificate]` | `[certificate, MCX]` |
| `[certificate, MCX]` | unchanged — already canonical |
| `[MCX, certificate, MCX]` | `[certificate, MCX, MCX]` |
| `[notificationsettings, MCX]` | unchanged — both re-rendered |
| `[loginwindow, certificate]` | unchanged — both verbatim |
| `[MCX, loginwindow, certificate]` | `[loginwindow, certificate, MCX]` |
| `[MCX-a, certificate, MCX-b, certificate-b, MCX-c]` | `[certificate, certificate-b, MCX-a, MCX-b, MCX-c]` |

A positional array compare therefore pairs unrelated entries and reports cascading false drift on every key of both — which reached a user as a bogus `"Jamf Pro cannot store this payload faithfully"` error plus a create-time rollback for a payload the server had stored perfectly. `maskPayloadContent` closes this by **stable-sorting the array by `PayloadType`** (`canonicalisePayloadContentOrder`), so both sides reach the comparator in the same order and every comparator built on `MaskPayload` — the lenient `LenientEqualPlist` and the strict `structuralEqual` behind `ThreeWayCompare` — inherits the fix.

Two properties make this a canonicalisation rather than a tolerance:

- **Entry order carries no meaning.** Apple treats each `PayloadContent` entry as an independent payload, so an order-insensitive comparison is the correct one.
- **The sort is stable and keyed only on `PayloadType`**, so same-type siblings keep their authored order relative to each other and an edit to one of them still surfaces as drift. That granularity is exactly what the wire law needs: same `PayloadType` implies the same partition, so Jamf Pro *cannot* reorder same-type siblings.

Do not reach for `PayloadIdentifier` as a pairing key — it is masked (server-assignable) and already gone by compare time.

The same reorder breaks the **read-back diagnostic**, which quotes indexed paths (`PayloadContent[1].Foo`) and so needs both sides to agree on which entry index 1 is. `PayloadFidelityErrorDetail` permutes the stored array back into the *authored* order (`alignPayloadContentOrder`) rather than sorting both, so the indices it prints match the payload the operator wrote.

**Web clip icons are compared after normalising, not byte-for-byte.** A `com.apple.webClip.managed` entry's `Icon` is the one image in Apple's payload schema (Release-v26.4 declares thirteen `data`-typed keys; the other twelve are certificates, a font, two VPN secrets and an SSO group identifier), and Jamf Pro **always** re-renders it — wire-probed 2026-09-09 against Jamf Pro 11.3x on both `mobiledeviceconfigurationprofiles` and `osxconfigurationprofiles`:

| Authored | Stored |
|---|---|
| 8x8 PNG | 180x180 PNG (upscaled) |
| 120x120 PNG | 180x180 PNG |
| 180x180 PNG | same dimensions, different bytes |
| 200x100 PNG | 180x90 PNG — longest side 180, aspect preserved |
| JPEG or GIF | PNG |
| anything undecodable, including an empty blob | a fixed 180x180 placeholder PNG |

The stored form keeps the source colour model and carries only `IHDR`/`IDAT`/`IEND` — every ancillary chunk is stripped. The re-render is **deterministic**, which is why an icon read back out of Jamf Pro and re-submitted does round-trip byte-exact; it is also why the failure was latent until something triggered a write, and then failed *every* write to that profile (issue #418). Reproducing the re-render client-side would mean matching another encoder's zlib settings and filter choices, so `LenientEqualPlist` compares that one leaf through `iconsEquivalent`: both sides are decoded, box-filtered to a 16x16 grid, and compared on mean per-channel difference against a tolerance of 8 (measured on the same probe run: Jamf Pro's own re-render of an icon scores 0.65–2.38, two different icons score 45.1, so the threshold sits an order of magnitude above the noise and a factor of five below a real change — `TestIconsEquivalent_ToleranceHeadroom` pins both populations). An icon either side cannot decode never compares equal, so a placeholder substitution surfaces rather than passing.

Three rules go with it. **Do not widen this to data blobs in general** — a certificate, a font and a VPN shared secret were probed in the same run and all three come back byte-identical, so a re-rendered certificate is real corruption and must keep failing. And **`data_files` is masked, for a reason one-sidedness does not cover.** It is Jamf Pro's integer handle for a stored icon, it exists only on the stored side, and the intersection compare would ignore it there. The strict comparator behind `ThreeWayCompare` and `PayloadsStructurallyEqual` would not: it demands matching keysets and values, and it compares *two server payloads* (the one stashed at apply against the one read back now) to tell an admin-side edit from a steady refresh. Two consecutive reads of one unchanged profile return **different** `data_files` values (wire-probed 2026-09-09), so before the mask every refresh of an icon-bearing profile reported drift nobody caused and the plan never settled. Mobile-device profiles emit the key; macOS profiles do not.

And **the tolerance applies only where one side of the comparison is a server response — never to two user-authored payloads.** The strict comparator is the delicate one here, because unlike `LenientEqualPlist` it straddles authoring layers: `structuralEqual` backs *both* arms of `ThreeWayCompare`, and arm one compares `planInput` against `lastInput`, two pieces of user HCL neither of which has ever been near Jamf Pro's renderer. So `payloadsStrictlyDiffer` takes an explicit `tolerateIconRerender` flag, threaded down through `structuralEqual`'s map and slice branches: `false` for the user arm, `true` for the drift arm (`lastCanonical` vs `serverNow`) and for `PayloadsStructurallyEqual`, whose operands are both server responses. Tolerating the user arm is silent data loss, not just lost fidelity: two authored icons differing by a small badge normalise inside the tolerance, so arm one reports no change, arm two agrees, `ThreeWayCompare` returns `DecisionNoOp`, and the plan modifier rewrites `plan.payloads` back to state — the edit can never be applied and no diagnostic is raised. The tolerance is still load-bearing in the drift arm even though its left side is an authored-shaped value, because `threeWayServerNow` falls back to `state.payloads` when private state has no server snapshot, and lenient self-healing leaves that in the authored form. `TestThreeWayCompare_AuthoredIconEditWithinToleranceApplies` pins the user arm; `TestThreeWayCompare_ServerIconRerenderIsNoOp` and `TestPayloadsStructurallyEqual_ServerIconRerender` pin the other two call sites.

If `inp_masked == srv_masked` the modifier suppresses the diff by setting `plan.payloads = state.payloads`. Otherwise the plan keeps the raw user input and Terraform plans the change.

**Trade-off accepted**: if the user authors a meaningful change to one of the masked keys (e.g. a hand-tuned `PayloadOrganization`), the provider will not detect drift — the server overwrites that value on the next write anyway, so a permanent diff would be the alternative. Document this in the `payloads` attribute `MarkdownDescription`.

**Update-path identifier injection.** On Update, the input builder parses `plan.payloads`, overwrites the top-level `PayloadUUID` and `PayloadIdentifier` with the values from `state.payloads`, and reserialises before PUT. Without this step every Update mints fresh UUIDs server-side and devices treat the update as a fresh profile installation ("ghost profile"). Top-level only is sufficient — nested `PayloadContent[i].PayloadUUID` survives PUT cycles without intervention once stored. Same mechanism as jamf-cli `profileconvert.InjectIdentifiers`.

**Asymmetric envelope `<level>` normalisation.** Classic accepts `<level>Computer</level>` on write but echoes `<level>System</level>` on read; `<level>Computer Level</level>` is silently rejected and defaults to `User`. Use the input-boundary translation pattern from §"Asymmetric server normalisation on type-style discriminator fields" — translate user-facing `Computer Level` / `User Level` to wire `Computer` / `User` on write, map wire `System` / `User` back on read.

**Read-back verification diagnostics.** When the mask compare fails after a write, the resource rolls the create back (update cannot) and fails with `payloadhelpers.PayloadFidelityErrorDetail`, which diffs supplied-vs-stored string leaves, skips masked keys, and classifies each divergence: line feeds/tabs deleted → recommend `&#13;` / `&#8232;` / an MCX payload; extra `&`/`<` entity layer → PI-827, unfixable client-side; non-BMP characters replaced or the enclosing dict dropped → Jamf-side limitation; value absent; unrecognised. Each finding names the plist path and quotes both forms `%q`-escaped around the first difference so invisible characters are visible. **Non-string leaves are diffed too** (`diffNonStringLeaves`), through the same intersection semantics, numeric leniency and icon equivalence the equality check uses, so the differ can never name a leaf the comparison was happy with — and, more to the point, can never come up empty on a tree that compared unequal. That was the second half of issue #418: the differ walked authored *string* leaves only, so a diverging data blob yielded zero findings while the trees were still unequal, and the operator got the unattributed fallback listing line feeds and ampersands that had nothing to do with it. A blob, a list or a dictionary is reported as a summary (`describeLeaf`) rather than dumped, and such a finding is marked `described` so the excerpting is skipped — excerpting a summary around its first differing character quotes half a sentence. Two constraints when editing it: bullet text must arrive **pre-wrapped** (Terraform re-wraps flush-left paragraphs but leaves indented lines as-is), and never add a static blanket explanation — a diagnostic that names characters the payload does not contain is worse than none. Acceptance `ExpectError` patterns for it must be **single tokens** (see §ExpectError line-wrap).

Reference implementation: the shared mask / compare / identifier-injection logic lives in `internal/common/payloadhelpers/` (`MaskPayload`, `PayloadsSemanticallyEqual`, `ThreeWayCompare`, `InjectTopLevelIdentifiers`); the per-resource glue is in `internal/resources/pro/macos_configuration_profile/` (`plan_modifiers.go` plan-time integration, `input_builders.go` / `crud.go` Update-path injection, `resource.go` MarkdownDescription disclosing the masked key set to users).

### Payload storage categories and the import fidelity gate

The read-back verification above is a **post-write** check, and it is only half a guard. Create can roll back when it fires; Update has nothing safe to roll back *to*, because the Classic API refuses to accept the original value back once the profile holds the mangled form (wire-verified: re-submitting a server's own unmodified `<payloads>` returns `409 Unable to update the database`). Repair is admin-UI-only.

That leaves **import** as the one way an unstorable profile can come under management: the provider will not create one, but it will happily adopt one authored in the admin UI. Such a profile imports clean and plans as a no-op, then the first apply that touches it — a rename, a scope tweak, anything — rewrites the payload irreversibly. `payloadhelpers.PayloadImportRisk` therefore refuses the import up front.

**Storage category is per `(platform, PayloadType)`, and the two platforms genuinely disagree.** Jamf Pro either re-renders a payload fragment (parses and re-serialises it — `&`, `<`, LF and TAB all survive, and unknown keys are preserved, so this is not schema filtering) or stores it verbatim (keeps the extra entity layer the Classic API requires on submission, and deletes LF and TAB; `>`, CR, U+2028/U+2029/U+0085 survive). Wire-probed 2026-08-06 against Jamf Pro 11.30.x:

| PayloadType | macOS | Mobile device |
|---|---|---|
| `com.apple.ManagedClient.preferences` | re-render | **verbatim** |
| `com.apple.applicationaccess` | **verbatim** | re-render |
| `com.apple.notificationsettings`, `com.apple.mobiledevice.passwordpolicy`, `com.apple.webcontent-filter` | re-render | re-render |
| `com.apple.systempolicy.control`, `com.apple.security.firewall`, `com.apple.SubmitDiagInfo`, `com.apple.servicemanagement`, `com.apple.extensiblesso` | re-render | not probed |
| `com.apple.shareddeviceconfiguration` | not probed | re-render |
| `com.apple.loginwindow`, `com.apple.MCX`, `com.apple.TCC.configuration-profile-policy`, `com.apple.vpn.managed`, `com.apple.wifi.managed`, `com.apple.dock`, `com.apple.screensaver`, `com.apple.SetupAssistant.managed`, `com.apple.systempreferences`, `com.apple.finder`, `com.apple.appstore`, `com.apple.dashboard`, `com.apple.system-extension-policy`, `com.apple.syspolicy.kernel-extension-policy` | verbatim | verbatim (where probed) |
| `com.apple.webClip.managed` | n/a | verbatim |

The first two rows are why a single global table is not an option — it would necessarily get one platform wrong. `testdata/reserved_character_corpus.mobileconfig` exists in both resources' fixtures with identical content and opposite expected verdicts; that pair is the regression guard.

**Rules for the table** (`faithfulPayloadTypes` in `importgate.go`):

- Only **re-render** types are listed. Everything absent — unprobed types, and every string slot outside the `PayloadContent` array such as a top-level `ConsentText` dictionary — is treated as verbatim.
- **Deny-by-default is deliberate and asymmetric.** A wrong "verbatim" guess costs one refused import, which is loud, immediate and recoverable. A wrong "re-render" guess lets a profile through to be corrupted irreversibly by its first update. Never guess in the second direction.
- **Add an entry only from a wire probe, never by inference.** The harness is `storage_category_probe_test.go` (build tag `payload_probe`). Inference is not idle caution: of the payload types with no prior classification, roughly a quarter turned out re-render (`servicemanagement`, `extensiblesso`, `webcontent-filter`), so deny-by-default alone would have refused them. `webcontent-filter` only yielded to a probe built from a real payload lifted off a tenant — every synthetic shape was schema-rejected, which is what `TestProbeStorageCategoriesFromRealPayload` is for.
- Note that a re-render *ingest* path may still add or drop keys (a Web Clip gains `Icon` / `data_files` and loses unrecognised keys). That is independent of the escaping question this table answers; do not conflate the two.

**Implementation constraints.**

- The gate builds a *predicted* stored form and diffs it through `diffPayloadTrees` — the same comparator, mask and classifier the post-write checks use. Do not write an independent predicate: sharing the comparator is what makes the gate and the post-write check unable to disagree, and reduces correctness to one testable function.
- **Gate on first-time hydration** — see [§First-time hydration detection in Read](#first-time-hydration-detection-in-read) for the general rule. Both profile resources use the `state.General == nil` signal (`general` is schema-`Required`). An earlier revision got this wrong and every profile imported cleanly while every unit test passed.
- **Never gate ordinary refresh.** A profile already under management, or corrupted out-of-band, must keep refreshing so drift stays visible and the resource stays removable — otherwise the guard is worse than the trap.
- **No opt-out, by design.** A profile Jamf Pro cannot store faithfully cannot be managed safely, and a provider-global override would silently reinstate the corruption. A misclassification is fixed in the table for everyone; the diagnostic asks the operator to report the `PayloadType`.
- List resources **drop** a refused item from the stream and name it in one consolidated warning, rather than failing the stream and abandoning config generation for the rest of the tenant. Do not stream it identity-only with a nil `Resource`: the framework documents that as merely warning, but Terraform CLI 1.15 panics rendering it in `genconfig` (`cty.Value.GetAttr` on a null).

**Testing.** The predicate takes unit tests, but the *wiring* needs acceptance tests, and the fixture cannot be created through the provider — Create refuses it. The CI tenant keeps two permanent profiles named `Fidelity Test Sentinel [DO NOT DELETE]` (a macOS Login Window with `&` in `LoginwindowText`, and a mobile Web Clip with `&`/`<` in `Label` and `&` in `URL`), authored via admin-UI upload so they hold true `&`/`<` rather than the pre-mangled form. Acceptance tests resolve them **by name, never by ID**, and skip with a message describing the fixture when absent.

## Error Handling

Use the shared helpers from `internal/common/helpers` rather than rolling your own:

- `helpers.IsNotFoundError(err)` — 404 detection in `Read`/`Delete` operations.
- `helpers.IsServerError(err)` — 5xx detection for retry decisions.
- `helpers.ResolveTimeout(ctx, value, defaultDuration)` — resolve `framework-timeouts` values to a concrete deadline.
- `helpers.ReconcileOptionalBool` / `ReconcileOptionalInt` / `ReconcileOptionalString` / `Reconcile*Pointer` — preserve Terraform null/optional semantics when the API returns zero values.
- `helpers.PreserveStringWhenWireEmpty(wire, current)` — for user-authored string fields where the Classic API echoes empty strings after successful writes. Defends against the masked-error-path bug when the field is a sibling of a `Sensitive` attribute. See §"State builders" rule 4a.
- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the error chain.

## Naming Patterns

### Resources

Terraform construct names follow `jamfplatform_<domain>_<entity>` (or `jamfplatform_<domain>_<entity>_<verb>` for actions):

- `jamfplatform_device_group` (resource, data source, list resource)
- `jamfplatform_blueprints_blueprint` (resource, data source, list resource)
- `jamfplatform_cbengine_benchmark` (resource, data source, list resource)
- `jamfplatform_device_erase` / `_restart` / `_shutdown` / `_unmanage` (actions)
- `jamfplatform_security_cloud_dns_zone` / `_ztna_gateway` / `_ztna_grouped_gateway` (resource, data source, list resource)

### Test names

Test functions use the pattern `TestAccResource_<Resource>_<Scenario>` for acceptance tests and `Test<Function>_<Case>` for unit tests:

```go
func TestAccResource_DeviceGroup_SmartComputer(t *testing.T) { ... }
func TestAccDataSource_Baselines(t *testing.T) { ... }
func TestBuildBlueprintInput_MinimalConfig(t *testing.T) { ... }
```

### Acceptance test resource names

Use the `tf-acc-` prefix for all resources created during acceptance tests:

```
tf-acc-static-computer
tf-acc-benchmark-all-rules
tf-acc-bp-scope-passcode
```

## Jamf Account Resource Naming

Resources backed by the `account/` package of `jamfplatform-go-sdk` carry the
`jamfplatform_account_` prefix. This is the provider's **organization-level** family: the
objects a practitioner manages at *account.jamf.com → Organization*, above and across
individual Jamf Pro, School, Protect and Security Cloud tenants.

Three layers disagree about what to call this family, so the choice is recorded rather than
inferred: the SDK package is `account`, the gateway prefix is `sso`, and the spec bundle is
`account-sso`. The Terraform name follows the **product surface** a practitioner navigates —
Jamf Account — because the gateway prefix is a routing artifact and because the family will
grow past SSO (licensing, deal registrations, distributor operations all live in the same SDK
package).

### Rules

1. **Terraform construct name**: `jamfplatform_account_<resource>`. The SSO constructs are
   `jamfplatform_account_sso_connection` and `jamfplatform_account_sso_domain` — the `sso_`
   qualifier comes from the UI section, because "connection" and "domain" are both ambiguous
   on their own.
2. **Do not confuse it with Jamf Pro's SSO.** `jamfplatform_pro_sso_settings` and
   `jamfplatform_pro_sso_failover_url` configure a single Jamf Pro tenant's own SSO. They are
   a different product, a different endpoint and a different scope. The prefix is what keeps
   them apart, which is why the family is not named `jamfplatform_sso_*`.
3. **Go package path**: `internal/resources/account/<resource>/` — flat, one leaf package per
   construct family, folder name equal to the Terraform slug minus the `jamfplatform_account_`
   prefix. Actions live under `internal/actions/account/<resource>/`, one package per resource
   with a file per verb, as in `internal/actions/pro/mdm/`.
4. **Organization scope only, via `providerdata.ConfigureAccount`.** Do not hand-roll
   Configure and do not route through `ConfigurePro` — there is no customer-tenant version to
   read, and an organization may hold no Jamf Pro tenant at all. The gate is
   `ScopeOrganization` alone; see [§API integration scope](#api-integration-scope-every-construct-declares-which-scopes-reach-it).
5. **No organization ID travels anywhere.** An organization-scoped integration authenticates
   on client id and secret alone, and the gateway resolves the organization from the access
   token; `/sso/v1` ignores `X-Environment-Id` and `X-Tenant-Id` when either is sent. There is
   no `organization_id` provider attribute and no SDK `WithOrganizationID`. A
   `X-Organization-Id` header does exist and is parsed by the gateway, but nothing needs to
   send it.
6. **US gateway only.** The namespace is absent from the EU gateway — a bogus route under
   `/sso/v1` there returns the gateway's own bare `404 page not found`, versus
   `403 BAD_PERMISSIONS` in US. Acceptance tests skip on a non-US base URL, and the
   region limitation belongs in user-facing documentation.
7. **Acceptance tests use `testhelpers.AccPreCheckAccount`, never `AccPreCheck`.** The latter
   skips unless a scope variable is set, and this family requires both to be *unset* — so
   routing these tests through it would skip every one of them while still reporting green.
8. **Import by domain name is a sanctioned exception to [§Import format](#import-format).**
   `jamfplatform_account_sso_domain` imports by name, not id, for two reasons that both have to
   hold before an exception is granted: nothing in the API reads a domain by id (the per-id GET
   is unrouted), and the id is **not stable** — removing and re-claiming a domain mints a new
   one. Where an id is neither usable nor stable, the name is the primary key. Note the
   acceptance harness defaults `ImportStateId` to the `id` attribute, which is exactly the value
   `ImportState` rejects, so a name-imported construct must set `ImportStateId` explicitly.
9. **Canonicalise by validator, not by plan modifier.** The server lower-cases a stored domain.
   Because `domain` is `Required`, [§Plan-modifier rewrites are NOT a valid option for `Required`
   attributes](#plan-modifier-rewrites-are-not-a-valid-option-for-required-attributes) rules out
   rewriting the value, so strict acceptance plus a diagnostic naming the spelling to use is the
   only correct option. Lookups stay case-insensitive so an import still forgives mixed case.

### Jamf Account shapes that recur

Wire-probed 2026-09-02 against two organization-scoped integrations.

- **A collection may answer with either an envelope or a bare array.** The SDK's
  `UnwrapResults` tolerates both, and its own tests pin both shapes after a period when the
  account lists shipped broken for five days. Do not assume the declared envelope.
- **No RSQL filter exists on any account collection GET**, so a list resource here is an
  unfiltered stream — as in AI Governance.
- **`403 BAD_PERMISSIONS` on a spec-advertised route means unrouted, not unprivileged**, the
  same law as [§Jamf Security Cloud](#jamf-security-cloud-resource-naming). It is how the
  absence of a per-id domain read and of any domain update was established. The one case where
  it can be read as a genuine authorization refusal is when a *different* credential answers
  `200` on the same URL in the same region — then the route is provably mapped.
- **`errors[].field` is populated only for top-level required fields.** An invalid enum comes
  back as `MALFORMED_REQUEST_BODY` with `field: null` and the offending value never named, and
  a discriminator/payload mismatch is an unattributed `500`. So every enum, and every
  discriminator-to-payload pairing, must be validated at plan time from the SDK's own
  `*Values()` helpers — the server will not attribute the mistake for you.

## Jamf Security Cloud Resource Naming

Resources backed by the `securitycloud/` package of `jamfplatform-go-sdk` carry the
`jamfplatform_security_cloud_` prefix. The SDK generates all five Security Cloud API
namespaces — `jsc-categories`, `jsc-dns`, `jsc-ztna`, `securitycloud-devices` and
`uem-connect` — into that one Go package, but those namespace names are gateway routing
artifacts that appear nowhere in the admin UI, so they are not part of the Terraform name.

### Rules

1. **Terraform construct name**: `jamfplatform_security_cloud_<resource>`, regardless of
   which API namespace inside the SDK package backs it. Same reasoning as
   [§Jamf Pro Resource Naming](#jamf-pro-resource-naming): the API-layer split is not the
   user's problem.
2. **Slug source**: the admin-UI label, not the SDK filename, wherever the two diverge.
   `zones.go` under *Integrations → Custom DNS → DNS zones* becomes
   `security_cloud_dns_zone` — the `dns_` qualifier comes from the UI section, because
   "zone" alone is ambiguous once ZTNA gateways and device groups are also in the
   namespace.
3. **Singular vs plural**: as for Jamf Pro — singular resource and by-id/by-name data
   source, plural data source and list resource.
4. **Go package path**: `internal/resources/security_cloud/<resource>/` — flat, one leaf
   package per construct family, folder name equal to the Terraform slug minus the
   `jamfplatform_security_cloud_` prefix. Code shared across two or more Security Cloud
   packages lives under `internal/common/<topic>/`, never in a sibling.
5. **No tenant version gate.** Security Cloud is continuously deployed and has no
   customer-tenant version, so these constructs declare no `minJamfProVersion` and must
   **not** be routed through `ConfigurePro` / `ConfigureProClassic` — those fetch the Jamf
   Pro version, which a Security Cloud-only tenant cannot answer.
6. **Entitlement is separate from authentication.** A tenant can authenticate fine and
   still not have a Security Cloud surface; the API says so with `403 NOT_ENTITLED`.
   Translate that code into a named diagnostic rather than letting the raw error through.

### Security Cloud shapes that recur

These patterns show up across the Security Cloud namespaces often enough to state once here
rather than rediscover per resource. Wire-probed on 2026-08-27 unless a later date is given.

**A single-element array is a scalar.** Several fields are declared as arrays but rejected at
any size but one — the IPsec cipher suites' `encryption`, `integrity` and `dhGroups`, and the
Jamf-side `left.subnets`. Model these as a single string or number and collapse at the
boundary in the input/state builders. A list would only offer users room the server refuses,
and the refusal message names the field's size rather than the mistake.

**An enum violation is unattributed.** An unknown enum value comes back as
`400 [INVALID_FIELD] Request body is missing or malformed.` — no field, no offending value.
Validate every enum at plan time, and build both the `OneOf` validator and the documented
value list from the SDK's generated `*Values()` helper so they cannot drift from each other or
from the API. Where the SDK documents a set but generates no helper (grouped gateway's
`recoveryDelayInSec`), restate it in the package's `enums.go` and say why in a comment.

**Whether a referenced object can be deleted is per construct — probe it.** For *gateways*,
Jamf Security Cloud refuses to delete anything another object points at — a gateway named by a
DNS zone's name server or by a grouped gateway's membership — with `409 CONFLICT` and no
structured detail identifying the referrer. That is the Terraform destroy-ordering trap
described in
[§Referenced-by-name dependencies](#referenced-by-name-dependencies--has_dependencies-on-delete):
removing the reference *and* destroying the target in one apply lets Terraform sequence the
destroy first. Translate the 409 into a diagnostic that names the possible referrers and says
to split the applies, because the server will not.

**Device groups behave the opposite way** (wire-probed 2026-08-29), which is why this is a
probe and not a rule: deleting a group that a ZTNA app's `assignments.inclusions.groups` still
names returns `204`, and the app is silently left at `allUsers: false` with an empty group list
— a combination the API rejects on *write*. There is no 409 to translate and nothing for
Terraform to sequence, so the hazard belongs in the resource's description rather than in a
diagnostic. Do not carry either behaviour across from one construct to another; probe the
delete of a referenced object per construct.

**An unmapped route answers `403 BAD_PERMISSIONS`, not `404`** (wire-probed 2026-08-29). A
`GET` on a deliberately bogus Security Cloud path returns exactly the body a genuinely
unprivileged call does, so the status is ambiguous by construction and **must not be
translated into a "grant this privilege" diagnostic** — it would be wrong half the time.
Two consequences. First, when the bundled spec advertises an endpoint the gateway 403s on,
that is evidence the route is unmapped rather than that a privilege is missing: confirm it by
probing a bogus path with the same token, and by checking a sibling call under the same
privilege still succeeds. Second, this is *not* the entitlement failure: `403 NOT_ENTITLED` is
a different code and does deserve its own named diagnostic.

**A `400` from the gateway is not always a bad request.** `GET /securitycloud/v2/groups` under a
tenant-scoped credential with no Security Cloud entitlement answers
`400 [URI_NOT_TEMPLATED] Failed to (fully) template URI to the downstream service`, while
`GET /securitycloud/v1/ztna/apps` on the same credential in the same invocation answers the usual
`403 BAD_PERMISSIONS` (probed 2026-09-04). So this namespace has a **third** failure shape beyond
the `403` and the `400 INVALID_FIELD` above, it is per-route rather than per-service, and it says
nothing about the request body — do not translate it into a validation diagnostic. A
tenant-scoped operator without the entitlement can therefore see either code from the same
construct depending on which endpoint runs first.

**"Unrouted" is a dated snapshot, not a property of the endpoint**, and the exemplar of the
rule above is the case that proves it. `PUT /securitycloud/v2/groups/{id}` 403'd on 2026-08-29
while `PUT /securitycloud/v1/groups/{id}` returned 200 under the same `device-groups:update`,
so `security_cloud_device_group` called the deprecated v1 write under a staticcheck
suppression. The 403 cleared on 2026-09-03 when the authorization policy deployed, and the
handler behind it then 404'd until it was fixed on 2026-09-04 — probed at 12:51 (404) and
13:33 (success) the same day. Two lessons. A `403` clearing is **not** the same event as the
capability arriving, so a route that stops 403ing still needs its success probed, and a
handler that accepts a write and discards it would be worse than the refusal — read the change
back through a *different* operation. And a suppression justified by an unrouted successor is
temporary by construction: it names a date and an upstream report, and the next SDK bump is
where it gets re-probed. SDK v0.22.0 withdrew both v1 write paths with the spec, so that one
is gone.

**A read-only status timestamp does not belong in the schema.** The gateway's
`status.updatedAt` and the grouped gateway's `updatedAt` advance on every server-side
re-evaluation. A Computed attribute over such a value makes every refresh report the object as
changed outside Terraform, about something no configuration can act on. Omit it and say so in
the state builder's doc comment; keep the fields that settle (`status.state`,
`status.tunnelState`) and the ones that never move (`createdAt`).

### Security Cloud Configure: use the `providerdata.ConfigureSecurityCloud` helper

```go
func (r *Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_<name>")
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }
    r.client = client
}
```

It type-asserts `*providerdata.Data`, applies the `RequireScope` gate
(`SecurityCloudScopes`, which the SDK registry resolves to environment-or-tenant) and
returns a `*securitycloud.Client`. It deliberately does not go through `configureSub` —
see rule 5.

Acceptance tests gate on `testhelpers.AccPreCheckSecurityCloud`, which skips unless the
operator has declared that the configured scope belongs to a Security Cloud tenant; see
[TESTING.md](TESTING.md).

## Jamf Pro Resource Naming

Resources backed by the `pro/` or `proclassic/` packages of `jamfplatform-go-sdk` use a dedicated naming scheme that hides the API-layer split from users. Both layers expose Jamf Pro functionality; end users should not care which one is wired up.

### Rules

1. **Terraform construct name**: `jamfplatform_pro_<resource>` for every Jamf Pro resource, data source, list resource, or action — regardless of whether it is sourced from `pro/` or `proclassic/`.
2. **Slug source**: derive `<resource>` from the SDK filename, normalized to snake_case and singularized. Examples:
   - `pro/scripts.go` → `jamfplatform_pro_script`
   - `pro/categories.go` → `jamfplatform_pro_category`
   - `pro/smart_computer_groups.go` → `jamfplatform_pro_smart_computer_group`
   - `proclassic/networksegments.go` → `jamfplatform_pro_network_segment`
   - `proclassic/osxconfigurationprofiles.go` → `jamfplatform_pro_macos_configuration_profile` (override; modern terminology)
3. **Singular vs plural**:
   - Resource: singular (`pro_script`).
   - Data source: singular for ID/name lookup (`pro_script`), plural for filtered/list lookups (`pro_scripts`).
   - List resource: plural (`pro_scripts`).
   - Action: singular verb suffix (`pro_computer_erase`).
4. **Go package path**: `internal/resources/pro/<resource>/` (flat — no domain tier). Every Pro resource is its own leaf package directly under `pro/`. The `<resource>` folder name is the **Terraform slug minus the `jamfplatform_pro_` prefix**, snake_case. Examples: `jamfplatform_pro_category` → `pro/category/`; `jamfplatform_pro_self_service_plus_settings` → `pro/self_service_plus_settings/`; `jamfplatform_pro_macos_configuration_profile` → `pro/macos_configuration_profile/`. Do not drop descriptive suffixes (keep `_settings`, `_group`, etc.) — the folder name must match the Terraform slug exactly so future maintainers can grep one to find the other. The Go package declaration matches the folder name verbatim. Code shared across two or more Pro packages lives under `internal/common/<topic>/`, never in a sibling resource package.
5. **Pro vs ProClassic preference**: default to `pro/`. Use `proclassic/` only when:
   - `pro/` has no equivalent endpoint, OR
   - `pro/` is materially less feature-complete (e.g. read-only when classic offers CRUD, missing required fields).
   - When both are wired across multiple resources, document the rationale in the resource's package-level comment.
6. **Overrides**: where the SDK filename is awkward, outdated, or ambiguous, override the Terraform slug. Record the override in `JAMF_PRO_INVENTORY.md` (gitignored planning file) at the time the batch is approved. There is no upfront override table — decisions happen per batch.
   - **Prefer the admin-UI label over an API-namespace artifact when they diverge.** The default slug source is the SDK filename (rule 2), but when that name is a wire/namespace artifact absent from the admin UI, name the construct after the UI instead — the same principle as [§Attribute names mirror the Jamf Pro admin UI](#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic), applied to the construct. Examples: `jamfplatform_pro_computer_check_in_settings` (pro V3 SDK namespace `client_check_in` — "client" appears nowhere in the UI, which reads *Computers → Check-in*; this also matches the classic `computercheckin` it supersedes); `jamfplatform_pro_mdm_profile_settings` (pro V1 SDK type `DeviceCommunicationSettings` — the UI panel is "MDM profile settings"). When two endpoints (e.g. a pro and a classic face) are the same settings object, pick one UI-aligned slug and mark the other **superseded** in the inventory.
7. **Inventory tracking**: every Jamf Pro construct (planned, in-design, in-progress, shipped, skipped) is tracked in `JAMF_PRO_INVENTORY.md`. Not committed.

### Minimum Jamf Pro version check

The provider uses **one** `jamfplatform.Client` built from the standard `JAMFPLATFORM_*` credentials — there is no separate "Pro credentials" set. The same client serves Platform Services resources (`blueprints/*`, `cbengine/*`, `device_group`, etc.) and Jamf Pro resources (`pro/*`).

Pro resources gate themselves on a minimum Jamf Pro tenant version. Platform Services resources do not — they remain usable against tenants that don't have Jamf Pro provisioned.

Every Pro resource declares its minimum Jamf Pro version as an unexported `const` in `resource.go`:

```go
const minJamfProVersion = "11.5.0"  // empty string = no version check
```

- **Co-locate** with the resource. Do not centralize.
- **Empty string** (`""`) skips the check — use only when the resource genuinely works on the provider's declared minimum Pro version with no endpoint-specific floor.
- **Source the value** during the SDK-comparison phase of the resource-addition workflow ([CONTRIBUTING.md §Adding a Jamf Pro Resource](CONTRIBUTING.md#adding-a-jamf-pro-resource)). Record in `JAMF_PRO_INVENTORY.md`.

The tenant version is fetched **lazily** inside `providerdata.ConfigurePro` via `pd.GetJamfProVersion(ctx)`, which uses `sync.Once` to fetch via `GetJamfProVersionV1` exactly once per Data value and caches the result. Subsequent Pro Configure calls reuse the cached value.

#### `providerdata.Data` lives in its own package

The value Terraform passes to every Configure call (`req.ProviderData`) is `*providerdata.Data` — defined in `internal/providerdata/providerdata.go`, **not** `internal/provider/`. It carries the SDK client plus the `sync.Once`-cached lazy Pro version fetch.

- **Why a separate package**: `internal/provider/` already imports every resource package in order to register them. If resource packages also imported `internal/provider` to name the `Data` type, Go would reject the cycle. `internal/providerdata/` is a leaf package — resources import it, provider imports it, no loop.
- **Why a wrapper at all**: Platform Services resources only ever needed the raw `*jamfplatform.Client`, so the provider used to pass it directly as `ResourceData`. Pro resources require lazily-fetched, cached tenant Jamf Pro version state (so dozens of Pro resources in one config don't each re-call `GetJamfProVersionV1`). A `sync.Once` field on a shared struct is the natural shape, hence the wrapper.

#### Pro Configure: use the `providerdata.ConfigurePro` helper

Every Jamf Pro resource, data source, list resource and action funnels its Configure through `providerdata.ConfigurePro` — a single helper that performs the type assertion, fetches the cached tenant version, runs the per-resource version gate when set, and appends the provider-floor advisory warning when applicable. Do not hand-roll the boilerplate.

```go
const minJamfProVersion = "11.5.0"  // empty string = no version check

func (r *Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_<name>")
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }
    r.client = client
}
```

The helper returns `(*pro.Client, diag.Diagnostics)`. When `req.ProviderData` is nil (early framework lifecycle) it returns `(nil, nil)` — leave `r.client` unset and let the next Configure call populate it. Data sources and list resources use the same one-liner shape; only the request/response types differ.

Platform Services resources do not use `ConfigurePro` — they only need the raw client. They type-assert `req.ProviderData.(*providerdata.Data)` and read `.Client` directly, then gate on API integration scope (below).

#### API integration scope: every construct declares which scopes reach it

A Jamf API integration is created against one of three scopes, and the provider exposes all three: **Platform environment** (`environment_id` → `X-Environment-Id`) is the **preferred** scope; **Tenant** (`tenant_id` → `X-Tenant-Id`) is the legacy one Jamf documents as "targeting integrations without a platform environment"; **Organization management** is neither attribute set — no scope header, context resolved from the access token. The two attributes are mutually exclusive and **both optional**, so a construct cannot assume a scope was configured at all. Every construct's Configure therefore gates on `providerdata.RequireScope`, naming the scopes it can actually be reached under, **in preference order**:

```go
resp.Diagnostics.Append(pd.RequireScope("jamfplatform_<name>", providerdata.BlueprintsScopes...)...)
if resp.Diagnostics.HasError() {
    return
}
```

**Never write the kinds out at a call site.** Pass one of the derived family sets in `internal/providerdata/scopes.go` — `ProScopes`, `SecurityCloudScopes`, `AccountScopes`, `AIGovernanceScopes`, `BlueprintsScopes`, `ComplianceBenchmarksScopes`, `DevicesScopes`, `DeviceGroupsScopes`, `DeviceActionsScopes`. Each is resolved at initialisation from the SDK privilege registry's `MethodPrivileges.Scopes` (SDK v0.22.0), which carries the scope kinds the endpoints are published at, so a spec ingest that moves a family arrives as **one changed value with a pinned expectation to agree with** rather than going stale silently at 27 call sites. `scopes_test.go` pins every set, asserts the family's methods agree with each other, and fails on a widening entry the registry has caught up with.

- **Pro and ProClassic constructs need no call site** — the gate is applied once inside `configureSub`, so `ConfigurePro` / `ConfigureProClassic` already enforce `ProScopes` for every `pro/` package and Pro action.
- **Security Cloud constructs need no call site either** — `providerdata.ConfigureSecurityCloud` applies `SecurityCloudScopes` (see [§Jamf Security Cloud Resource Naming](#jamf-security-cloud-resource-naming)).
- **Platform Services constructs wire it explicitly**, immediately after the `*providerdata.Data` type assertion.
- **Jamf Account constructs need no call site** — `providerdata.ConfigureAccount` applies `AccountScopes` (see [§Jamf Account Resource Naming](#jamf-account-resource-naming)).
- **Argument order is presentation order** — it is the order the diagnostic lists the scopes in, so environment comes first. The derived sets are already ordered that way; do not re-order them at a call site.
- **Do not gate in provider Configure instead.** The answer differs per API family, and the Platform API GA proved it twice over: Blueprints and Compliance Benchmarks are now environment-only, while a provider-level assertion would also block the organization-level constructs that legitimately run without a scope header.
- **Organization scope is not a rejected special case.** It is the only scope that reaches the `jamfplatform_account_*` family, and it is rejected everywhere else. That two-way enforcement is the point of the gate: it converts an opaque `403` mid-apply into a named diagnostic at Configure, in both directions.

**A spec-declared set is not always what the gateway serves.** The GA deleted `X-Tenant-Id` from six Platform specs while the gateway went on answering it, so `scopes.go` carries a `gatewayWidenings` table: a family, a scope, and the wire evidence with its date. Three entries today — Platform devices, device groups and device actions — wire-verified 2026-09-04 by two independent probes, and confirmed by the acceptance suite running green under tenant scope for `jamfplatform_device_group` and `jamfplatform_devices`. That last part is the evidence a status code cannot give: the constructs work end to end on a credential the spec says should not reach them, so narrowing them would break working configurations. An entry is an **assertion about the gateway**, the provider-side counterpart of the SDK's own `config.scopeTypes` override, and is **deleted rather than edited** when either half of its justification goes.

**A narrowing needs evidence of its own, and a `403` is not it.** Blueprints and Compliance Benchmarks are deliberately absent from that table, and the reason is not that a `403` classified them: two *different* tenant-scoped credentials are refused `BAD_PERMISSIONS` on both, and two credentials that each lack the capability distinguish unrouted from unprivileged no better than one does (see §Security Cloud shapes that recur). The proof that a shared `403` settles nothing is in the same list — Security Cloud answers it identically and is reachable under both scopes.

What decides it sits outside the API. **Jamf Account's permission picker does not offer the `blueprints` or `compliance-benchmarks` capabilities when the integration being created is tenant-scoped** (observed in the picker, 2026-09-04), so no tenant credential can ever hold them whichever the gateway is doing with the route, and no configuration a practitioner could write is taken away by refusing them. Note what falsifies that: a change to the picker, not a `200` from either route. So when you narrow a family, say which evidence you have and where it came from. An ungrantable capability is a decision you can act on, an ambiguous `403` is not, and a narrowing recorded without either is a guess that reads like a finding.

Scope resolution itself (config beats environment, both-at-once is an error, a shadowed environment variable warns) lives in `internal/provider/scope.go`; the `ScopeKind` vocabulary and the gate live in `internal/providerdata/scope.go`, and the derived family sets in `internal/providerdata/scopes.go`. `providerdata.New` derives the configured scope from the SDK client's own `Client.Scope()` rather than taking it as a parameter, so a caller cannot build a `Data` whose declared scope differs from the header its client sends.

Scope is also **documented**, not only enforced: `permissions.Section` opens its "Required Jamf permissions" lead-in with the integration scope the endpoints are published at, derived from the same registry as the table. That line reports what the **spec** declares and deliberately omits the widenings — a published table should carry Jamf's declaration rather than this provider's tolerances, and `RequireScope`'s diagnostic is the authority on what a given release accepts.

#### Failure modes

| Condition | Outcome |
|-----------|---------|
| Tenant does not have Jamf Pro provisioned, Pro resource with non-empty `minJamfProVersion` used | `GetJamfProVersionV1` errors → propagated by `GetJamfProVersion` → resource Configure surfaces as "Failed to read Jamf Pro tenant version" |
| Tenant does not have Jamf Pro provisioned, Pro resource with empty `minJamfProVersion` used | Configure passes; first CRUD call surfaces the underlying Jamf API error |
| `GetJamfProVersionV1` returns a transient network/auth error | Same path as above; cached on `ProviderData` until next `terraform` invocation |
| Tenant version parses, `< required` | `RequireMinJamfProVersion` error |
| Tenant version unparseable | `RequireMinJamfProVersion` error with raw string |
| `minJamfProVersion = ""` | Version check skipped; no fetch triggered by this resource |

### Endpoint adoption & migration policy

**Scope: Jamf Pro resources only** (those backed by `jamfplatform-go-sdk/jamfplatform/pro/` or `proclassic/`). Pro APIs versionize endpoints (V1 / V2 / V3 / ...), can be deprecated by Jamf, and run on customer-provisioned Jamf Pro tenants with their own version drift. This policy governs when Pro resources move between endpoint versions. **All version transitions follow the same rules** — V1→V2, V2→V3, V3→V4, and so on. References to "V1" and "V2" below are generic placeholders for "current version N" and "new version N+1".

**Out of scope: Platform Services SDK packages** — `blueprints/`, `compliancebenchmarks/`, `devicegroups/`, `devices/`, `deviceactions/`, `ddmreport/`. These are continuously-deployed Jamf Platform microservices, not customer-versioned Jamf Pro endpoints. Always track the latest stable function in the SDK; no deprecation buffer, no quarterly audit, no annotation block required. When a Platform Services SDK function is updated (e.g., a new V2 added), migrate the corresponding resource opportunistically — typically as part of the SDK dependency bump that introduces it.

#### Platform Services *Terraform schema* deprecation — 90-day window

The endpoint-version policy above governs *SDK/endpoint* moves and does not cover **Terraform schema** changes. When a Platform Services resource (`blueprints/*`, `cbengine/*`, `device_group`, …) must reshape its Terraform surface in a backward-incompatible way (rename/retype/supersede an attribute), ship it **additively first**, then remove after a short window:

1. **Add** the new attribute; keep the old one working.
2. **Mark the old attribute `Deprecated`** with a user-facing `DeprecationMessage` that names the replacement **and the earliest removal date**.
3. **Annotate it in code** with a greppable marker so removals can be batched later:
   ```go
   // PLATFORM-DEPRECATED remove-after=YYYY-MM-DD replaced-by=<new_attr> — <one-line reason>
   ```
4. **Window: 90 days** from the release that ships the deprecation (Platform microservices move fast and users track the latest — no Pro-style 12-month buffer). On or after `remove-after`, the field is eligible for **batch removal**: `grep -rn "PLATFORM-DEPRECATED remove-after=" internal/resources/`, drop every field whose date has passed, and land the removal with the schema `Version` bump + `UpgradeState` that migrates old state (see [[feedback-no-state-upgraders]] — published state exists, so removals are breaking and need an upgrader).

This applies only to Platform Services resources. Jamf Pro schema deprecations follow the [rename PR / major-version](#jamf-pro-resource-naming) convention instead.

**ProClassic is unversioned**: `jamfplatform/proclassic/` functions do not carry V1/V2 suffixes. Migration timing (below) does not apply to ProClassic-backed resources. Annotation block is still recommended for documenting which SDK function is in use, but `Status:` simply tracks "current" against the SDK release in use.

#### SDK side-by-side dependency

The buffered migration timeline below assumes the SDK exposes both the deprecated version (N) and the new version (N+1) simultaneously during the deprecation window. **This applies to every version transition** — V1→V2, V2→V3, V3→V4, etc. — not just V1→V2.

The SDK does not currently retain side-by-side versioned functions on regeneration (the upstream generator change requested in [jamfplatform-go-sdk#19](https://github.com/Jamf-Concepts/jamfplatform-go-sdk/issues/19) closed without merging). Practical consequences:

- When the SDK exposes both versions of an endpoint: follow the buffer policy below.
- When the SDK only exposes the new version on regeneration: migration is **SDK-bump-driven** — migrate at the SDK bump or pin the SDK to the prior release (only as a temporary workaround). Document the constraint in the resource's annotation block.
- When a Jamf deprecation is announced for an endpoint we use (at any version pair), file an issue against `jamfplatform-go-sdk` requesting retention of the deprecated version through our migration window.

#### Deprecated with no generated successor

The case above assumes the successor reaches us. Sometimes it does not: the SDK marks version N deprecated while generating **no** client for N+1, because N+1 is absent from the bundled OpenAPI spec even though a live tenant serves it. `golangci-lint` runs `staticcheck` by default and `SA1019` is an error, so this combination turns a green build red with no migration available.

When that happens:

1. **Verify the successor on the wire before concluding anything.** The bundled spec is not authoritative about what a tenant serves — probe the real endpoint and record the status code, the presence or absence of a `Deprecation` response header, and whether the query contract (filters, sections, paging) is unchanged. A spec that deprecates every version of a surface with no replacement present is a stale-bundle symptom, not Jamf's intent.
2. **Raise it upstream against `jamfplatform-go-sdk`** with that wire evidence, and reference the issue from every suppression.
3. **Open the provider-side migration tracking issue** within the usual 30 days (see the timeline below). Being blocked on the SDK does not pause the clock — the deadline is set by Jamf's removal date, not by our dependency.
4. **Suppress `SA1019` at the call site**, never repo-wide, with `//nolint:staticcheck // SA1019: <why> — see <where the reasoning lives>`. Keep the full reasoning in one place per package (a package doc or a note beside the shared type) and have the per-line comments point at it.
5. **Do not route around the deprecation with a worse endpoint.** An undeprecated alternative that loses server-side filtering, paging, or a needed field is usually the worse trade — a tracked deprecation beats an unbounded per-call cost. If an alternative is rejected, say so and say why in the annotation, so the next maintainer does not re-litigate it.
6. **Record the status** in the resource's annotation block using the `Status: deprecated by Jamf …` form below, naming the successor version and the blocking issue.

Suppression is a holding position with a deadline attached, not a resolution. The hard floor in the timeline below still applies.

#### Adoption (new endpoints)

| Scenario | Action |
|----------|--------|
| Building a brand-new resource, SDK exposes V1 only | Use V1. |
| Building a brand-new resource, SDK exposes V2 (V1 still current) | **Use V2.** Default to latest stable on new resources — no migration cost. |
| Building a brand-new resource, SDK exposes V2 marked beta/EAP | Use V1. V2 only if V1 is unworkable; document the choice in the resource. |
| Existing **shipped** resource on V1, new V2 lands (V1 not deprecated) | **Do not migrate.** Wait for deprecation, fast-track trigger, or material feature gap. |
| Existing shipped resource on V1, V2 only adds optional new fields | Migrate opportunistically, e.g., when next touching the resource. Not urgent. |
| Existing shipped resource on V1, V2 fixes an issue meeting fast-track criteria | **Fast-track migrate** — see below. |

Rationale: migrating shipped resources costs schema/state-upgrader churn. Don't pay it without reason. New resources have no migration cost.

#### Deprecation migration timeline

Jamf's typical window: ~12 months from deprecation announcement to removal.

| Trigger | Action |
|---------|--------|
| Jamf marks V1 deprecated | Open a migration tracking issue within **30 days**. Schedule the work. |
| **6 months** after deprecation announcement (soft default) | Migration **should be merged**. Soft target. |
| **3 months before announced removal date** (hard floor) | Migration **must be merged**. Hard floor — no shipped resource may sit on a deprecated endpoint inside this window. If migration is not yet shipped, it becomes top priority and bumps the buildout roadmap. |
| Removal date | All migrations must already be shipped. Anything not migrated is a regression. |

The 6-month soft default and 3-month hard floor work as a pair: 6-month catches the common case, 3-month catches edge cases (Jamf shortens the window, a migration slips, a deprecation announcement was missed).

#### Fast-track exception

Skip the 6-month buffer and migrate as soon as feasible when V2 includes one of:

- **Security fix** (auth, scope, data exposure).
- **Data integrity** issue (V1 returns wrong values, silent truncation, race conditions).
- **Breaking bug** affecting a documented common use case where no workaround exists.
- **Critical feature** required by a user-reported issue with no viable workaround.

Maintainer call, documented in the migration PR description.

#### Slow-track exception

When migration would require a **breaking Terraform schema change**, defer to the next provider major version. Document explicitly in the resource's `crud.go` annotation (see below) and in the major-version release planning.

#### Tracking — the migration issue

Every deprecation gets exactly one tracking issue, opened from the **Deprecation migration** issue template (`.github/ISSUE_TEMPLATE/deprecation_migration.md`). The conventions below are mandatory — they are what makes the set of open migrations greppable and their deadlines visible.

**Label**: `deprecation-migration`, always. Add `blocked-upstream` while the SDK exposes no successor, and remove it when the successor ships. No other label substitutes: `grep`ping the label is how the quarterly audit finds outstanding migrations.

**Title format**:

```
deprecation(<sdk-family>): <surface> <from>→<to> — migrate by YYYY-MM-DD
```

- `<sdk-family>` is `pro`, `proclassic` or `platform`.
- `<surface>` is the API path segment, not the Terraform name (`computers-inventory`, not `jamfplatform_pro_computer`). Comma-separate multiple surfaces deprecated in the same announcement rather than opening one issue per surface.
- The date is the **binding** date: the 6-month soft target until Jamf publishes a removal date, then the 3-month hard floor if that lands earlier. Edit the title when the binding date moves — a stale date in a title is worse than none.

**Deadlines**: GitHub issues carry no due-date field, so the dates live on [GitHub Project #2](https://github.com/orgs/Jamf-Concepts/projects/2) in two Date fields — `Deprecated On` (Jamf's announcement date) and `Migrate By` (the binding date, matching the title). Every `deprecation-migration` issue must be on the project with both set. Milestones are reserved for releases and must not be used to carry migration deadlines.

**PR title**: conventional-commit `refactor`, naming the same surfaces as the issue:

```
refactor(<pkg-or-resource>): migrate <surface> <from>→<to>
```

The PR body closes the issue (`Closes #NNN`) and records the fast-track or slow-track call if either applies. One PR per surface unless the surfaces are coupled (e.g. a shared side-channel that cannot move independently).

**While a migration is outstanding**, every call to a deprecated symbol carries a suppression naming the issue, so the reason survives without reading the tracker:

```go
//nolint:staticcheck // SA1019: no v4 client generated yet — see #NNN
```

**Closing condition**: the issue closes only when no call site reaches the deprecated method *and* every `SA1019` suppression for that surface is gone — `grep -rn "SA1019" internal/` is the check. A migration that leaves suppressions behind is not finished, because the next audit cannot tell it from an un-started one.

#### Tracking — in-code annotation (Pro resources only)

Every Jamf Pro resource's `crud.go` carries an annotation block at the top of the file listing the SDK endpoints in use, their status, and the last maintainer review date. Platform Services resources do **not** carry this annotation.

```go
// SDK endpoints used:
//   pro.CreateScriptV2 (introduced Jamf Pro 11.8)
//   pro.ReadScriptV2   (introduced Jamf Pro 11.8)
//   pro.UpdateScriptV2 (introduced Jamf Pro 11.8)
//   pro.DeleteScriptV2 (introduced Jamf Pro 11.8)
//
// Status: current. Last reviewed 2026-05-18.
```

When V1 becomes deprecated, change the line:

```go
// Status: deprecated by Jamf YYYY-MM-DD; migrate by YYYY-MM-DD (6mo soft / 3mo hard floor). Last reviewed YYYY-MM-DD.
```

When the migration is in flight:

```go
// Status: migration in progress on V3 in PR #NNN. Last reviewed YYYY-MM-DD.
```

This annotation is the single source of truth. No separate `ENDPOINT_VERSIONS.md` to keep in sync.

#### Quarterly audit cadence (Pro resources only)

Once per quarter, a maintainer:

1. Scans Jamf Pro release notes and the `jamfplatform-go-sdk` changelog for new Pro endpoint versions and deprecations (Pro / ProClassic only — Platform Services microservices are out of scope).
2. Greps the repo for endpoint annotations: `grep -rA5 "SDK endpoints used:" internal/resources/pro/`.
3. Reconciles each annotation against current Jamf + SDK state.
4. Updates `Last reviewed YYYY-MM-DD` on every annotation that's still current.
5. Opens migration tracking issues for any newly-deprecated Pro endpoints, following [§Tracking — the migration issue](#tracking--the-migration-issue).
6. Reviews every open `deprecation-migration` issue — `gh issue list --label deprecation-migration` — and checks each one's `Migrate By` date on Project #2 against the current window, recomputing it if Jamf has since published a removal date.
7. Flags any annotation whose `Last reviewed` date is older than 120 days (means a prior audit was skipped).

The `Last reviewed` date is the drift detector — any annotation more than a quarter stale signals the audit hasn't happened.

### ID type handling

Jamf endpoints expose mixed ID shapes: `pro/` uses integer IDs for many resources and UUIDs for newer ones; `proclassic/` is almost exclusively integer IDs. Terraform's `ID` attribute is always `types.String`.

**Convention**: always stringify integer IDs in Terraform state. Convert back to `int` / `int64` when calling the SDK.

Helpers in `internal/common/helpers/ids.go`:

- `IntIDToString(id int64) types.String`
- `StringToIntID(s types.String) (int64, error)`
- `StringValueFromIntPtr(*int) types.String` — for ProClassic SDK pointer IDs.
- UUIDs pass through unchanged as `types.String`.

`ImportState` parses the imported string per the rules below before populating state.

### Import format

Default convention for `ImportState`:

- **Single ID**: `terraform import jamfplatform_pro_<name>.foo <id>` — where `<id>` is the resource's primary key (integer or UUID as Jamf exposes it). Standard `resource.ImportStatePassthroughID` usage in most cases.
- **Composite / parent-scoped** (rare): `terraform import jamfplatform_pro_<name>.foo <parent_id>:<id>` — colon-separated. Parse with `strings.SplitN(req.ID, ":", 2)`. Validate both segments before setting state. Document the expected format in the resource's `MarkdownDescription` and in `examples/resources/<name>/import.sh`.

Avoid name-based imports; use IDs only.

### Endpoint shape classification

During the SDK-comparison gate ([CONTRIBUTING.md §Adding a Jamf Pro Resource](CONTRIBUTING.md#adding-a-jamf-pro-resource) step 3), classify the endpoint based on which operations the SDK exposes:

| Operations available | Construct type | Notes |
|----------------------|----------------|-------|
| Create + Read + Update + Delete | `resource` (+ usually a singular `data source` and a plural `list resource`) | Standard CRUD |
| Read only (single + list, or list only) | `data source` (+ plural `data source` or `list resource`) | No state-managed object |
| Update only (no Create/Delete) | `resource` flagged as **singleton** — one record per tenant | See [Singleton resources](#singleton-resources) below for the full convention (fixed ID, Create→Update, no-op Delete, import format). E.g., `activation_code`, `computer_check_in_settings`, `mdm_profile_settings`, `self_service_plus_settings`. |
| Fire-and-forget command (Create returns command ID, no Read/Update/Delete) | `action` | E.g., `pro_computer_erase`, `pro_computer_restart` |

Record the classification in `JAMF_PRO_INVENTORY.md` Notes column during the in-design phase.

### Singleton resources

Jamf Pro objects that exist one-per-tenant and are exposed as Update-only on the API are modeled as **singleton** resources. The whole convention below is the load-bearing definition — any new singleton must follow it.

**Package**: `internal/resources/pro/<resource>/` — its own flat leaf package like every other Pro resource (`activation_code`, `computer_check_in_settings`, `mdm_profile_settings`, `self_service_plus_settings`, `jamf_pro_*`, …). Reference template: `internal/resources/pro/self_service_plus_settings/`.

**ProClassic singletons & secrets returned in clear**: a singleton may be ProClassic (e.g. `activation_code` on `/JSSResource/activationcode`) — funnel `Configure` through `providerdata.ConfigureProClassic` and keep the same Create→Update→GET shape. When a singleton field is a secret that the **GET returns in clear** (the classic activation-code GET echoes the license key verbatim, not a masked sentinel), model it as a plain `Required`/`Optional` + `Sensitive` string: normal drift detection applies and the WriteOnly + `_wo_version` machinery in [§Plaintext secrets](#plaintext-secrets--writeonly-with-_wo_version-rotation-companion) is unnecessary (that pattern is for secrets the API will **not** read back). When several fields share one write call, **always send the full set from state** — a partial PUT can wipe the others (a partial activation-code PUT risks wiping the license), so never build a sparse body.

**Leaf folder name**: matches the Terraform slug exactly (rule #4 above), e.g. `self_service_plus_settings/` for `jamfplatform_pro_self_service_plus_settings`.

**Fixed ID**: every singleton stores `helpers.SingletonID` (`"singleton"`, defined in `internal/common/helpers/singleton.go`) as its Terraform state ID. Set it in Create, Read, and Update via `state.ID = types.StringValue(helpers.SingletonID)`. Do not import any other identifier.

**Identity schema**: declare a single `id` string attribute with `RequiredForImport: true` — same shape as CRUD resources, just always populated with `helpers.SingletonID`.

**Schema `id` attribute**: `Computed: true` with `stringplanmodifier.UseStateForUnknown()`. Never `Required` (users cannot pick the ID — it is always `"singleton"`).

**Create**: the Jamf Pro API has no Create endpoint for singletons (the record already exists). Funnel `Create` into the same `Update<X>V1` SDK call used by `Update`, then `Get<X>V1` to capture authoritative state, set `state.ID = types.StringValue(helpers.SingletonID)`. The post-write `Get` is mandatory — it picks up any server-side transformations, computed defaults, or future field additions without requiring code changes.

**Delete**: no-op on the remote — the record cannot be deleted. The handler signature uses `_` markers on the unused `req` / `resp` parameters to signal the omission is intentional, and emits a single `tflog.Trace` line. No state mutation; Terraform removes the resource from state on its own after the handler returns. Document this in the handler doc-comment so reviewers understand the omitted SDK call is deliberate.

**Defensive nil-client guard**: every CRUD handler (`Create`, `Read`, `Update`) opens with `if r.client == nil { resp.Diagnostics.AddError(providerNotConfiguredError()) ; return }` before reading plan/state. Defense-in-depth against framework lifecycle edge cases or misconfigured provider blocks routing to CRUD with an unconfigured client. Centralize the message in a package-local helper so the wording stays uniform across the three handlers. The `Delete` no-op does not need the guard — it makes no SDK call.

**Import**: `terraform import <type>.<name> singleton`. `ImportState` **must** validate `req.ID == helpers.SingletonID` and reject any other identifier with a clear error diagnostic — silent normalization on the next Read masks the mis-import and confuses users. Pass the validated ID through to `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`. The example `import.sh` under `examples/resources/<type>/import.sh` must contain this exact command.

**No list resource, no plural data source**: singletons have nothing to list. Provide the singular data source only when reading the current settings without managing them is useful.

**State builders — nil-fallback semantics**: SDK structs for singleton fields commonly use pointer types (`*bool`, `*string`) with `json:",omitempty"`. When the API response is nil for a **Required** schema attribute, fall back to the field's zero value with an explicit doc-comment explaining why (Required attributes cannot be null in committed state). When the field is **Optional**, prefer `helpers.ReconcileOptionalBoolPointer` / `ReconcileOptionalStringPointer` so user-set nulls are preserved rather than collapsed. The assigner **must not** write `state.ID` — the CRUD handler is responsible for stamping `helpers.SingletonID` after the assign call, and a test should pin that the assigner leaves any pre-existing ID untouched.

**Acceptance test**: `CheckDestroy` is wired but inverted — it asserts the record **still exists** on the tenant after Terraform destroys the resource from state (the singleton-specific shape of the standard CheckDestroy contract). Test the two Update paths (e.g. toggle a bool true→false), import with `ImportStateId: "singleton"`, and a dedicated step asserting `ImportStateId: "not-the-singleton"` is rejected with `ExpectError` matching the ImportState error summary.

**Unit tests**: a `state_builders_test.go` is mandatory — it pins (a) nil and non-nil round-trip for every assigner, (b) that the assigner does not clobber `state.ID`, and (c) that `helpers.SingletonID` is `"singleton"` (catches accidental drift in the constant).

**Before opening the PR**: run `make fix fmt lint test` (must be clean) then `make generate` to rebuild `docs/resources/pro_<name>.md` and `docs/data-sources/pro_<name>.md`. Commit the generated docs with the source.

### Tenant singletons with a real clear

A third shape sits between [§Singleton resources](#singleton-resources) and ordinary CRUD: an object that exists **one per tenant with no identifier**, like a singleton, but whose `Delete` the API genuinely honours and whose **absence is observable**. Both Jamf Security Cloud custom DNS singletons are this shape — `jamfplatform_security_cloud_dns_search_domain` and `jamfplatform_security_cloud_dns_hostname_mappings`. Do not force one into §Singleton resources: its no-op `Delete` would leave `terraform destroy` silently not clearing tenant-wide configuration that the API is perfectly willing to clear.

**Follow §Singleton resources for** the fixed `helpers.SingletonID`, the identity schema, the `id` attribute shape, the validated import identifier, and the defensive nil-client guards in every handler.

**Diverge on exactly four points:**

1. **`Delete` calls the clear endpoint for real.** Probe whether clearing an already-clear object is idempotent — both DNS endpoints answer `204` either way, so neither needs a not-found special case beyond the usual `helpers.IsNotFoundError` guard.
2. **`CheckDestroy` is the ordinary contract** — assert the object is gone. The inverted singleton `CheckDestroy` (assert it still exists) would pass while the provider silently cleared nothing, which is the failure this shape most invites.
3. **`Read` treats absence as deleted** and calls `resp.State.RemoveResource`. **How absence presents is per construct — probe it, do not inherit it.** The two DNS singletons disagree: an unset search domain is a `404` carrying `SEARCH_DOMAIN_NOT_SET`, while an empty hostname-mapping set is an ordinary `200` with `totalCount: 0`. One is caught by `helpers.IsNotFoundError`; the other has to be recognised from the payload.
4. **`Delete` is unconditional — it clears whatever is stored, not only what this resource wrote.** The clear endpoints take no body and no precondition, so a value an administrator changed out of band since the last refresh is discarded without a diagnostic. It is reachable without any race: `terraform destroy -refresh=false` never looks, and an ordinary destroy whose refresh raced a change in the admin UI is the same outcome. **Probe whether the construct's clear is recoverable.** Where it is not — the stored value cannot be reconstructed from anything Terraform holds — read before clearing and refuse the delete when what is stored is not what state records, so the operator chooses rather than discovers. Where it is recoverable (both DNS singletons are: a search domain is one string and a mapping set is re-declarable), the unconditional clear is acceptable and this paragraph is the record of why.

**`Create` refuses to clobber a value that already existed — and that is a narrower guarantee than it looks.** These endpoints are unconditional upserts that report no conflict, so nothing on the wire separates "creating the tenant's value" from "silently replacing what an administrator set by hand". `Create` therefore reads first and errors with the import command when a value already exists. That preflight read is why the resource's declared SDK methods include the read privilege; `permissions_test.go` catches it if the list is not updated.

Be precise about what that buys. The preflight GET and the write are two unsynchronised requests against an endpoint with **no conditional-write primitive** — no ETag, no `If-Match`, no version field to make the write depend on what the read saw. So the preflight **reliably catches a value that existed before this apply started**, which is the case that actually happens in practice (an administrator configured the tenant by hand, and the operator needs to be told to import rather than overwrite). It **narrows but does not close** the window against a value written concurrently, and it is not an ordering barrier between writers. Two shapes stay reachable:

- **Two blocks in one configuration.** Two instances of a one-per-tenant resource is legal HCL — Terraform's uniqueness is per type *and name*, not per type. With no dependency between them Terraform walks both concurrently at the default `-parallelism=10`, so both preflights can read "unset", both write, the last writer wins, and both land in state under the same fixed `helpers.SingletonID`. Every later plan then proposes an update on the loser, forever. If the walk happens to serialise instead, the second `Create` errors telling the operator to import a value its own sibling just wrote — confusing, but at least loud.
- **Two workspaces, or two CI runs, in the same window.** Same mechanism across processes, and no in-process lock can help.

**A resource cannot detect the duplicate-block case in code.** The framework gives a resource no way to see a sibling instance of itself: `ModifyPlan`, `ConfigValidators` and `ValidateConfig` each receive only the one instance's own configuration, and there is no provider-level hook that enumerates the instances of a type in a plan. So for a one-per-tenant construct **the schema description must carry the warning** — say in the resource's `MarkdownDescription` that the object is one per tenant, that declaring it twice (in one configuration or across workspaces) makes the instances fight, and that adopting an existing value means `terraform import`. Documentation is the only place the constraint can live; do not write a preflight that implies otherwise.

A singular data source is worth providing, and **whether an empty result is an error is the wire's call, not a house style**: reading an unset search domain errors, because a data source silently yielding `""` would feed that into whatever referenced it; reading an empty mapping set returns an empty collection, because a `for` expression over it needs no special case.

### Attribute defaults do not work inside `SetNestedAttribute`

An attribute `Default` (`booldefault.StaticBool`, and by construction its siblings) placed on an attribute **nested inside a `SetNestedAttribute`** overrides a value the configuration set **explicitly**, producing a permanent no-op diff. Verified against a live tenant on 2026-08-29 on `security_cloud/dns_hostname_mappings`: with `Optional + Computed + booldefault.StaticBool(false)`, a configuration saying `connect_to_ztna = true` planned as `false`, so every plan after the first proposed the same change forever. **It reproduces with a single element**, so it is not the set-element correlation problem that usually explains this shape.

Prefer **`Required`** for such an attribute. It removes the mechanism instead of working around it, and where the admin UI shows the control on every row — as it does for both traffic-vectoring checkboxes — every element genuinely has a value to state.

Two things this does *not* say. An `Optional` attribute inside a `SetNestedAttribute` with **neither** `Computed` nor `Default` is unaffected, verified in the same session. And **no unit test catches this** — the schema builds, the state builders round-trip, and every assertion passes. Pin the shape instead: assert `Required` and `Default == nil` in `schema_test.go`, with the reason, so a well-meaning later change to `Optional + Default` fails at the gate rather than in a user's plan.

### Full-replace endpoints & shared backing stores

Some Jamf Pro `PUT` endpoints are **full-replace**: any field omitted from the request body is reset to its server-side default, not left untouched. This is **not** discoverable from the SDK type or the OpenAPI spec — you **must** wire-probe it during the in-design phase. Probe: `GET` the object, `PUT` a body with one field changed and several others omitted, then `GET` again. If the omitted fields reverted to defaults, the endpoint is full-replace. Probe **both directions** (omit field A, then omit field B) — `/v4/enrollment` resets *every* omitted scalar. Record the finding in the spike doc and the `crud.go` annotation block.

Full-replace has three consequences:

**1. Every write must send a complete body.** Build the `PUT` payload from a fresh `GET` (not from plan alone), overlaying the fields the resource owns. Never send a sparse body.

**2. Default `Optional+Computed`+`UseStateForUnknown` for every user-settable optional scalar (omit = preserve).** A TF user must not need to know whether an endpoint merges or full-replaces to predict what omitting a field does. One mental model provider-wide: **declared ⇒ Terraform owns it (drift-reverted); omitted ⇒ preserved.** On a full-replace endpoint that means every user-settable optional field is `Optional + Computed` with `stringplanmodifier.UseStateForUnknown()` (scalars) / `UseNonNullStateForUnknown()` inside nested collections. Mechanism: omit ⇒ plan `Unknown` ⇒ the plan modifier carries the prior state value in ⇒ the input builder (`helpers.OptionalStringPointer`, already drop-on-null) re-emits it ⇒ full-replace keeps it. A *declared* field is a known plan value, the modifier no-ops, so it still drift-reverts — you only lose drift-revert on fields you omit, which is the point. Reference: `jamfplatform_pro_script` (`script_contents` et al.) and `jamfplatform_pro_building` (address scalars).

Carve-outs (a field matching any of these is **not** `Optional+Computed`), and one anti-pattern:
- **API-required ⇒ `Required`** (wire-probe / SDK / spec): if a create/update is rejected without the field, it is `Required` — no `Optional`, no `Computed`.
- **Read-only / server-derived ⇒ `Computed`-only** (`id`, derived `*_name`, echoes the user never sets — see [§Server-derived computed fields](#server-derived-computed-fields--optionalcomputed-attributes)).
- **Write-only secret objects ⇒ `Optional`-only, preserve-on-omit** — keystores / signing certs are not `Computed` (the server never echoes them back to absorb). See [§Write-only secrets are the exception to full-replace](#full-replace-endpoints--shared-backing-stores) below.
- **Individually-optional *nullable* fields inside a full-replace block ⇒ `Optional`-only** — when the server echoes `null` (no default to absorb) and rejects a blank/zero on a present field, model `Optional`-only and omit unset fields. See [§Individually-optional nullable fields](#full-replace-endpoints--shared-backing-stores) below.
- **Discriminator-gated fields** (only valid when a sibling discriminator has a given value — e.g. `script` only with `input_type = SCRIPT`; see [§Cross-field validation](#cross-field-validation)) split by whether the field is **required** or **optional** when its type is active:
  - **Required-when-active** (e.g. `script` when SCRIPT): always declared when valid, so there is no omit path to protect — keep plain `Optional`. Plain `UseStateForUnknown` is actively *wrong* here: it carries the prior value into the plan on a discriminator transition (e.g. SCRIPT→TEXT, config omits `script`), the input builder sends it, and the server `400`s on the now-foreign field (the ConfigValidator checks `req.Config`, not the USFU-mutated plan, so it won't catch it).
  - **Optional-when-active** (e.g. `popup_menu_choices` when POPUP): plain `Optional` leaves the wipe-on-omit footgun. To convert it safely you must **(a)** gate the input builder so a companion field is sent *only* when its discriminator value is active (so a stale value can never reach the wire on a transition), and **(b)** use a **discriminator-aware** plan modifier — `UseStateForUnknown` behaviour while the type is active, but predict the cleared value (`SetNull`/null) when the discriminator has moved away, so a transition does not trip "inconsistent result after apply". Reference: `computer_extension_attribute` `popupChoicesPlanModifier` + the `switch inputType` in its input builder.
- **Anti-pattern — `Optional+Computed` *without* the plan modifier.** That genuinely is a silent wipe: an omitted field drops from the `PUT` and full-replace clears it, while the schema reads as "preserve." The `UseStateForUnknown` plan modifier is load-bearing, not optional — never ship `Optional+Computed` on a full-replace field without it.

The default (`Optional+Computed`+USFU) is for plain owned scalars the server defaults to a stable value; the carve-outs above (secrets, individually-nullable blocks) are detailed in their own sub-sections below and take precedence over the default when they apply.

**Description convention.** End each reworked `Optional+Computed` *string* field's `MarkdownDescription` with the standard sentence so users see one consistent contract: `Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.` Sets clear with `[]`; bool/int/enum have no blank (omit preserves, set a concrete value to change) — word those accordingly rather than promising a `""` clear.

**Acceptance coverage.** Each full-replace resource carries at least one *split-ownership* test proving omit=preserve on a representative co-managed field: create without the field in HCL, set it out-of-band via the acc client (simulating a UI edit), change an unrelated managed field (e.g. `name`), assert the out-of-band value survived; then set `""`/`[]` and assert it clears. One representative field per resource — the field-by-field coverage is the existing create/update/import suite. (`TestAccResource_ProScript_SplitOwnership`, `TestAccResource_ProBuilding_SplitOwnership` are the references.)

This applies uniformly to **all** full-replace resources, including fully-owned **singleton settings** — the provider is in development, so the older "prefer `Required` for singletons" bias has been dropped. `Required` is now reserved for the API-required carve-out only. (`jamfplatform_pro_re_enrollment_settings` is the reference singleton conversion.)

**Singletons: GET-on-create to adopt, not clobber.** `UseStateForUnknown` only preserves an omitted field on *update* (it carries the prior state in); on *first create* there is no prior state, so an omitted field drops from the write and full-replace resets it to the server default. For a **multi-instance** resource that is the correct behaviour — "create" is a brand-new record, there is nothing to preserve. But a **singleton always pre-exists**, so "create" is really adoption: read the live object in `Create` (inside any write lock) and merge the user's declared fields over it, so toggles the user did not declare keep their current value on the first apply too. Do this **where the endpoint allows it** — a future singleton may wire-probe a behaviour (e.g. create-vs-update on different verbs, or fields the read does not echo) that makes it impractical; decide per resource from the probe. Reference: `jamfplatform_pro_re_enrollment_settings` Create reads `GetReenrollmentSettingsV1` and passes it as the merge base to `buildReenrollmentInput` (`current`); on update the merge base is nil because USFU has already filled the plan. Note the per-type state-builder split: Optional+Computed **strings** reconcile with `ReconcileOptionalStringPointer` (handles the `""` blank), but for Optional+Computed **bools/ints** adopt the server value directly (e.g. `BoolPointerValueOrNull`) — the bool/int `Reconcile*` helpers return null when prior state is unset, which would blank an omitted toggle instead of adopting the live value.

**A singleton carrying Optional-only fields needs the merge base on update as well.** The create-only rule above rests on `UseStateForUnknown` covering the update path, which it does only for `Optional+Computed`. An `Optional-only` field plans as **null** when the configuration omits it — no prior value is carried in — and null is a reset on a full-replace endpoint, so every apply that never mentions the field clears it. `sso_settings` is the reference: the enrollment SSO block and the SAML metadata file are Optional-only (there is no server default for either to absorb), so `Create` and `Update` both read `/v3/sso` and merge the plan over it (#405). Reading it on update is cheap and cannot change anything else, because the base is consulted only where the plan has no value: a declared field still wins and `""` still clears. Convert the field to `Optional+Computed` instead where it genuinely has a server default — that is what `group_enrollment_access_name` did in the same change.

**3. Shared backing store → read-merge-write + a process-shared mutex.** When two endpoints (or two resources) are two views of *one* record — a write to either propagates to the other's read — extra care is required:

- **Round-trip non-owned fields.** A resource that owns only a *subset* of a full-replace object must read the live `GET`, overlay its own fields, and pass the fields it does **not** own back **unchanged** (they are not in its schema or state). `jamfplatform_pro_user_initiated_enrollment_settings` round-trips the six re-enrollment fields it does not own (see `mergeEnrollmentSettingsInput`).
- **Serialize with a provider-shared mutex.** Terraform applies dependency-free resources concurrently (e.g. both created on first apply), so two resources doing read-merge-write against one store can clobber each other from a stale read. Add a by-reference `sync.Mutex` to `providerdata.Data` with an exported accessor (precedent: `EnrollmentWriteLock()`), grab it in each resource's `Configure` (type-assert `req.ProviderData.(*providerdata.Data)`, nil-safe), and lock it around the **entire** `GET → merge → PUT` critical section in `Create`/`Update`. `Read` is `GET`-only and takes no lock. Operations against a *different* endpoint (e.g. the access-group sub-collection on `/v3`) run outside the lock.
- **Document the residual cross-process gap.** A process-shared mutex only serializes within one provider process; two separate `terraform apply` runs against the same tenant still race when the object carries no version/ETag for optimistic concurrency. State this limit in the resource doc-comment.

**Write-only secrets are the exception to full-replace.** A write-only keystore/identity object (e.g. a signing certificate) may be **preserve-on-omit**: omitting the object keeps the stored secret, while a sibling toggle (not the object) is what deletes it. Wire-probe the lifecycle (set / keep-on-omit / delete) explicitly. In the merge, set such pointers to `nil` so `omitempty` omits them (preserves the secret) — **never** echo the server's `null` back, and only build the object when the user is uploading. See `applyCertToBody` in `user_initiated_enrollment_settings`. This mirrors the WriteOnly + `_wo_version` rotation pattern in [§Plaintext secrets](#plaintext-secrets--writeonly-with-_wo_version-rotation-companion).

**Individually-optional nullable fields inside a full-replace block.** Some full-replace nested blocks accept each field as independently optional **and nullable**: the server echoes `null` for an unset field (never a zero value) and **rejects** a blank string or non-positive number on any field that *is* present. For these (e.g. `jamfplatform_pro_app_installer.notification_settings`), model each field **Optional-only** (not `Optional+Computed` — there is no server default to absorb), **omit** unset fields from the request (send `nil`, never `""`/`0`), and preserve `null` in the flatten so state matches config. Add present-only validators (`LengthAtLeast(1)`, `AtLeast(1)`) so the server's constraint surfaces at plan time. Field-level full-replace still holds — dropping a previously-set field clears it to `null`. Reserve `Optional+Computed` for the sibling fields the server genuinely defaults (a boolean defaulting to `false`); a single block can mix the two principled-ly. **Don't zero-fill** "to send a complete body" — that's the trap that turns an unset field into a `400`.

### Merge-patch (`PATCH` → `204`) settings updates

Some pro settings endpoints update via **JSON merge-patch** (`Content-Type: application/merge-patch+json`) and return **`204 No Content`** — the opposite of the full-replace `PUT`-echoes-body shape above. Wire-probe to confirm (flip one field, omit the rest, `GET` again):

- **Omitting a field preserves its server value** (merge-patch semantics) — so `Optional+Computed` is correct here, and the *opposite* of the full-replace bias toward `Required`. Build the payload by sending **only** the fields the user set: map each `Optional+Computed` attribute to a pointer that is `nil` when the planned value is null/unknown, so `omitempty` drops it and the merge-patch leaves the server value untouched. (Reference: `optBool` in `computer_inventory_collection_settings/input_builders.go`.)
- **`204` means no echoed body**, so a `GET`-after-write is **mandatory** in both Create and Update to capture authoritative state (computed defaults, server coercions). This is the same GET-after the singleton convention already requires — but here it is load-bearing, not just future-proofing.
- Probe whether the server **coerces dependent fields** (a parent toggle that disables child sub-options): see [§Cross-field validation](#cross-field-validation) for handling — a config validator, not a plan modifier. Reference: `jamfplatform_pro_computer_inventory_collection_settings`.

#### Merge-patch merges *inside* a nested object, so a discriminated block must send explicit nulls

Merge-patch does not replace a nested object; it merges into it, field by field. When a
nested object is **discriminated** — one member selects which of its siblings are legal —
that makes a change of discriminator unexpressible by omission, and both directions fail
for opposite reasons. On `jamfplatform_security_cloud_ztna_app`'s `routing` block, whose
`mode` decides whether a gateway and a resolution mode are required or forbidden:

- switching to the mode that **forbids** the siblings, sending only the discriminator,
  leaves the previous values merged in — and the server refuses the combination;
- switching to the mode that **requires** them, sending only the discriminator and one
  sibling, arrives missing the other — and the server refuses that too.

Both draw the same opaque refusal (`400 [INVALID_FIELD] routing: Routing definition is not
valid.`), so the failure looks like a validation bug rather than a serialisation one.

Two consequences:

- **Send the whole discriminated object on every write, with absent members as explicit
  `null`s** — never a partial. This is the nested-object analogue of the always-emit rule
  for classic wrappers and scalars above.
- **A nil pointer with `,omitempty` cannot express `null`.** The SDK generator has a config
  knob for exactly this: list the schema in `emitNullForOptional` (`tools/generate/config.json`)
  and it drops `,omitempty` from that schema's pointer fields, so nil marshals as `null`.
  Fixing this upstream is required — a provider-side workaround is not available, because
  the generated `*PatchRequest` type is the method's parameter. Wire-probe that the create
  path also accepts the explicit-null form before doing it; on this endpoint it does.

Reference: `internal/resources/security_cloud/ztna_app/input_builders.go` (`routingToWire`),
and `TestRoutingToWireEmitsExplicitNulls`, which asserts the marshalled JSON rather than the
struct — the struct is identical either way and only the tag decides which body goes out.

### Classic XML `PUT` merge where *empty clears* (omit = retain)

A third write shape, distinct from both full-replace (omit = wipe) and merge-patch (omit = preserve): some classic XML `PUT` endpoints **merge**, but an **empty element clears** the field. Wire-probe both halves (`PUT` with a block/field omitted → `GET` (retained?); `PUT` with an empty `<field></field>` → `GET` (cleared?)). On `jamfplatform_pro_mobile_device_enrollment_profile`: omitting a field or whole block retains the server value; an empty element clears it; `PUT` returns `201` with no body (GET-after mandatory).

- **Always-emit managed scalar strings (empty when null).** For plain `Optional` strings, map null → a pointer to `""` (not `nil`) so a removed value is explicitly cleared and config↔state stays aligned. Omitting (sending `nil`) would leave the server value, producing a perpetual diff after the user deletes the attribute — and, because Read echoes the retained value into state, a failed apply ("Provider produced inconsistent result after apply") rather than a merely stale one (issue #384, six resources). The shared helpers are `helpers.AlwaysEmitStringPointer` and `helpers.AlwaysEmitBoolPointer` (null → `""` / `false`, unknown → `nil`), paired with `helpers.ReconcileOptional*Pointer` in the state builder. References: `printer/`, `network_segment/`, `directory_binding/`; `clearable()` in `mobile_device_enrollment_profile/input_builders.go` is the same shape, package-local.
- **Optional+Computed fields omit when null/unknown.** Fields with a genuine server default (booleans, a server-defaulted int, a `site_id` that defaults to `-1`) stay `Optional+Computed` and are *omitted* (`nil`) — the Computed-ness absorbs the server value, so "can't unset" is acceptable and avoids the empty-clear churn.
- **Clear by omission, not `= ""`.** The user-facing contract is: omit an attribute to clear it (null → emitted empty → server clears → `GET` null → state null == config null ✓). An explicit `= ""` is a *known* plan value the server drops to null, yielding a `plan("")`-vs-`state(null)` inconsistency — benign (HCL users omit, they don't assign `""`), but note it on the helper and write acceptance "clear" steps as omission, never `room = ""`.
- **Optional nested blocks the server always returns:** refresh a block only when it is already authored (model pointer non-nil) so the always-returned server defaults don't fabricate a block the user never declared; consequently the block is not populated on import → `ImportStateVerifyIgnore` it (same precedent as policy sub-blocks).

**Bearer-auth-only gaps — some classic sub-resources can't be written via this provider.** The Jamf Pro `/fileuploads/{resource}/...` attachment-upload endpoint **rejects OAuth bearer auth for some resources** while accepting it for others — `enrollmentprofiles` returns `403` via the platform gateway *and* `401` "requires user authentication" direct, while `mobiledevices` uploads succeed (`204`) with the same token. This provider authenticates only via OAuth bearer, so such sub-resources are **read-only** regardless of any SDK fix. Wire-probe the upload (it's the canonical signature) before promising a writable attachment/upload field; if bearer is refused, model the sub-resource read-only (`Computed` list) and document it. Reference: `mobile_device_enrollment_profile` attachments.

### Server-managed child collections via create/delete-only endpoints

Some settings objects carry a **child collection** whose members are managed by **dedicated create/delete endpoints** with **no update endpoint** and which the parent settings write cannot mutate (the inventory-collection custom application paths: the merge-patch settings body rejects new entries — "Id field is required" — because the server mints the id on create). Model these as a flat **`Set[String]`** of the user-meaningful field, not a `SetNestedAttribute`:

- **Diff by value, not by id.** A newly declared member has no id yet (the server assigns it on create), so the diff cannot key on id. Reconcile in Create/Update: value in plan ∉ server → POST (create); value on server ∉ plan → DELETE by id (resolve the id from a `GET` at apply-time). Keep the server id **out of Terraform state** entirely — it is only needed for the delete call. A changed member is naturally remove-old + add-new.
- This sidesteps both nested-collection traps at once — `Set` + `Computed` id instability, and the `Optional+Computed`-inside-nested plan-modifier rule — because the only user field (`path`) has no computed sibling and no nesting.
- **Filter server-managed built-ins.** Built-in members the server always returns (the built-in application paths carry the sentinel id `-1`) must be filtered out of state on Read, or they perma-diff against the user's declared set.
- **Probe value normalization.** If the server canonicalises the value (trailing slash, case), a string-match diff churns forever — wire-probe and either canonicalise or suppress. (Inventory-collection paths are stored verbatim, so no suppression is needed.)
- **Honour endpoint scope limits.** If the create endpoint's scope enum accepts only a subset (the inventory-collection custom-path scope accepts only `APP`, so the UI's Fonts/Plug-ins custom paths are unreachable via this API), model only what the API supports and **document the coverage gap** in the schema `MarkdownDescription`. Reference: `application_search_paths` in `jamfplatform_pro_computer_inventory_collection_settings`.

### Classic sub-collections — omit / empty / present clear semantics

A classic `PUT` **merges at the sub-element level**: a repeated wrapper like `<criteria>` or `<display_fields>` behaves three ways, wire-probed on `/advancedcomputersearches` and `/advancedusersearches`:

- **wrapper omitted** → server **keeps** the existing collection (classic top-level merge; note this is the opposite of category elements *inside* `<scope>`, whose omission clears once the subtree replace triggers — see §Granular per-category ownership).
- **wrapper present but empty** (`<criteria></criteria>`) → server **clears** the collection to zero.
- **wrapper present with members** → server **replaces** the whole collection (not merge-by-key).

So a resource that wants the Terraform config to be **authoritative** over a managed collection — including the ability to remove every member — must **always emit the wrapper** (empty when the user clears it), never rely on `omitempty`/nil. Build a non-nil-but-possibly-empty wrapper unconditionally; flatten an empty/absent server wrapper back to **null** (not `[]`) so an omitted block round-trips (`null` config → `null` state). Consequence for users: **omit the block to clear it; do not write `criteria = []`** — a known-empty list mismatches the null flatten and trips "produced inconsistent result after apply". Document this in the schema `MarkdownDescription` and exercise both the shrink (N→1) and clear (omit) transitions in acceptance. Reference: `internal/resources/pro/advanced_computer_search/` (`buildCriteriaWrapper`/`buildDisplayFieldsWrapper` always-emit; `criteria.BuildCriterionSlice` returns a non-nil empty slice).

**Alternatively, expose the wire-natural three-way directly (opt-out)** when leaving a collection *unmanaged* is a useful option and its data is costly to recreate (licence serials, purchase orders): emit the wrapper **only when the attribute is non-null** — `null`/omitted → not sent → server **retains** (the user opts out of managing the collection); `[]` → empty wrapper → **clears**; `[items]` → **replaces**. This relies on the framework decoding a null list to a nil Go slice and `[]` to a non-nil zero-length slice (verified), so the build's nil check separates "don't manage" from "clear". On read, only refresh a list whose incoming model is non-nil (managed) — leave an unmanaged list null and ignore the server echo, exactly like scope's granular per-category gating (`scope.RefreshManagedSet`) — and flatten an empty *managed* collection back to `[]` (not null) so a managed `[]` round-trips. Trade-off: `terraform import` leaves these lists unmanaged (the importer has no prior model), so `ImportStateVerify` must ignore them and users re-declare to take ownership. Reference: `internal/resources/pro/licensed_software/` (`software_definitions` + `licenses`; `buildLicensedSoftwareInput` conditional-emit, `assignLicensedSoftwareResourceModel` managed-gating).

**The same merge semantics apply to top-level *scalar* fields, not just repeated wrappers.** A classic `PUT` that omits an optional scalar element (e.g. `<description/>`) **keeps** the stored value — so a config that drops the field would never clear it, surfacing as "produced inconsistent result after apply" (plan `null`, state retains the old value). For a clearable optional scalar on a classic resource, **always emit the element** (empty when the user omits it — `<description></description>` clears it server-side, wire-probed) rather than using `omitempty`/nil; the state builder then reconciles the echoed `""` back to `null` so an omitted field round-trips. This is the scalar analogue of the always-emit-wrapper rule above, and it is **classic-only** — Pro JSON full-replace clears omitted scalars for free. Note the asymmetry *within* a classic object: top-level scalars merge, but a provided nested element (e.g. `<input_type>`) **replaces** its whole subtree. Reference: `internal/resources/pro/user_extension_attribute/` (`description` always-emitted; `input_type` subtree replaced). Two caveats, both wire-probed 2026-09-06: **the clear token is per field, not per endpoint** — an integer reference clears with its own sentinel (`account_group.ldap_server_id` clears with `<ldap_server><id>-1</id></ldap_server>`; `0` is refused `409 Problem with LDAP Server ID` and an empty `<ldap_server/>` retains), so probe the token before assuming `""`/`0`; and **a field the server requires under the current mode refuses the empty element** (`webhook.header` under `HEADER` → `409 INVALID_REQUIRED`, `webhook.username` under `BASIC` → `409 Username is required`), so emit the empty element only under the modes that do not require the field and add a plan-time validator for the modes that do. Every clear must be asserted on the wire in acceptance (`testhelpers.CheckLiveObject`), because a null in state cannot distinguish "cleared" from "retained and reconciled away".

### Server-*expanding* collections — intersect-on-read, not omit/empty/present

The omit/empty/present model above assumes the server stores **exactly** what you send. Some endpoints don't: they **silently expand** a submitted collection (adding dependency entries you never sent, sometimes into a *different* sub-collection) **and silently drop** unrecognised entries (returning `2xx`, no error). The Jamf Pro admin-account privilege grid is the canonical case (wire-probed): submitting `jss_objects=["Update Buildings"]` stores `jss_objects=["Update Buildings"]` **plus** `jss_settings=["Read Activation Code"]`, and an invalid string just vanishes. Naïvely storing the server echo permadiffs (config `{X}` vs state `{X, +server-added}`) and a typo permadiffs forever.

The reconciliation that works (wire-proven across all four cases):

- **Intersect-on-read.** On refresh-`Read`, for each sub-collection the config *manages* (non-null in prior state), store `declared ∩ server` — the user's declared members that the server still has. Server-added extras are dropped (never enter state ⇒ no diff); a genuine removal sticks (it's simply absent from `declared`); drift where the server loses a declared member shrinks the intersection ⇒ the next plan re-adds it. Sub-collections **null in prior state stay null** (unmanaged); import (no prior state) materialises the full server grid.
- **Trust the plan value on write — do NOT GET-after-write-and-intersect this collection.** If the server drops a value (typo / not-grantable), a GET-after-write intersect makes applied-state ≠ planned-config ⇒ a **hard "provider produced inconsistent result after apply"** abort. Writing the planned value to state (and reconciling only on the *next* refresh) degrades that to a soft diff instead. (Base/scalar fields can still GET-after-write as normal; only the expanding collection is plan-trusted.)
- **Validate at plan time**, because intersect-on-read turns a dropped typo into a *soft* diff but cannot prevent it. Discover the valid vocabulary from a live source and reject unknown entries in `ModifyPlan` (warn loudly, don't silently skip, if discovery fails). See §"plan-time validation against a live list".
- **Do NOT use a `state ⊇ config` semantic-equality plan modifier** to suppress the expansion diff. It is fatal: after *any* removal the new config is a proper subset of state, so `config ⊆ state` stays true and the modifier retains the removed member — it suppresses **every** removal, not just server-added ones. Intersect-on-read has no such flaw.

Model the collection as **plain `Optional`** (not `Optional+Computed`) — there is nothing for the framework to compute; the builder/state-builder own reconciliation. Reference: `internal/common/accountprivileges/` (`IntersectIntoState`, `Discover`/`Validate`) consumed by `internal/resources/pro/{account,account_group}/`.

The classic `<criterion>` element itself (smart groups + advanced searches) is shared: `criteria.CriterionAttributes(operators)` (schema), `criteria.CriterionModel` + `criteria.BuildCriterionSlice`/`FlattenCriterionSlice` (model + build/flatten). Pass `criteria.Operators` for the full set or `criteria.Without(...)` for the user-attribute subset. Note `device_group` is **not** a consumer — it is a Platform Services resource with divergent legacy attribute names (`order`/`criteria`/`operator`). Model an **ordered** classic collection (priority/parens load-bearing — e.g. `criteria`) as a `List`; model an unordered one the server re-sorts (e.g. `display_fields` columns) as a `Set`, so its order-independent comparison absorbs the server reorder rather than perma-diffing.

### Referenced-by-name dependencies & `HAS_DEPENDENCIES` on delete

Some Jamf Pro objects refuse deletion while another object references them, returning `406 HAS_DEPENDENCIES`. The pattern was wire-derived from the API role / API client pair (deleting a role still assigned to a client), whose constructs were withdrawn at the Platform API GA — no shipped construct currently has this shape, but the rules below hold for the next one that does. When a resource references another **by name** (rather than by a `Computed` id), be aware of two consequences:

- **Don't add a plan-time validator that checks the referenced name against a live server list.** A name interpolated from another resource created in the *same* apply is a *known* value at plan time (a `Required` display name, say), so the validator can't skip it on unknown-grounds — yet the referenced object isn't on the server yet, so the lookup fails with a false "not found" plan error. This breaks the canonical compose pattern. Rely on Jamf Pro's clear apply-time error instead. (A live-list validator is still correct for values that are *not* produced by sibling resources — e.g. a fixed per-version privilege list.)
- **The delete-while-referenced 406 is a Terraform ordering gotcha, not a provider bug.** Removing the reference *and* destroying the referenced resource in one apply can let Terraform sequence the destroy before the dependent's update (the config dependency edge is gone in the new graph). Document the constraint in the referenced resource's `MarkdownDescription` ("remove it from the consumer first, in a separate apply"). In acceptance tests, exercise scope add/remove by keeping the referenced fixture **defined-but-unreferenced** when dropping it from the consumer, so no in-use object is destroyed mid-test.

### Referencing a server-managed catalog by name

When a resource references a **server-managed catalog** object — one the user never creates, e.g. a title catalog the server publishes — prefer exposing it **by name** with the ID `Computed`, rather than forcing the user to supply an opaque server ID:

- Expose `<thing>_name` as `Required`; keep `<thing>_id` `Computed` (plain `Computed`, no `UseStateForUnknown` — it is derived from the mutable name, so it must recompute when the name changes; see [§Server-derived computed fields](#server-derived-computed-fields--optionalcomputed-attributes)).
- **Resolve name → ID at apply** (Create/Update); a miss is a clear "not in catalog" error. Use the SDK's `Resolve…IDByName` where one exists; otherwise list the catalog through the service's own filter and decide the match in the provider (see `app_installer/name_lookup.go`).
- **Read/import must reverse-resolve ID → name** from the catalog (the object's `GET` returns only the ID). Do **not** echo the configured name back from state — on import there is no prior config, so an echo yields an empty name and fails `ImportStateVerify`. Make the reverse-resolve best-effort: preserve the existing state value on a transient catalog error rather than failing the refresh.
- **Safe only with an exact-match resolver, and a service-side filter is not one.** A case-insensitive/fuzzy match would accept `"jamf composer"`, store it, then reverse-resolve to the canonical `"Jamf Composer"` → perpetual diff. Confirm the resolver matches exactly: the platform `ResolveByNameClientPaged` does (`v != name`), but Jamf Pro's own RSQL `name==`/`titleName==` filter does **not** — it is case-insensitive and treats `*` as a glob, wire-verified on both the App Installer catalog and its deployments. So a filter narrows the request only; decide the match on byte equality in the provider, and treat two objects sharing one exact name as an ambiguity error rather than an arbitrary pick.
- **Plan-time name validation IS appropriate here** — unlike the sibling-resource case above. A server catalog is never created in the same apply, so a best-effort live-list/`GET` check (warn on transport error, error on a genuine miss, skip null/unknown) fails fast without breaking compose. Reference: `internal/resources/pro/app_request_settings/validators.go` (`validateAppStoreLocale`).

Reference: `jamfplatform_pro_app_installer.app_title_name` → Computed `app_title_id`, resolved by `internal/resources/pro/app_installer/name_lookup.go`.

### Classic membership: author by username, mirror the resolved ID set as Computed

Some classic objects expose the **same membership in two parallel collections** — a username list and an ID list (e.g. `jamfplatform_pro_class` returns both `<students>`/`<student>` usernames and `<student_ids>`/`<id>`). The server resolves **both directions**: write usernames and it fills the IDs; write IDs and it fills the usernames. Model the **UI-facing identity** (username) as the authored attribute and the sibling ID list as plain `Computed`:

- `students` / `teachers` — `Optional` `Set` of usernames (the admin-UI surface).
- `student_ids` / `teacher_ids` — plain `Computed` `Set` of resolved IDs (no `UseStateForUnknown`; they recompute when the username set changes — same rule as any [server-derived field](#server-derived-computed-fields--optionalcomputed-attributes)).
- Do **not** send the ID collections on write — author by username only; the server resolves them.

Two wire behaviours flip the usual rules — **probe them, don't assume**:

- **Unknown usernames are auto-created, not rejected.** A username that matches no existing user returns `201`, and the server *mints a new user record* for it (verified on `/classes`). So — unlike the [server-managed catalog](#referencing-a-server-managed-catalog-by-name) case — **do NOT add a plan-time name-existence validator**: there is nothing to validate against, and a "not found" check would be wrong. Referenced **group IDs**, by contrast, *are* validated (a bogus ID returns `409`) — rely on Jamf Pro's apply-time error for those.
- **The server canonicalises usernames** (e.g. `Kyle@X` → `kyle@x` when it matches an existing user). A username-keyed `Set` therefore drifts unless the state builder preserves the configured casing: reconcile case-insensitively — when a returned value matches a configured one ignoring case, keep the configured spelling; otherwise take the server value. This both prevents the post-apply "produced inconsistent result" error and suppresses perpetual diffs.

Membership is **authoritative**: emit every collection in full on every write so the config is the source of truth — an empty wrapper element clears it (see [§Classic sub-collections — omit / empty / present clear semantics](#classic-sub-collections--omit--empty--present-clear-semantics), or the `advanced_computer_search` reference). Preserve the prior null-vs-empty set shape when the server returns nothing.

Reference: `jamfplatform_pro_class` (`students`/`teachers` → Computed `student_ids`/`teacher_ids`).

### Delete semantics: not-found, async, and propagation-blocked

The SDK transport does **not** retry 4xx and does **not** treat `DELETE→404` as success — both are consumer concerns (an eventual-consistency retry needs context the transport lacks; see [§Pro error/retry helpers](#pro-errorretry-helpers)). So **every `Delete` must branch on `helpers.IsNotFoundError`** to treat an already-absent record as success — never assume the SDK swallows the 404.

Beyond not-found, **wire-probe the delete** during the in-design phase — the response status, whether removal is synchronous, and (for async deletes) whether *polling itself interferes*. Probe: create a throwaway object, issue a single `DELETE` (capture status + body via `jamf-cli ... delete <id> -vvv`), then `GET`-by-id over time. Four patterns, each a distinct handler:

| Wire behaviour | Handler | Reference |
|---|---|---|
| Clean synchronous (`200`/`204`, `GET`→404 immediately) | Plain delete + `IsNotFoundError` not-found-as-success branch | `mac_app_store_app` (`/macapplications`) |
| Accepted behind a **misleading** status (e.g. `400` with an `<id>` body), removal prompt and **not** GET-sensitive | Poll `GET`-until-not-found via `helpers.ConfirmAsyncDelete`; error only on timeout | `mobile_device_app` (`/mobiledeviceapplications`) |
| Accepted behind a misleading status, slow **and GET-sensitive** — polling to confirm *delays* the server-side removal | **Fire-and-trust**: issue `DELETE` once, **never GET**, `helpers.IsClientError`→`AddWarning` (drop from state), surface 5xx/transport as errors. Destroy-time verification is impossible → acc `CheckDestroy` is a documented no-op | `ebook` (`/ebooks`) |
| Blocked by a still-propagating dependency (`406`/`409` while a dependent resource's own deletion catches up — Platform Services) | **Re-issue** the `DELETE` on `406`/`409` until it clears, the object is gone, or the timeout fires (`jamfplatform.PollUntil`). Re-issuing is *correct* here, unlike the accepted-async case | `device_group` |

The two misleading-status patterns look identical on the wire (same `400`) but diverge on GET-sensitivity — **probe it, do not assume** (`/ebooks` and `/mobiledeviceapplications` return the same misleading `400` yet behave oppositely under polling). Likewise never generalise one classic delete's behaviour to a sibling: `/macapplications` is clean-sync while the other two are misleading-async.

### Pro error/retry helpers

Eventual-consistency and retry are **consumer concerns**, not transport concerns — the SDK client cannot tell a permanent `409` ("Problem with site ID") from a transient one, so it does **not** retry 4xx. The transport retries only **`429`/`Retry-After`** (an unambiguous, server-instructed throttle) and applies a configurable **inter-request delay** (`WithMinRequestInterval`) that paces all outbound calls. The SDK's own default is 100ms, applied only when the option is not passed; the provider always passes its own value, which defaults to **0** — no pacing — a deliberate provider-wide throughput decision, since the interval gates request *starts* and so at 100ms caps the whole provider at 10 requests per second however many operations Terraform runs in parallel. Operators put pacing back with the provider `min_request_interval_ms` attribute / `JAMFPLATFORM_MIN_REQUEST_INTERVAL_MS` env var (attribute wins); features that fan out reads bound their own concurrency instead. Everything else surfaces to the resource, which owns any eventual-consistency handling (poll, re-issue, or fire-and-trust — see [§Delete semantics](#delete-semantics-not-found-async-and-propagation-blocked) and the per-resource `crud.go` annotation blocks).

`internal/common/helpers/helpers.go` provides the classifiers:

- `IsNotFoundError(err)` — `404` (and the classic `400 INVALID_ID`); use in every `Read`/`Delete`.
- `IsClientError(err)` — any `4xx`; distinguishes an accepted-but-misleading `4xx` (treat as success-with-warning on a fire-and-trust delete) from a `5xx`/transport failure that must surface.
- `IsServerError(err)` — `5xx`.
- `IsGatewayUnrouted(err)` — the **gateway's** own `404 page not found`, meaning the request reached no Jamf service.
- `IsEdgeBlocked(err)` — an HTML error page from a CDN, WAF or IP allowlist, meaning the same thing one layer further out.

**Both of the last two must be excluded anywhere a status decides whether to clear
state.** Neither reply came from a Jamf service, so reading either as "the object is gone"
deletes a live object from state — silently, and across the whole estate, since a refresh
then empties the state file and `terraform plan` reports a clean set of creates with exit
status `0`. `IsNotFoundError` already excludes both. A resource classifying a status itself
must do so explicitly: `ebook`'s `isAcceptedAsyncDelete` is the worked example. The trap is
that an edge page carries whatever status the edge chose and **CloudFront chooses `404`
among them**, so it is not caught by excluding the gateway's plain-text reply.

`IsEdgeBlocked` is a thin `errors.Is(err, jamfplatform.ErrUnexpectedResponse)` check, so the
classification stays the SDK's — which is what makes it safe. SDK v1.0.0 marks an edge page
and deliberately exempts Jamf Pro's own HTML "Status page" template, so a classic `404`
keeps meaning "deleted". Never widen this to "an HTML `404` is never gone": every
`/proclassic` `Read` depends on the opposite.

Testing either one needs `internal/testhelpers/gatewaystub`, not a hand-built
`*APIResponseError`. The marker is attached inside the SDK's unexported transport, so no
exported constructor can produce a marked error and a fabricated one asserts only that the
provider agrees with itself.

**Render every API error through `helpers.APIErrorDetail(err)`, never `err.Error()`.**

```go
resp.Diagnostics.AddError("Error creating Jamf Pro department", helpers.APIErrorDetail(err))
```

It appends what to do about a response that came from a CDN, firewall or allowlist instead
of Jamf, splitting a standing block (reports this host's public IP address, looked up through
`internal/common/egressip` and cached once per process) from a gateway failure (already
retried, run it again). For every error a Jamf service produced it returns `err.Error()`
unchanged, so it is applied at **every** call site rather than the ones judged likely to see
a block: which call an edge page lands on is a property of the network at that moment, not of
the resource. The two CloudFront failures that prompted it landed on *creates*, the
operations an obvious "reads and deletes only" narrowing would have skipped.
`internal/conformance/api_error_detail_test.go` walks the AST and fails on an error rendered
into the detail of `AddError`, `AddAttributeError`, `diag.NewErrorDiagnostic` or
`diag.NewAttributeErrorDiagnostic` — an `.Error()` call on any receiver, or an error-shaped
identifier handed to `fmt.Sprintf`, through any amount of concatenation — so a new construct
cannot miss it. The guard's first version matched one shape, a bare `err.Error()` standing
alone, and passed while ~180 details still rendered raw, `category`'s own list resource and
classic `Delete` among them. Two things it deliberately does not enforce: a **warning**
diagnostic (`AddWarning`, `diag.NewWarningDiagnostic`), most of which render a decode or
plan-modifier failure rather than an API call, so those are converted by hand where they do
report one; and it recognises no per-site exemption, so a locally produced error in a detail
— a base64 or JSON decode, a version parse, a state re-encode — is wrapped like any other.
That costs nothing, since `APIErrorDetail` appends nothing to an error the SDK never marked.

For genuinely transient Pro states (`429`, `423 Locked` on in-flight async ops, `409` on stale `PATCH`/`PUT`), keep retry logic in-resource until **3 or more** resources need it, then extract a shared `RetryWithBackoff(ctx, op, isRetriable, maxAttempts)` (same deferred-abstraction discipline as shared schemas). `device_group`'s propagation-delete retry is the current in-resource precedent.

### Provider overall minimum Jamf Pro version (advisory warning)

Independent of per-resource `minJamfProVersion` constants (which are hard errors), the provider declares an **overall recommended minimum Jamf Pro version**: the highest version any shipped Pro resource requires. Surfaces as a **warning, not an error**, when the tenant version is below this floor:

```go
// internal/providerdata/providerdata.go
const ProviderMinJamfProVersion = "11.31.0"  // tracks jamfplatform.JamfProAPIVersion; bump with the SDK
```

Every Pro resource funnels its Configure through `providerdata.ConfigurePro`, which calls `pd.GetJamfProVersion(ctx)` unconditionally — regardless of whether the resource declares a per-resource `minJamfProVersion` const. The call is cached via `sync.Once` on `providerdata.Data`, so it fires at most once per `terraform` invocation. After the version is cached, the helper computes the floor warning and appends it to the Configure response.

When the fetch errors (e.g. 403 on a non-Jamf-Pro tenant): the helper swallows the error silently for resources with empty `minJamfProVersion` (the SDK CRUD call will surface the real failure later); for resources with a non-empty `minJamfProVersion` the helper surfaces the fetch error as a Configure-time hard error ("Failed to read Jamf Pro tenant version …"). Always-fetch + selective-swallow is the shape — that is exactly what `ConfigurePro` does, so callers do not need to think about it.

The warning text:

> Jamf Pro tenant older than provider build target
>
> This provider release was built against the Jamf Pro API as of version `A.B.C`. The tenant reports `X.Y.Z`. Some Pro resources may rely on endpoints or fields that did not exist in the tenant's version and could fail at apply time. Upgrade Jamf Pro or pin an older provider release.

Rationale: per-resource `const minJamfProVersion` covers hard correctness for individual resources that need newer endpoints. The provider-level floor catches the broader case where the user's tenant lags the API spec the provider was generated from — many Pro resources may quietly rely on fields or endpoints that only exist in newer Jamf Pro versions without each declaring a per-resource gate. Warning is enough — the per-resource error remains the safety net for resources that have one.

**Release-time process**: before tagging a release, grep all `minJamfProVersion` constants under `internal/resources/pro/`, take the max, update `ProviderMinJamfProVersion` in `internal/providerdata/providerdata.go` if it has moved. The provider Schema description interpolates the const so `docs/index.md` updates automatically after `make generate`. The hard-coded version string in `README.md` (under "Supported Jamf products and tenant version targets") must be bumped by hand. When additional Jamf products are added in the future, each gets its own `Provider<Product>MinVersion` constant + row in the provider Schema table + row in the README table; the release-time process expands accordingly.

### Shared abstractions — when to extract

Two extraction triggers apply, one for schemas and one for code helpers. They differ because the failure modes differ.

**Schemas — 3-consumer rule.** Many Jamf Pro resources expose similar shapes (scope, site, category, criteria, self-service payload). **Do not extract these into shared schemas upfront** — superficially similar Jamf APIs often differ in field names, ID types (int vs UUID string), and null semantics. Premature abstraction produces helpers with per-resource branching that is harder to read than the original duplication. Refactor trigger: **3 or more** shipped resources with verified-identical shape (same fields, types, null semantics — checked against the SDK structs, not eyeballed). Extract under `internal/common/schemas/`. Reference precedent: `internal/common/scope/` (scope sub-blocks extracted at sub-block granularity; the top-level scope shape stayed per-resource because it diverged across the eight scope-bearing resources).

**Code helpers — 2-consumer rule when logic is non-trivial.** Parsers, normalisation/diff-suppression functions, identifier injectors, and similar non-trivial code with no per-resource branching extract at the **second** consumer, not the third. Two copies of a 200-LOC plist mask or a `WriteOnly` rotation comparator means two places to chase the next server-side mutation; a duplicated bug now lives twice. The schema 3-consumer rule was written for cases where premature abstraction produces ugly branching — that risk does not apply to code helpers that compose without branching.

Canonical example: when `jamfplatform_pro_mobile_device_configuration_profile` landed as the second consumer of the mobileconfig mask / compare / identifier-injection helpers, they were lifted out of `macos_configuration_profile` into the shared `internal/common/payloadhelpers/` package (`MaskPayload`, `PayloadsSemanticallyEqual`, `ThreeWayCompare`, `InjectTopLevelIdentifiers`) at the second consumer — not duplicated, not deferred to a third. The `pro/` tree is flat, so any code shared across two or more Pro packages lives under `internal/common/<topic>/` (`payloadhelpers`, `availabletitles`, `invitationcommon`, …) — never in one resource package imported by a sibling.

Trigger summary:

| Kind | Trigger | Destination |
|---|---|---|
| Schema (shape, field set) | 3+ verified-identical SDK shapes | `internal/common/schemas/` (or domain-specific package like `internal/common/scope/`) |
| Code helper (non-trivial, no per-resource branching) | 2 consumers | `internal/common/<topic>/` |
| Trivial code helper (1-line wrapper) | Stays in-resource; extract only on demonstrated need | `internal/common/helpers/` |

### Scope helper

The `<scope>` block of every Jamf Classic-API resource (policies, ebooks, mac applications, mobile device applications, OS X configuration profiles, mobile device configuration profiles, patch policies, restricted software) shares its sub-block target categories — buildings, departments, computers, computer_groups, mobile_devices, mobile_device_groups, network_segments, jss_users, jss_user_groups, ibeacons, and the directory-service name-only siblings. The 3-consumer rule fires at **sub-block granularity**, not at the top-level scope shape (which diverges across the eight scope-bearing resources). Shared helpers live under `internal/common/scope/`.

**Block layout — `targets` / `limitations` / `exclusions`.** Every `scope` block mirrors the Jamf Pro admin UI's three Scope tabs. The all-flags (`all_computers` / `all_mobile_devices` / `all_jss_users`) and every per-category target ID/name set live inside a `targets` `SingleNestedAttribute`; `limitations` and `exclusions` are its siblings. `targets` is **Optional-only** (never Optional+Computed — an Optional+Computed `SingleNestedAttribute` over a `*struct` model fails Unknown decode at apply), and every attribute inside the three sub-blocks is Optional-only too — the null/`[]` distinction carries the granular-ownership contract (§Granular per-category ownership below). Sub-block struct allocation still gates on `<target>.Targets != nil` — where `<target>` is the model being written to state (the mutated param, which is the new plan on Update), **not** a separate prior-state reference — and within an allocated sub-block each category refreshes independently via `scope.RefreshManagedSet`. See [§`SingleNestedAttribute` blocks](#singlenestedattribute-blocks-optional-only-when-the-model-uses-typed-pointer) for why gating on the prior state crashes the remove-on-update transition. In the model, target fields move into a dedicated `…ScopeTargetsModel` reached via `Targets *…ScopeTargetsModel`; the build side reads them through a `TargetsOrZero()` value accessor (returns all-null fields when the block is omitted) so input-builders stay nil-guard-free.

**Item shape — IDs-only `Set<String>`.** Sub-blocks collapse to a flat `schema.SetAttribute{ElementType: types.StringType, Optional: true}` carrying only the numeric Jamf Pro classic ID (or name string, for the directory-service categories). Server-augmented `<name>` and `<udid>` wire fields are discarded on read; only IDs round-trip through Terraform state. Authoring uses interpolation: `computer_ids = [for c in data.jamfplatform_pro_computers.example: c.id]`.

**A numeric Jamf Pro group ID does not identify a group on its own — always pair it with the estate.** Jamf Pro numbers computer groups and mobile device groups in independent sequences, so the same integer names two different groups: on the test tenant, `1` is both the `All Managed Clients` computer group and the `All Managed iPads` mobile device group. Any map, cache, or resolver keyed on the ID alone silently loses one of every colliding pair, and the survivor is then rejected as the wrong type — which reads as "group not found" for a group that plainly exists. Key on `{deviceType, id}` (see `internal/common/impact/cache.go`), or thread an estate discriminator through the lookup the way `internal/common/criteria`'s `GroupResolver` threads `ObjectType`. **Platform group UUIDs are unique tenant-wide**, so a UUID-keyed index needs no discriminator, and UUID → Jamf Pro ID resolution (`device_group`'s `resolveJamfProID`) is unambiguous in that direction. Unit tests will not catch a regression here unless the fixture deliberately contains a cross-estate ID collision — this was found only by running against a live tenant.

**Naming convention.**

| Sub-block kind | Suffix | Examples |
|---|---|---|
| ID-bearing | `_ids` | `computer_ids`, `computer_group_ids`, `building_ids`, `department_ids`, `jss_user_ids`, `jss_user_group_ids`, `network_segment_ids`, `ibeacon_ids`, `class_ids` |
| Directory-service name-only | `_names` | `directory_service_or_local_user_names`, `directory_service_user_group_names`, `limit_to_user_group_names` |

Limitations and exclusions share the same attribute names — the wire-shape divergence (limitations user is name-only on wire; exclusions user is id+name on wire) is collapsed at the TF layer because both sides write `<user><name>…</name></user>` and discarding the response-side `<id>` is consistent with Option B.

**Composition pattern.** Per-resource glue assembles the resource's `scope` schema by composing `scope.IDSetAttribute` / `scope.NameSetAttribute` calls. There is **no** single top-level `ScopeAttribute()` mega-factory *unified across all eight* scope-bearing classic resources (`Policy`, `Ebook`, `MacApplication`, `MobileDeviceApplication`, `OsXConfigurationProfile`, `MobileDeviceConfigurationProfile`, `PatchPolicy`, `RestrictedSoftware`) — they expose materially different top-level field sets, so a unified factory would either leak unsupported fields or devolve into per-resource branching.

**Platform-level composite factories.** Where several resources share the *same platform's* scope shape (targets + limitations + exclusions), that shape is extracted into a platform-specific composite: `scope.ComputerScopeAttributes(scope.ComputerScopeOptions{…})` (computer-side: `policy`, `os_x_configuration_profile`, `mac_app_store_app`) and `scope.MobileScopeAttributes(scope.MobileScopeOptions{…})` (mobile-side: `mobile_device_configuration_profile`, `mobile_device_app`). Each returns the full `<scope>` attribute map for one platform; the caller wraps it in its own `schema.SingleNestedAttribute` with a resource-specific description. Rules:

- **Per-resource deltas go through an `Options` struct, not a fork.** Today the only axis is `IncludeIbeacons bool` — endpoints that silently drop iBeacon limitations/exclusions (wire-probed: `macapplications`, `mobiledeviceapplications`) set it `false` so the attribute is absent rather than permadiffing.
- **The framework matches model fields to schema attributes exactly, so each `IncludeIbeacons` variant needs its own model struct.** Hence the paired `ComputerScopeModel` / `ComputerScopeModelNoIbeacons` (and `MobileScopeModel` / `MobileScopeModelNoIbeacons`): the `…NoIbeacons` variants drop `IbeaconIDs` from their limitations/exclusions sub-models. A resource references whichever matches its `IncludeIbeacons` value as `Scope *scope.<Variant>` in its `tfsdk` model.
- **Extraction trigger.** The 3-consumer rule still fires at sub-block granularity for the `IDSetAttribute`/`NameSetAttribute` primitives. The platform composite is a maintainer call — it may be extracted ahead of three consumers (the mobile composite was extracted at two: `mobile_device_configuration_profile` + `mobile_device_app`) when a same-platform sibling is imminent.

File shape mirrors `internal/common/scope/computer_{model,schema}.go` and `mobile_{model,schema}.go`.

**Cross-field validator — `scope.AllFlagConflictsWith`.** A value-discriminated `validator.Bool` for `all_computers` / `all_mobile_devices` / `all_jss_users` semantics: fires only when the bool is true, attaches one attribute error per populated conflicting Set. Off-the-shelf `boolvalidator.ConflictsWith` triggers on any value and cannot express the "only when true" rule. The validator uses `path.MatchRelative().AtParent().AtName("…")` against its conflicting siblings — because each flag and the sets it conflicts with both live inside `targets`, these relative paths are unaffected by the nesting. Resource-specific constraints (e.g. `RestrictedSoftware` rejects `limitations` entirely) stay in the resource package — they are not shared scope logic.

**All-flags — plain `Optional`, no modifiers, no defaults.** Every scope attribute — the category sets and the all-flags — is `Optional`-only. The null/`[]` (or null/`false`) distinction carries the ownership contract below, so nothing here may be `Computed`, carry a state-forwarding plan modifier, or a static default. `AllFlagConflictsWith` is retained unchanged.

**Granular per-category ownership (load-bearing invariant).** The user-facing contract for every scope category, in both directions:

- **Declared (including `[]`) ⇒ Terraform owns the category.** Members drift-revert on refresh; `[]` clears the category.
- **Omitted (null) ⇒ the category is left as configured outside Terraform.** It never enters state or plans, and updates preserve it.

The classic wire cannot express the second half directly. Wire law, probed 2026-07-08 on 11.29.1 across `macapplications`, `policies`, `osxconfigurationprofiles`, `restrictedsoftware`, `mobiledeviceconfigurationprofiles`, `mobiledeviceapplications`, `ebooks` (whose dual computer+mobile union is ONE replace unit), and `vppassignments` — all identical:

| PUT body | Server behaviour |
|---|---|
| no `<scope>` element | scope untouched (top-level classic merge) |
| `<scope></scope>` (no child elements) | ignored — untouched |
| `<scope>` with only all-flag(s) at their current value | ignored — untouched |
| `<scope>` with ≥1 category element, **even an empty one** | **entire subtree replaced** — every category not in the body, across targets/limitations/exclusions, is cleared |
| all-flag `true` | flag set; its conflicting target categories wiped; limitations/exclusions coexist and follow the body |

(`patchpolicies` is unprobed — same family assumed, probe before release; `vppinvitations` was deliberately not probed — a probe risks sending real invitation emails — and is treated as the same wire family as `vppassignments`.)

Consequences, all mandatory:

1. **Update is read-merge-write, scope-only.** When the plan declares a `scope` block, `Update` GETs the live object *before* building the PUT, flattens **only its scope** with the hydrate-all path (`includeUnmanaged=true` into a zero scope model), overlays the declared categories via the shape's `scope.Merge*` helper (local scope models implement an equivalent local merge), and builds the wire payload from a shallow-copied plan whose `Scope` is the merged model. The ORIGINAL plan is what goes to state. No other section of the GET response is ever echoed into the PUT — that would convert omitted non-scope sections from invisible to Terraform-asserted. `Create` never merges: undeclared categories are simply absent from the POST.
2. **Merged output emits the full explicit skeleton.** Every merged category is non-null, and `BuildIDSlice` / `BuildNameSlice` return a **non-nil empty slice** for `[]` (declared clear) versus `nil` for null/unknown (unmanaged, element omitted). An empty category therefore marshals as an explicit empty element (`<computer_groups></computer_groups>`) — the wire's only reliable clear gesture: a merged body with no category elements at all would hit the ignored rows above and a final clear would silently no-op. The old "never emit an empty parent" rule is inverted for scope.
3. **Read-side per-category gating.** Scope flatten functions refresh each category through `scope.RefreshManagedSet` / `scope.RefreshManagedBool`, gated on the field of the model being written (the plan on create/update, prior state on read — never a separate prior-state reference; see §`SingleNestedAttribute` blocks). Wire-flatten helpers must return `scope.EmptyStringSet()` — never a null set — for absent wire elements, or a managed category would be nulled and trip the post-apply consistency check. `includeUnmanaged=true` bypasses the gate for import hydration, `terraform query` config generation, and the Update merge base.
4. **All-flag precedence in the merge.** A merged all-flag that is `true` empties exactly the target categories its `AllFlagConflictsWith` validator names — never limitations/exclusions, which coexist with a true flag (wire-probed). Declared conflicts are still rejected at plan time by the validator; the precedence rule only affects preserved server values.
5. **No plan-time co-managed warning — deliberate.** An earlier iteration warned on every plan about undeclared categories with members configured outside Terraform. It was removed for consistency with the provider's other omit=preserve surfaces (none of which editorialise about UI-owned values) and because a warning that fires on 100% of plans for a deliberate co-management setup is noise, not signal — Terraform offers no per-resource way to mute it. The read-merge-write never destroys unmanaged categories, so the warning was pure visibility; the split-ownership acceptance tests carry the safety proof. Do not reintroduce without at least gating on plans that actually modify scope.
6. **Acceptance coverage.** Each scope-bearing resource carries a scope *split-ownership* test proving omit=preserve on a representative category: create with one declared category, add another category out-of-band via the acc client, apply an unrelated change, assert the out-of-band members survived server-side and never entered state; then declare `[]` and assert it clears. Reference: `TestAccResource_ProMacApp_ScopeSplitOwnership`. Import steps that `ImportStateVerify` against subset-scope configs add `"scope"` to `ImportStateVerifyIgnore` — import hydrates every category for visibility, while apply keeps declared-only state.

**`classes` will not be overwritten in one write — deliver it in two (ebooks only today).** On top of the replace rule above, Jamf Pro stores `<classes>` only while the **stored** category is empty. A populated-to-populated transition is refused and the category is cleared instead, whatever the body carries: re-sending the identical `<classes>` cleared it 5/5, and so did omitting it (wire-probed 2026-09-12 on 11.31.1, `/proclassic/ebooks`, and independent of the child's shape — `<id>`, `<name>` and both together behave the same). Combined with the wholesale replace, **no single body preserves a stored class across another scope change** — which is how `jamfplatform_pro_ebook` came to fail the post-apply consistency check *and* destroy the class on every second apply (issue #428). Two bodies do: the first carries the caller's whole desired scope with `<classes>` emptied, the second the same scope with the classes populated (verified 5/5). Rules:

- **Gate on the category, not the resource.** `classes` is an ebook-only category today, but the rule belongs to the element. Gate on "the merged desired scope carries class members" — a scope with none takes the single-request path, so the extra write happens only where it is the difference between keeping and losing the stored classes, and a resource that gains the category later needs no further change. Reference: `ebookScopeClassesCleared` in `internal/resources/pro/ebook/input_builders.go`.
- **The first request carries the change.** It is the second body with one field zeroed, so an apply interrupted between the two leaves exactly what a single request leaves today — the change applied, the classes empty, restored by the next apply. Never order it the other way.
- **`Update` only.** A new object's category starts empty, so `Create` stores classes first time. A fix applied only at create looks right in a basic acceptance test and still fails on the second apply.
- **Clearing needs one request.** A declared `[]` against a populated category is the one populated-to-X transition the server honours.
- **Regression coverage needs a live class.** `jamfplatform_pro_class` mints one; see `TestAccResource_ProEbook_ScopeClassesSurviveUpdate`. The failing shape is "create with a class plus another target, then change the *other* target" — a create-only test passes either way.

**Non-scope section omission (unchanged).** The classic API tolerates absent sections — a `POST`/`PUT` body need not include `<reboot>`, `<self_service>`, etc. when the caller does not manage them; a nil SDK pointer with `,omitempty` omits the element. TF block absent (null) → SDK field nil → wire omits; `all_*` booleans use `helpers.OptionalBoolPointer` so unset flags collapse to nil (`false` marshals as a real element, distinct from omission).

**Directory-service user-group preflight (mandatory for every scope-bearing resource).** The classic server matches each `directory_service_user_group_names` entry against the tenant's configured LDAP / cloud-IdP and rejects an unknown name at *apply* with an opaque `409 "Problem matching limitation user group"`. Surface that at *plan* instead:

- Validation helper: `scope.ValidateDirectoryServiceUserGroupNames(ctx, searcher, set, attrPath) diag.Diagnostics` (in `internal/common/scope/ldap_preflight.go`), backed by `ldapgroups.ResolveByName` (server-agnostic exact-name match — scope groups carry a name only, no server id). Unknown name → attribute **error**; search/transport failure or unconfigured LDAP → attribute **warning** (best-effort, never blocks a plan); null/unknown set or element → skipped.
- Wire it in **`ModifyPlan`, not a `ConfigValidator`** — the search needs the configured client, and `ConfigValidator`s run before `Configure`. The resource holds a `pro.Client` (the `ldapgroups.Searcher`) obtained by **also** calling `providerdata.ConfigurePro` in `Configure`, alongside its `ConfigureProClassic` CRUD client (`ConfigurePro` returns `(nil, nil)` pre-configure, leaving the preflight a no-op).
- Call it once per set — `scope.limitations` and `scope.exclusions` — at paths `path.Root("scope").AtName("limitations"|"exclusions").AtName("directory_service_user_group_names")`.
- Run it **first** in `ModifyPlan`, before any payload-diff early-return (those return on create, where the preflight must still fire). It runs on every non-destroy plan; guard `req.Plan.Raw.IsNull()` for destroy.
- Coverage is `directory_service_user_group_names` only — there is no `/users` analogue endpoint, so `directory_service_or_local_user_names` (local users, stored verbatim) is not validated.

Reference impls: `mac_app_store_app`, `policy`, `macos_configuration_profile`, `mobile_device_configuration_profile` (the last two add a `preflightScopeGroups` helper since their `ModifyPlan` already carries payload-diff logic).
