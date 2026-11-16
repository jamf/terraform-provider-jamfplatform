# Schema design

Attribute naming, enums, Sets vs Lists, secrets, `Optional+Computed`, nested blocks and cross-field validation. Part of the [style guide](../STYLE_GUIDE.md).

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

**The wire name cannot go in a comment beside the attribute**, despite the snippet above: [§Go Conventions](package-layout.md#go-conventions) forbids comments inside function bodies, and a schema is built inside one. Put the whole wire mapping in the package doc comment instead, as a table — `internal/resources/security_cloud/ztna_gateway/resource.go` is the reference. That reads better anyway: one place to scan when chasing a field name seen in an API response, rather than a dozen scattered one-liners.

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

**A server that re-sorts forces a Set — unless entries can repeat.** A Set cannot hold two equal elements, so a collection the server both re-sorts and lets repeat fails with `Duplicate Set Element` on read, and one bad object fails a whole `terraform query`. Probe for repeats as well as order: write the same entry twice, and two entries that share an id but differ in a secondary field, then `GET`. When repeats survive, model a `ListNestedAttribute` and align the read to the prior list (the plan on Create/Update, prior state on Read) with `helpers.AlignToPrior`, keyed on the server's sort key and nothing finer, so entries that tie on the key pair first-in, first-out in the order the server keeps them; use `helpers.AlignToPriorFunc` when the plan can hold an unknown key (an id interpolated from a resource created in the same apply). The written order then survives, a reorder across sort groups plans one update and then plans clean, and the probe has to show that update changes nothing stored. Never reach for a list-level plan modifier that plans the prior order when the multiset matches (that is a rule 1a collapse), or semantic equality (it never runs at plan time, and on Create the prior value is the plan, whose server-derived names are still Unknown). A collection the server stores in the order written is a plain positional List: do not align it, so a reorder made in the admin UI surfaces as drift. Reference: `jamfplatform_pro_policy` scripts (sorted by id, then priority) and packages (sorted by name), aligned; printers, dock items and directory bindings, positional; `jamfplatform_pro_inventory_preload_record` `extension_attributes`, positional (issue #568, wire-probed 2026-11-11).

**Never model ordered data as a Set.** Smart-group `criteria` is the cautionary case: the `order` field, parentheses, and `and_or` joins are all positional, yet it was first modelled as a `SetNestedAttribute`. Two failures resulted: (1) Terraform delivers Set elements to the provider in arbitrary hash order, so an index-derived `order` came out scrambled; (2) Set elements correlate plan↔state by **value**, so a positional reconcile (`prev = current[i]`) paired each server element with an unrelated hash-ordered prior-state element and emitted values matching no planned element — surfacing as `planned set element … does not correlate with any element in actual`. The fix is a `ListNestedAttribute` (the data source already modelled it as a list): lists correlate positionally so `prev[i]` lines up with `server[i]`, and the null-preserving `Reconcile*` helpers keep user-omitted fields null. See `internal/resources/device_group/`. No state rewrite is needed for a Set→List swap — both serialise to a JSON array in state — but an in-place upgrade yields a one-time, self-healing reorder plan. Bump the schema `Version` anyway, with a pass-through upgrader for the previous version, when the List can hold values the Set could not (repeats): the bump is what makes a provider still on the old version refuse the newer state instead of failing to decode it. Reference: `jamfplatform_pro_policy` v2 → v3, `jamfplatform_pro_inventory_preload_record` v0 → v1.

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

**A value Jamf Pro derives from a sibling this resource sets is Computed-only, carried by `planmodifiers.MirrorOfString`.** The Self Service icon is the case: `self_service_icon.uri` and `.filename` are looked up from `id`, and wire-probed on the classic PUT (issue #468) the server ignores any authored pair and refuses a filename sent without an id. So neither is `Optional` — an authored value can only match what apply returns by accident, and one that differs fails every apply. Neither `UseStateForUnknown` nor `UseNonNullStateForUnknown` is safe either: both plan the previous source's value into an apply that moves the source, and Terraform rejects the result with "Provider produced inconsistent result after apply". With no modifier the attribute is Unknown in every plan that changes anything else. `MirrorOfString(NestedStringSource(block, "id"))` carries the prior value while the source is unchanged in config and goes Unknown when it moves, or is Unknown itself because the source resource is being replaced in the same apply. Input builders send the source only. Reference: `self_service_icon` in `policy`, `mac_app_store_app` and `mobile_device_app`; the shared unit assertion is `internal/testhelpers/iconmirror`, so a new resource carrying that block adds one line to its schema test rather than a modifier test of its own.

**Clear sentinels are per-field, not universal — probe each one.** The `-1` "none" sentinel above is a *convention*, not a guarantee. Wire-probe the actual clear value for every nullable reference field, because it can differ **between fields on the same endpoint**. The classic `/patchsoftwaretitles` endpoint showed it: `<site>` cleared with `id=-1` (the norm) but `<category>` cleared with `id=0`, and `-1` was a silent no-op. The endpoint that replaced it, `/v3/patch-software-title-configurations` (merge-patch), answers `-1` for both and refuses `0` and `""` with 400 `INVALID_ID`, and classic `<category>`, `<site>` and the other classic references each differ again (wire-probed for issue #573: an empty `<site><id></id></site>` and a bare `<site/>` retain the old site on every classic endpoint, a bare `<category/>` clears a policy's category but `<category><id></id></category>` draws a 409, and `0` is a 409 on all of them). When the wire sentinel diverges from the user-facing one, **honor the user-facing contract by translating at the wire layer** rather than leaking the quirk into the schema/docs/tests: map the user's sentinel to the wire clear value in the input builder, and collapse every server "none" echo back to the single user-facing sentinel in the state builder. Where the wire has one clear value and it needs no translation, refuse every other spelling at plan time with the shared `commonvalidators.ClearableReferenceID` (a positive ID or `-1`) instead of letting `""` or `0` reach the API, where it is dropped (inconsistent result after apply), stored literally or refused with a 4xx. Variants for the fields that differ: `ClearableReferenceIDWith` adds sentinels (`-2` for the cloud distribution point), `ClearableReferenceIDInt64` is the Int64 twin, and `PositiveReferenceID` / `PositiveReferenceIDInt64` accept no sentinel at all, for a reference Jamf Pro cannot clear (a Self Service icon: replace only) or that clears only by removing the attribute (`webhook.smart_group_id`). A reference whose PUT ignores the value on update (`volume_purchasing_notification.site_id`) gets `RequiresReplace`. Reference for translation: `internal/resources/pro/patch_software_title/` (`refIDPtr` maps any non-positive id to wire `-1`; `refIDValue` collapses the `-1`/`0` echoes to `"-1"`); for plan-time refusal, `internal/resources/pro/ebook/`; for a state-side collapse, `user_initiated_enrollment_settings` (`siteIDValue` turns the `"0"` Jamf Pro reports for no site into `"-1"`).

**A classic clear sentinel can silently corrupt the shared v2 representation.** Classic and modern (v2) endpoints for the same entity write to one underlying record, but validate differently. On patch software titles the classic `<category><id>0</id>` clear leaves the v2 `categoryId` as the literal `"0"` — which `/v2/patch-software-title-configurations` then **rejects on every write** (`id field must be string of positive numeric value or -1`). So a hybrid resource that does classic CRUD plus a v2 side-call (e.g. extension-attribute accept) will see the v2 call 400 whenever the title has no category — even though the v2 body never mentions `categoryId`, because the server re-validates the whole stored object. Fix: **re-assert the user-facing value (always v2-valid) in the same v2 write**, repairing the divergence. Historical for `patch_software_title`, which now writes only through v3 and no longer sends a classic clear: a title last written through classic can still carry `"0"`, which is why `acceptPendingExtensionAttributes` re-sends `categoryId` alongside the EA merge-patch and `refIDPtr` normalises every non-positive id to `-1`.

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

**1a. Nested-list elements and optional blocks: use `UseNonNullStateForUnknown`, unless the resource can collapse a plan.** `Optional+Computed` scalars inside a `ListNestedAttribute` or `SetNestedAttribute`, and inside an `Optional` `SingleNestedAttribute`, MUST use `stringplanmodifier.UseNonNullStateForUnknown()` (and bool/int siblings) rather than `UseStateForUnknown`. `UseStateForUnknown` copies the prior `StateValue` into the plan — including `Null` — and for an appended list element the prior state at the new index is `Null`. When the server then returns a value for that field on the new element, the framework consistency check trips with `Provider produced inconsistent result after apply`. If a `Sensitive` sibling (e.g. a `WriteOnly` password) lives on the same nested element, the error path is redacted up to the nearest non-sensitive ancestor, masking the real attribute and producing the misleading `.<parent>: inconsistent values for sensitive attribute`. `UseNonNullStateForUnknown` skips the copy when prior state is `Null`, leaving the plan `Unknown` so any post-apply value is accepted. The same null prior appears when an `Optional` single-nested block goes from omitted to declared: every leaf in it is `Null` in state, so `UseStateForUnknown` plans `null` and the apply fails on whichever leaf the server defaults (`policy` `self_service.notification_location` and `packages.distribution_point`, `mobile_device_app` `self_service.feature_on_main_page` and `notification_enabled`, all reproduced live). `UseNonNullStateForUnknown` has one cost. The framework marks every Computed attribute whose configuration is null Unknown, before any modifier runs, in any plan whose proposed state differs from prior state, and the non-null variant leaves a null prior Unknown. That is harmless while the plan really changes something: the leaf shows "(known after apply)" and the apply settles it back to null. It becomes a permanent diff, with `plan -refresh=false` never empty, only when **both** of these hold:

- **(a) A null-forever leaf**: the leaf stays null in state while its block exists, because Jamf Pro never returns it or the flattener keeps a null (`helpers.ReconcileOptionalString*` turns an empty wire string into null). Most resources have these. A `policy` with six blocks declared had eleven in them after apply (`restart_options.specify_startup`, `self_service.notification_subject`, five `files_and_processes` strings; wire-verified 2026-10-14).
- **(b) A collapse mechanism**: something that restores the change that made the plan differ, so the Unknowns are all that is left. Terraform core accepts a planned value that differs from a non-null configuration only when it equals the prior value, and the framework marks only attributes whose configuration is null, so there are exactly two: a plan modifier on an `Optional` or `Required` attribute that plans the prior value in place of the configured one (the CRLF/LF `preferences` equality on `jamfplatform_pro_mobile_device_app`, which left `general.external_url` and `itunes_store_url` Unknown, `TestAccResource_ProMobileApp_AppConfiguration`), or a `ModifyPlan` that writes the plan. A modifier that acts only on an Unknown plan value cannot be one: the stock carry modifiers, `UseStateForUnknownInExistingBlock*` and the id-keyed `UseKeyedStateForUnknown*` all return early on a configured value. Semantic equality cannot be one either, because it never runs at plan time.

So: when the resource has no collapse mechanism, use `UseNonNullStateForUnknown` for both list elements and singleton-block leaves, and add `nullprior.AssertNoPlanCollapse(t, schema, ".")` to the resource's schema test. It fails on any plan modifier on a configurable attribute that is not proven to act only on Unknown, and on any write to a `Plan` in the package or the module packages it imports. Only when the resource has one, switch its singleton-block leaves to `planmodifiers.UseStateForUnknownInExistingBlockString` / `Bool` / `Int64`, which carry a null prior while the block already exists and leave it Unknown only when the block is new. That is not free: it plans the carried null where the non-null variant planned Unknown, so a leaf Jamf Pro starts returning when a sibling changes (`mobile_device_app` `after_install_button_text`, gated on `make_available_after_install`) fails the apply. Wire-probe every sibling-gated leaf before converting. It is also not enough on its own: a plain `Computed` leaf (`category_name`, `site_name`) goes Unknown the same way and needs `planmodifiers.MirrorOfString`, and a leaf of a Set or List element has no block to look up and stays exposed. Every resource also keeps `nullprior.AssertOptionalBlockLeavesPlanUnknown` (`internal/testhelpers/nullprior`), which fails on a singleton-block leaf that plans a known value from a null prior. Reference without a collapse: `internal/resources/pro/policy/resource.go` `optComputedString` / `optComputedBool` / `optComputedInt` and `TestPolicyResource_NothingCollapsesAPlan`. Reference with one: `internal/resources/pro/mobile_device_app/resource.go` `optComputedString` / `optComputedBool` / `optComputedInt64`. The prestage enrollments and both configuration profiles also have one (a `ModifyPlan` that writes the plan) and use the in-existing-block modifiers, reproduced live 2026-10-14. A block that is never null in state when next declared is not a case for either modifier: `directory_binding` is one, because Jamf Pro populates the typed block whether or not it was sent and the state builder keeps it, so its test passes `nullprior.ExemptBlocks` with the reason. The exemption holds because `typeBlockConfigValidator` requires the block for its type at plan time and `RequiresReplace` on `type` keeps a type change out of Update. Relax either rule and the exemption has to go.

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

References: `internal/resources/pro/directory_binding/resource.go` (`type`: Required + OneOf with no canonicalising plan modifier, the preferred pattern; its `RequiresReplace` is unrelated); `internal/resources/pro/disk_encryption_configuration/resource.go` (`key_type` accepts only `Individual and Institutional` with lowercase `and`).

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

### Attribute defaults do not work inside `SetNestedAttribute`

An attribute `Default` (`booldefault.StaticBool`, and by construction its siblings) placed on an attribute **nested inside a `SetNestedAttribute`** overrides a value the configuration set **explicitly**, producing a permanent no-op diff. Verified against a live tenant on 2026-08-29 on `security_cloud/dns_hostname_mappings`: with `Optional + Computed + booldefault.StaticBool(false)`, a configuration saying `connect_to_ztna = true` planned as `false`, so every plan after the first proposed the same change forever. **It reproduces with a single element**, so it is not the set-element correlation problem that usually explains this shape.

Prefer **`Required`** for such an attribute. It removes the mechanism instead of working around it, and where the admin UI shows the control on every row — as it does for both traffic-vectoring checkboxes — every element genuinely has a value to state.

Two things this does *not* say. An `Optional` attribute inside a `SetNestedAttribute` with **neither** `Computed` nor `Default` is unaffected, verified in the same session. And **no unit test catches this** — the schema builds, the state builders round-trip, and every assertion passes. Pin the shape instead: assert `Required` and `Default == nil` in `schema_test.go`, with the reason, so a well-meaning later change to `Optional + Default` fails at the gate rather than in a user's plan.

### Referencing a server-managed catalog by name

When a resource references a **server-managed catalog** object — one the user never creates, e.g. a title catalog the server publishes — prefer exposing it **by name** with the ID `Computed`, rather than forcing the user to supply an opaque server ID:

- Expose `<thing>_name` as `Required`; keep `<thing>_id` `Computed` (plain `Computed`, no `UseStateForUnknown` — it is derived from the mutable name, so it must recompute when the name changes; see [§Server-derived computed fields](#server-derived-computed-fields--optionalcomputed-attributes)).
- **Resolve name → ID at apply** (Create/Update); a miss is a clear "not in catalog" error. Use the SDK's `Resolve…IDByName` where one exists; otherwise list the catalog through the service's own filter and decide the match in the provider (see `app_installer/name_lookup.go`).
- **Read/import must reverse-resolve ID → name** from the catalog (the object's `GET` returns only the ID). Do **not** echo the configured name back from state — on import there is no prior config, so an echo yields an empty name and fails `ImportStateVerify`. Make the reverse-resolve best-effort: preserve the existing state value on a transient catalog error rather than failing the refresh.
- **Safe only with an exact-match resolver, and a service-side filter is not one.** A case-insensitive/fuzzy match would accept `"jamf composer"`, store it, then reverse-resolve to the canonical `"Jamf Composer"` → perpetual diff. Confirm the resolver matches exactly: the platform `ResolveByNameClientPaged` does (`v != name`), but Jamf Pro's own RSQL `name==`/`titleName==` filter does **not** — it is case-insensitive and treats `*` as a glob, wire-verified on both the App Installer catalog and its deployments. So a filter narrows the request only; decide the match on byte equality in the provider, and treat two objects sharing one exact name as an ambiguity error rather than an arbitrary pick.
- **Plan-time name validation IS appropriate here** — unlike the sibling-resource case above. A server catalog is never created in the same apply, so a best-effort live-list/`GET` check (warn on transport error, error on a genuine miss, skip null/unknown) fails fast without breaking compose. Reference: `internal/resources/pro/app_request_settings/validators.go` (`validateAppStoreLocale`).

Reference: `jamfplatform_pro_app_installer.app_title_name` → Computed `app_title_id`, resolved by `internal/resources/pro/app_installer/name_lookup.go`.
