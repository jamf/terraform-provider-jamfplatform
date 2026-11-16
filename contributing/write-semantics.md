# Read, write and lifecycle semantics

Import hydration, server quirks, error handling, IDs and import, singletons, full-replace and merge-patch updates, classic collections, create and delete semantics, retry helpers. Part of the [style guide](../STYLE_GUIDE.md).

## First-time hydration detection in Read

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

## Working around server-broken classic endpoints

Some Jamf Pro classic endpoints are genuinely broken — the `/directorybindings/name/{name}` endpoint returns HTTP 500 for every name lookup as of 2026-05-23, even when the binding exists and is reachable by ID. List + ID paths work; only name lookup is dead.

When the SDK call backed by a broken endpoint is on a non-critical path (data source name resolver, optional fallback), route around it inside the resource package rather than waiting on the upstream fix:

- Implement a private helper in the resource package (e.g. `lookupByName` in `data_source.go`) that does **List + client-side name match + GetByID**.
- Doc-comment the helper with the bug summary, the date observed, and the upstream fix that would let the helper be deleted.
- Open an SDK issue so every consumer benefits once the SDK adopts the fallback (or once the server is fixed).

Reference: `internal/resources/pro/directory_binding/data_source.go` (`lookupByName`).

## SDK type-decode bugs — fix by generator override, not provider hand-decode

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

## Field-order-sensitive classic write payloads (rare)

The SDK generator emits struct fields **alphabetically**, so a marshaled classic `PUT`/`POST` body lists child elements in alphabetical order, not the order the OpenAPI spec / admin UI use. **Nearly every classic endpoint tolerates this** — the large majority of shipped classic resources marshal alphabetically and pass acceptance. Do **not** treat field order as a systemic risk or reorder structs pre-emptively.

A **rare** endpoint is order-sensitive on write and rejects the alphabetical body. Known case: ProClassic `/vppinvitations` `<general>` — `auto_register_managed_users` (alphabetically first) marshaled before the core fields returns **HTTP 500**; the same body in the GET wire order (`name`, `vpp_account`, `distribution_method`, …, `auto_register_managed_users` last) returns 201.

**Debugging cue:** a classic create/update that 500s with no obvious payload error (all fields present, names/types correct) — compare the marshaled child order against a live GET. If they differ and reordering to wire order fixes it, it's this.

**Fix in the SDK generator, per-schema — never globally.** A blanket "preserve spec order" switch would reorder every existing struct and risk regressing the resources that work today. Add a targeted per-schema field-order override (mirror the `fieldTypeOverrides` fix-prompt workflow above) so only the order-sensitive schema is reordered to wire order. Reference: `jamfplatform_pro_vpp_invitation` (SDK `a37279c`).

## Form-decoded classic input fields (encode on write)

A few classic text fields are **form-URL-decoded by the server on write** (`%XX` → byte, `+` → space). A literal `%` not followed by two hex digits is then a malformed escape and the write **500s**. Known case: `/vppinvitations` `<message>` (the invitation email body, which legitimately contains the `%@` registration-URL placeholder).

When a classic string field form-decodes input, **`url.QueryEscape` the value on write** in the input builder; the server decodes it back, so it stores and GETs the verbatim original (placeholders, literal `%`, embedded newlines all round-trip). **Read needs no transform** — the GET already returns the decoded value, which matches config. This also sidesteps newline-normalisation diffs for free. Reference: `encodedMessagePointer` in `internal/resources/pro/vpp_invitation/input_builders.go`.

## Error Handling

Use the shared helpers from `internal/common/helpers` rather than rolling your own:

- `helpers.IsNotFoundError(err)` — 404 detection in `Read`/`Delete` operations.
- `helpers.IsServerError(err)` — 5xx detection for retry decisions.
- `helpers.ResolveTimeout(ctx, value, defaultDuration)` — resolve `framework-timeouts` values to a concrete deadline.
- `helpers.ReconcileOptionalBool` / `ReconcileOptionalInt` / `ReconcileOptionalString` / `Reconcile*Pointer` — preserve Terraform null/optional semantics when the API returns zero values.
- `helpers.PreserveStringWhenWireEmpty(wire, current)` — for user-authored string fields where the Classic API echoes empty strings after successful writes. Defends against the masked-error-path bug when the field is a sibling of a `Sensitive` attribute. See §"State builders" rule 4a.
- **Whitespace Jamf Pro strips on save** is opt-in, per field: `helpers.ReconcileOptionalStringTrimmed` / `ReconcileOptionalStringPointerTrimmed` / `PreserveStringWhenWireEmptyTrimmed` / `StringPointerValueOrNullTrimmed` / `StringValueTrimmed` keep `current` when the wire value is `current` with its trailing, or leading and trailing, whitespace removed; any other server value wins and interior whitespace is compared exactly. Use them only for a field whose write reaches an endpoint wire-verified to trim, and cite the probe; the plain helpers stay strict, because on an endpoint that stores verbatim the tolerance hides an out-of-band trim. Wire-verified 2026-10-20, POST and PUT: Jamf Pro classic trims every string field on every endpoint probed (so a classic builder uses the `*Trimmed` forms; a raw `types.StringValue(wire)` there is unprotected until converted); Jamf Pro JSON trims the name on `/v1/departments`, `/v1/distribution-points`, `/v1/computer-extension-attributes` and `/v1/mobile-device-extension-attributes` only, and stores verbatim elsewhere (categories, buildings, scripts, packages, v3 computer groups, v2 mobile device groups, prestages, app installers, VPP locations, ADE instances, Venafi, and more — the list is on `wireIsTrimmedCurrent`). A resource with a classic fallback write (`smart_computer_group`, `smart_mobile_device_group`) trims on that path only. Without it, a heredoc's trailing `\n`, or a trailing space an object created in the Jamf Pro UI carries into state through import and `UseStateForUnknown`, fails apply with "Provider produced inconsistent result" (issue #510).
- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the error chain.

## ID type handling

Jamf endpoints expose mixed ID shapes: `pro/` uses integer IDs for many resources and UUIDs for newer ones; `proclassic/` is almost exclusively integer IDs. Terraform's `ID` attribute is always `types.String`.

**Convention**: always stringify integer IDs in Terraform state. Convert back to `int` / `int64` when calling the SDK.

Helpers in `internal/common/helpers/ids.go`:

- `IntIDToString(id int64) types.String`
- `StringToIntID(s types.String) (int64, error)`
- `StringValueFromIntPtr(*int) types.String` — for ProClassic SDK pointer IDs.
- UUIDs pass through unchanged as `types.String`.

`ImportState` parses the imported string per the rules below before populating state.

## Import format

Default convention for `ImportState`:

- **Single ID**: `terraform import jamfplatform_pro_<name>.foo <id>` — where `<id>` is the resource's primary key (integer or UUID as Jamf exposes it). Standard `resource.ImportStatePassthroughID` usage in most cases.
- **Composite / parent-scoped** (rare): `terraform import jamfplatform_pro_<name>.foo <parent_id>:<id>` — colon-separated. Parse with `strings.SplitN(req.ID, ":", 2)`. Validate both segments before setting state. Document the expected format in the resource's `MarkdownDescription` and in `examples/resources/<name>/import.sh`.

Avoid name-based imports; use IDs only.

## Endpoint shape classification

During the SDK-comparison gate ([CONTRIBUTING.md §Adding a Jamf Pro Resource](../CONTRIBUTING.md#adding-a-jamf-pro-resource) step 3), classify the endpoint based on which operations the SDK exposes:

| Operations available | Construct type | Notes |
|----------------------|----------------|-------|
| Create + Read + Update + Delete | `resource` (+ usually a singular `data source` and a plural `list resource`) | Standard CRUD |
| Read only (single + list, or list only) | `data source` (+ plural `data source` or `list resource`) | No state-managed object |
| Update only (no Create/Delete) | `resource` flagged as **singleton** — one record per tenant | See [Singleton resources](#singleton-resources) below for the full convention (fixed ID, Create→Update, no-op Delete, import format). E.g., `activation_code`, `computer_check_in_settings`, `mdm_profile_settings`, `self_service_plus_settings`. |
| Fire-and-forget command (Create returns command ID, no Read/Update/Delete) | `action` | E.g., `pro_computer_erase`, `pro_computer_restart` |

Record the classification in `JAMF_PRO_INVENTORY.md` Notes column during the in-design phase.

## Singleton resources

Jamf Pro objects that exist one-per-tenant and are exposed as Update-only on the API are modeled as **singleton** resources. The whole convention below is the load-bearing definition — any new singleton must follow it.

**Package**: `internal/resources/pro/<resource>/` — its own flat leaf package like every other Pro resource (`activation_code`, `computer_check_in_settings`, `mdm_profile_settings`, `self_service_plus_settings`, `jamf_pro_*`, …). Reference template: `internal/resources/pro/self_service_plus_settings/`.

**ProClassic singletons & secrets returned in clear**: a singleton may be ProClassic (e.g. `activation_code` on `/JSSResource/activationcode`) — funnel `Configure` through `providerdata.ConfigureProClassic` and keep the same Create→Update→GET shape. When a singleton field is a secret that the **GET returns in clear** (the classic activation-code GET echoes the license key verbatim, not a masked sentinel), model it as a plain `Required`/`Optional` + `Sensitive` string: normal drift detection applies and the WriteOnly + `_wo_version` machinery in [§Plaintext secrets](schema-design.md#plaintext-secrets--writeonly-with-_wo_version-rotation-companion) is unnecessary (that pattern is for secrets the API will **not** read back). When several fields share one write call, **always send the full set from state** — a partial PUT can wipe the others (a partial activation-code PUT risks wiping the license), so never build a sparse body.

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

## Tenant singletons with a real clear

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

## Full-replace endpoints & shared backing stores

Some Jamf Pro `PUT` endpoints are **full-replace**: any field omitted from the request body is reset to its server-side default, not left untouched. This is **not** discoverable from the SDK type or the OpenAPI spec — you **must** wire-probe it during the in-design phase. Probe: `GET` the object, `PUT` a body with one field changed and several others omitted, then `GET` again. If the omitted fields reverted to defaults, the endpoint is full-replace. Probe **both directions** (omit field A, then omit field B) — `/v4/enrollment` resets *every* omitted scalar. Record the finding in the spike doc and the `crud.go` annotation block.

Full-replace has three consequences:

**1. Every write must send a complete body.** Build the `PUT` payload from a fresh `GET` (not from plan alone), overlaying the fields the resource owns. Never send a sparse body.

**2. Default `Optional+Computed`+`UseStateForUnknown` for every user-settable optional scalar (omit = preserve).** A TF user must not need to know whether an endpoint merges or full-replaces to predict what omitting a field does. One mental model provider-wide: **declared ⇒ Terraform owns it (drift-reverted); omitted ⇒ preserved.** On a full-replace endpoint that means every user-settable optional field is `Optional + Computed` with `stringplanmodifier.UseStateForUnknown()` for a top-level scalar, and `UseNonNullStateForUnknown()` inside a nested collection or an optional block, switching to `planmodifiers.UseStateForUnknownInExistingBlock*` in an optional block only when the resource can collapse a plan (see rule 1a). Mechanism: omit ⇒ plan `Unknown` ⇒ the plan modifier carries the prior state value in ⇒ the input builder (`helpers.OptionalStringPointer`, already drop-on-null) re-emits it ⇒ full-replace keeps it. A *declared* field is a known plan value, the modifier no-ops, so it still drift-reverts — you only lose drift-revert on fields you omit, which is the point. Reference: `jamfplatform_pro_script` (`script_contents` et al.) and `jamfplatform_pro_building` (address scalars).

Carve-outs (a field matching any of these is **not** `Optional+Computed`), and one anti-pattern:
- **API-required ⇒ `Required`** (wire-probe / SDK / spec): if a create/update is rejected without the field, it is `Required` — no `Optional`, no `Computed`.
- **Read-only / server-derived ⇒ `Computed`-only** (`id`, derived `*_name`, echoes the user never sets — see [§Server-derived computed fields](schema-design.md#server-derived-computed-fields--optionalcomputed-attributes)).
- **Write-only secret objects ⇒ `Optional`-only, preserve-on-omit** — keystores / signing certs are not `Computed` (the server never echoes them back to absorb). See [§Write-only secrets are the exception to full-replace](#full-replace-endpoints--shared-backing-stores) below.
- **Individually-optional *nullable* fields inside a full-replace block ⇒ `Optional`-only** — when the server echoes `null` (no default to absorb) and rejects a blank/zero on a present field, model `Optional`-only and omit unset fields. See [§Individually-optional nullable fields](#full-replace-endpoints--shared-backing-stores) below.
- **Discriminator-gated fields** (only valid when a sibling discriminator has a given value — e.g. `script` only with `input_type = SCRIPT`; see [§Cross-field validation](schema-design.md#cross-field-validation)) split by whether the field is **required** or **optional** when its type is active:
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

**Write-only secrets are the exception to full-replace.** A write-only keystore/identity object (e.g. a signing certificate) may be **preserve-on-omit**: omitting the object keeps the stored secret, while a sibling toggle (not the object) is what deletes it. Wire-probe the lifecycle (set / keep-on-omit / delete) explicitly. In the merge, set such pointers to `nil` so `omitempty` omits them (preserves the secret) — **never** echo the server's `null` back, and only build the object when the user is uploading. See `applyCertToBody` in `user_initiated_enrollment_settings`. This mirrors the WriteOnly + `_wo_version` rotation pattern in [§Plaintext secrets](schema-design.md#plaintext-secrets--writeonly-with-_wo_version-rotation-companion).

**Individually-optional nullable fields inside a full-replace block.** Some full-replace nested blocks accept each field as independently optional **and nullable**: the server echoes `null` for an unset field (never a zero value) and **rejects** a blank string or non-positive number on any field that *is* present. For these (e.g. `jamfplatform_pro_app_installer.notification_settings`), model each field **Optional-only** (not `Optional+Computed` — there is no server default to absorb), **omit** unset fields from the request (send `nil`, never `""`/`0`), and preserve `null` in the flatten so state matches config. Add present-only validators (`LengthAtLeast(1)`, `AtLeast(1)`) so the server's constraint surfaces at plan time. Field-level full-replace still holds — dropping a previously-set field clears it to `null`. Reserve `Optional+Computed` for the sibling fields the server genuinely defaults (a boolean defaulting to `false`); a single block can mix the two principled-ly. **Don't zero-fill** "to send a complete body" — that's the trap that turns an unset field into a `400`.

## Merge-patch (`PATCH` → `204`) settings updates

Some pro settings endpoints update via **JSON merge-patch** (`Content-Type: application/merge-patch+json`) and return **`204 No Content`** — the opposite of the full-replace `PUT`-echoes-body shape above. Wire-probe to confirm (flip one field, omit the rest, `GET` again):

- **Omitting a field preserves its server value** (merge-patch semantics) — so `Optional+Computed` is correct here, and the *opposite* of the full-replace bias toward `Required`. Build the payload by sending **only** the fields the user set: map each `Optional+Computed` attribute to a pointer that is `nil` when the planned value is null/unknown, so `omitempty` drops it and the merge-patch leaves the server value untouched. (Reference: `optBool` in `computer_inventory_collection_settings/input_builders.go`.)
- **`204` means no echoed body**, so a `GET`-after-write is **mandatory** in both Create and Update to capture authoritative state (computed defaults, server coercions). This is the same GET-after the singleton convention already requires — but here it is load-bearing, not just future-proofing.
- Probe whether the server **coerces dependent fields** (a parent toggle that disables child sub-options): see [§Cross-field validation](schema-design.md#cross-field-validation) for handling — a config validator, not a plan modifier. Reference: `jamfplatform_pro_computer_inventory_collection_settings`.

### Merge-patch merges *inside* a nested object, so a discriminated block must send explicit nulls

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

## Classic XML `PUT` merge where *empty clears* (omit = retain)

A third write shape, distinct from both full-replace (omit = wipe) and merge-patch (omit = preserve): some classic XML `PUT` endpoints **merge**, but an **empty element clears** the field. Wire-probe both halves (`PUT` with a block/field omitted → `GET` (retained?); `PUT` with an empty `<field></field>` → `GET` (cleared?)). On `jamfplatform_pro_mobile_device_enrollment_profile`: omitting a field or whole block retains the server value; an empty element clears it; `PUT` returns `201` with no body (GET-after mandatory).

- **Always-emit managed scalar strings (empty when null).** For plain `Optional` strings, map null → a pointer to `""` (not `nil`) so a removed value is explicitly cleared and config↔state stays aligned. Omitting (sending `nil`) would leave the server value, producing a perpetual diff after the user deletes the attribute — and, because Read echoes the retained value into state, a failed apply ("Provider produced inconsistent result after apply") rather than a merely stale one (issue #384, six resources). The shared helpers are `helpers.AlwaysEmitStringPointer` and `helpers.AlwaysEmitBoolPointer` (null → `""` / `false`, unknown → `nil`), paired with `helpers.ReconcileOptional*Pointer` in the state builder. References: `printer/`, `network_segment/`, `directory_binding/`; `clearable()` in `mobile_device_enrollment_profile/input_builders.go` is the same shape, package-local.
- **Optional+Computed fields omit when null/unknown.** Fields with a genuine server default (booleans, a server-defaulted int, a `site_id` that defaults to `-1`) stay `Optional+Computed` and are *omitted* (`nil`) — the Computed-ness absorbs the server value, so "can't unset" is acceptable and avoids the empty-clear churn.
- **Clear by omission, not `= ""`.** The user-facing contract is: omit an attribute to clear it (null → emitted empty → server clears → `GET` null → state null == config null ✓). An explicit `= ""` is a *known* plan value the server drops to null, yielding a `plan("")`-vs-`state(null)` inconsistency — benign (HCL users omit, they don't assign `""`), but note it on the helper and write acceptance "clear" steps as omission, never `room = ""`.
- **Optional nested blocks the server always returns:** refresh a block only when it is already authored (model pointer non-nil) so the always-returned server defaults don't fabricate a block the user never declared; consequently the block is not populated on import → `ImportStateVerifyIgnore` it (same precedent as policy sub-blocks).

**Bearer-auth-only gaps — some classic sub-resources can't be written via this provider.** The Jamf Pro `/fileuploads/{resource}/...` attachment-upload endpoint **rejects OAuth bearer auth for some resources** while accepting it for others — `enrollmentprofiles` returns `403` via the platform gateway *and* `401` "requires user authentication" direct, while `mobiledevices` uploads succeed (`204`) with the same token. This provider authenticates only via OAuth bearer, so such sub-resources are **read-only** regardless of any SDK fix. Wire-probe the upload (it's the canonical signature) before promising a writable attachment/upload field; if bearer is refused, model the sub-resource read-only (`Computed` list) and document it. Reference: `mobile_device_enrollment_profile` attachments.

## Server-managed child collections via create/delete-only endpoints

Some settings objects carry a **child collection** whose members are managed by **dedicated create/delete endpoints** with **no update endpoint** and which the parent settings write cannot mutate (the inventory-collection custom application paths: the merge-patch settings body rejects new entries — "Id field is required" — because the server mints the id on create). Model these as a flat **`Set[String]`** of the user-meaningful field, not a `SetNestedAttribute`:

- **Diff by value, not by id.** A newly declared member has no id yet (the server assigns it on create), so the diff cannot key on id. Reconcile in Create/Update: value in plan ∉ server → POST (create); value on server ∉ plan → DELETE by id (resolve the id from a `GET` at apply-time). Keep the server id **out of Terraform state** entirely — it is only needed for the delete call. A changed member is naturally remove-old + add-new.
- This sidesteps both nested-collection traps at once — `Set` + `Computed` id instability, and the `Optional+Computed`-inside-nested plan-modifier rule — because the only user field (`path`) has no computed sibling and no nesting.
- **Filter server-managed built-ins.** Built-in members the server always returns (the built-in application paths carry the sentinel id `-1`) must be filtered out of state on Read, or they perma-diff against the user's declared set.
- **Probe value normalization.** If the server canonicalises the value (trailing slash, case), a string-match diff churns forever — wire-probe and either canonicalise or suppress. (Inventory-collection paths are stored verbatim, so no suppression is needed.)
- **Honour endpoint scope limits.** If the create endpoint's scope enum accepts only a subset (the inventory-collection custom-path scope accepts only `APP`, so the UI's Fonts/Plug-ins custom paths are unreachable via this API), model only what the API supports and **document the coverage gap** in the schema `MarkdownDescription`. Reference: `application_search_paths` in `jamfplatform_pro_computer_inventory_collection_settings`.

## Classic sub-collections — omit / empty / present clear semantics

A classic `PUT` **merges at the sub-element level**: a repeated wrapper like `<criteria>` or `<display_fields>` behaves three ways, wire-probed on `/advancedcomputersearches` and `/advancedusersearches`:

- **wrapper omitted** → server **keeps** the existing collection (classic top-level merge; note this is the opposite of category elements *inside* `<scope>`, whose omission clears once the subtree replace triggers — see §Granular per-category ownership).
- **wrapper present but empty** (`<criteria></criteria>`) → server **clears** the collection to zero.
- **wrapper present with members** → server **replaces** the whole collection (not merge-by-key).

So a resource that wants the Terraform config to be **authoritative** over a managed collection — including the ability to remove every member — must **always emit the wrapper** (empty when the user clears it), never rely on `omitempty`/nil. Build a non-nil-but-possibly-empty wrapper unconditionally; flatten an empty/absent server wrapper back to **null** (not `[]`) so an omitted block round-trips (`null` config → `null` state). Consequence for users: **omit the block to clear it; do not write `criteria = []`** — a known-empty list mismatches the null flatten and trips "produced inconsistent result after apply". Document this in the schema `MarkdownDescription` and exercise both the shrink (N→1) and clear (omit) transitions in acceptance. Reference: `internal/resources/pro/advanced_computer_search/` (`buildCriteriaWrapper`/`buildDisplayFieldsWrapper` always-emit; `criteria.BuildCriterionSlice` returns a non-nil empty slice).

**Alternatively, expose the wire-natural three-way directly (opt-out)** when leaving a collection *unmanaged* is a useful option and its data is costly to recreate (licence serials, purchase orders): emit the wrapper **only when the attribute is non-null** — `null`/omitted → not sent → server **retains** (the user opts out of managing the collection); `[]` → empty wrapper → **clears**; `[items]` → **replaces**. This relies on the framework decoding a null list to a nil Go slice and `[]` to a non-nil zero-length slice (verified), so the build's nil check separates "don't manage" from "clear". On read, only refresh a list whose incoming model is non-nil (managed) — leave an unmanaged list null and ignore the server echo, exactly like scope's granular per-category gating (`scope.RefreshManagedSet`) — and flatten an empty *managed* collection back to `[]` (not null) so a managed `[]` round-trips. Trade-off: `terraform import` leaves these lists unmanaged (the importer has no prior model), so `ImportStateVerify` must ignore them and users re-declare to take ownership. Reference: `internal/resources/pro/licensed_software/` (`software_definitions` + `licenses`; `buildLicensedSoftwareInput` conditional-emit, `assignLicensedSoftwareResourceModel` managed-gating).

**The same merge semantics apply to top-level *scalar* fields, not just repeated wrappers.** A classic `PUT` that omits an optional scalar element (e.g. `<description/>`) **keeps** the stored value — so a config that drops the field would never clear it, surfacing as "produced inconsistent result after apply" (plan `null`, state retains the old value). For a clearable optional scalar on a classic resource, **always emit the element** (empty when the user omits it — `<description></description>` clears it server-side, wire-probed) rather than using `omitempty`/nil; the state builder then reconciles the echoed `""` back to `null` so an omitted field round-trips. This is the scalar analogue of the always-emit-wrapper rule above, and it is **classic-only** — Pro JSON full-replace clears omitted scalars for free. Note the asymmetry *within* a classic object: top-level scalars merge, but a provided nested element (e.g. `<input_type>`) **replaces** its whole subtree. Reference: `internal/resources/pro/user_extension_attribute/` (`description` always-emitted; `input_type` subtree replaced). Two caveats, both wire-probed 2026-09-06: **the clear token is per field, not per endpoint** — an integer reference clears with its own sentinel (`account_group.ldap_server_id` clears with `<ldap_server><id>-1</id></ldap_server>`; `0` is refused `409 Problem with LDAP Server ID` and an empty `<ldap_server/>` retains), so probe the token before assuming `""`/`0`; and **a field the server requires under the current mode refuses the empty element** (`webhook.header` under `HEADER` → `409 INVALID_REQUIRED`, `webhook.username` under `BASIC` → `409 Username is required`), so emit the empty element only under the modes that do not require the field and add a plan-time validator for the modes that do. Every clear must be asserted on the wire in acceptance (`testhelpers.CheckLiveObject`), because a null in state cannot distinguish "cleared" from "retained and reconciled away".

## Server-*expanding* collections — intersect-on-read, not omit/empty/present

The omit/empty/present model above assumes the server stores **exactly** what you send. Some endpoints don't: they **silently expand** a submitted collection (adding dependency entries you never sent, sometimes into a *different* sub-collection) **and silently drop** unrecognised entries (returning `2xx`, no error). The Jamf Pro admin-account privilege grid is the canonical case (wire-probed): submitting `jss_objects=["Update Buildings"]` stores `jss_objects=["Update Buildings"]` **plus** `jss_settings=["Read Activation Code"]`, and an invalid string just vanishes. Naïvely storing the server echo permadiffs (config `{X}` vs state `{X, +server-added}`) and a typo permadiffs forever.

The reconciliation that works (wire-proven across all four cases):

- **Intersect-on-read.** On refresh-`Read`, for each sub-collection the config *manages* (non-null in prior state), store `declared ∩ server` — the user's declared members that the server still has. Server-added extras are dropped (never enter state ⇒ no diff); a genuine removal sticks (it's simply absent from `declared`); drift where the server loses a declared member shrinks the intersection ⇒ the next plan re-adds it. Sub-collections **null in prior state stay null** (unmanaged); import (no prior state) materialises the full server grid.
- **Trust the plan value on write — do NOT GET-after-write-and-intersect this collection.** If the server drops a value (typo / not-grantable), a GET-after-write intersect makes applied-state ≠ planned-config ⇒ a **hard "provider produced inconsistent result after apply"** abort. Writing the planned value to state (and reconciling only on the *next* refresh) degrades that to a soft diff instead. (Base/scalar fields can still GET-after-write as normal; only the expanding collection is plan-trusted.)
- **Validate at plan time**, because intersect-on-read turns a dropped typo into a *soft* diff but cannot prevent it. Discover the valid vocabulary from a live source and reject unknown entries in `ModifyPlan` (warn loudly, don't silently skip, if discovery fails). See §"plan-time validation against a live list".
- **Do NOT use a `state ⊇ config` semantic-equality plan modifier** to suppress the expansion diff. It is fatal: after *any* removal the new config is a proper subset of state, so `config ⊆ state` stays true and the modifier retains the removed member — it suppresses **every** removal, not just server-added ones. Intersect-on-read has no such flaw.

Model the collection as **plain `Optional`** (not `Optional+Computed`) — there is nothing for the framework to compute; the builder/state-builder own reconciliation. Reference: `internal/common/accountprivileges/` (`IntersectIntoState`, `Discover`/`Validate`) consumed by `internal/resources/pro/{account,account_group}/`.

The classic `<criterion>` element itself (smart groups + advanced searches) is shared: `criteria.CriterionAttributes(operators)` (schema), `criteria.CriterionModel` + `criteria.BuildCriterionSlice`/`FlattenCriterionSlice` (model + build/flatten). Pass `criteria.Operators` for the full set or `criteria.Without(...)` for the user-attribute subset. Note `device_group` is **not** a consumer — it is a Platform Services resource with divergent legacy attribute names (`order`/`criteria`/`operator`). Model an **ordered** classic collection (priority/parens load-bearing — e.g. `criteria`) as a `List`; model an unordered one the server re-sorts (e.g. `display_fields` columns) as a `Set`, so its order-independent comparison absorbs the server reorder rather than perma-diffing.

## Referenced-by-name dependencies & `HAS_DEPENDENCIES` on delete

Some Jamf Pro objects refuse deletion while another object references them, returning `406 HAS_DEPENDENCIES`. The pattern was wire-derived from the API role / API client pair (deleting a role still assigned to a client), whose constructs were withdrawn at the Platform API GA — no shipped construct currently has this shape, but the rules below hold for the next one that does. When a resource references another **by name** (rather than by a `Computed` id), be aware of two consequences:

- **Don't add a plan-time validator that checks the referenced name against a live server list.** A name interpolated from another resource created in the *same* apply is a *known* value at plan time (a `Required` display name, say), so the validator can't skip it on unknown-grounds — yet the referenced object isn't on the server yet, so the lookup fails with a false "not found" plan error. This breaks the canonical compose pattern. Rely on Jamf Pro's clear apply-time error instead. (A live-list validator is still correct for values that are *not* produced by sibling resources — e.g. a fixed per-version privilege list.)
- **The delete-while-referenced 406 is a Terraform ordering gotcha, not a provider bug.** Removing the reference *and* destroying the referenced resource in one apply can let Terraform sequence the destroy before the dependent's update (the config dependency edge is gone in the new graph). Document the constraint in the referenced resource's `MarkdownDescription` ("remove it from the consumer first, in a separate apply"). In acceptance tests, exercise scope add/remove by keeping the referenced fixture **defined-but-unreferenced** when dropping it from the consumer, so no in-use object is destroyed mid-test.

## Classic membership: author by username, mirror the resolved ID set as Computed

Some classic objects expose the **same membership in two parallel collections** — a username list and an ID list (e.g. `jamfplatform_pro_class` returns both `<students>`/`<student>` usernames and `<student_ids>`/`<id>`). The server resolves **both directions**: write usernames and it fills the IDs; write IDs and it fills the usernames. Model the **UI-facing identity** (username) as the authored attribute and the sibling ID list as plain `Computed`:

- `students` / `teachers` — `Optional` `Set` of usernames (the admin-UI surface).
- `student_ids` / `teacher_ids` — plain `Computed` `Set` of resolved IDs (no `UseStateForUnknown`; they recompute when the username set changes — same rule as any [server-derived field](schema-design.md#server-derived-computed-fields--optionalcomputed-attributes)).
- Do **not** send the ID collections on write — author by username only; the server resolves them.

Two wire behaviours flip the usual rules — **probe them, don't assume**:

- **Unknown usernames are auto-created, not rejected.** A username that matches no existing user returns `201`, and the server *mints a new user record* for it (verified on `/classes`). So — unlike the [server-managed catalog](schema-design.md#referencing-a-server-managed-catalog-by-name) case — **do NOT add a plan-time name-existence validator**: there is nothing to validate against, and a "not found" check would be wrong. Referenced **group IDs**, by contrast, *are* validated (a bogus ID returns `409`) — rely on Jamf Pro's apply-time error for those.
- **The server canonicalises usernames** (e.g. `Kyle@X` → `kyle@x` when it matches an existing user). A username-keyed `Set` therefore drifts unless the state builder preserves the configured casing: reconcile case-insensitively — when a returned value matches a configured one ignoring case, keep the configured spelling; otherwise take the server value. This both prevents the post-apply "produced inconsistent result" error and suppresses perpetual diffs.

Membership is **authoritative**: emit every collection in full on every write so the config is the source of truth — an empty wrapper element clears it (see [§Classic sub-collections — omit / empty / present clear semantics](#classic-sub-collections--omit--empty--present-clear-semantics), or the `advanced_computer_search` reference). Preserve the prior null-vs-empty set shape when the server returns nothing.

Reference: `jamfplatform_pro_class` (`students`/`teachers` → Computed `student_ids`/`teacher_ids`).

## Delete semantics: not-found, async, and propagation-blocked

The SDK transport does **not** retry 4xx and does **not** treat `DELETE→404` as success — both are consumer concerns (an eventual-consistency retry needs context the transport lacks; see [§Pro error/retry helpers](#pro-errorretry-helpers)). So **every `Delete` must branch on `helpers.IsNotFoundError`** to treat an already-absent record as success — never assume the SDK swallows the 404.

Beyond not-found, **wire-probe the delete** during the in-design phase — the response status, whether removal is synchronous, and (for async deletes) whether *polling itself interferes*. Probe: create a throwaway object, issue a single `DELETE` (capture status + body via `jamf-cli ... delete <id> -vvv`), then `GET`-by-id over time. Four patterns, each a distinct handler:

| Wire behaviour | Handler | Reference |
|---|---|---|
| Clean synchronous (`200`/`204`, `GET`→404 immediately) | Plain delete + `IsNotFoundError` not-found-as-success branch | `mac_app_store_app` (`/macapplications`) |
| Accepted behind a **misleading** status (e.g. `400` with an `<id>` body), removal prompt and **not** GET-sensitive | Poll `GET`-until-not-found via `helpers.ConfirmAsyncDelete`; error only on timeout | `mobile_device_app` (`/mobiledeviceapplications`) |
| Accepted behind a misleading status, slow **and GET-sensitive** — polling to confirm *delays* the server-side removal | **Fire-and-trust**: issue `DELETE` once, **never GET**, `helpers.IsClientError`→`AddWarning` (drop from state), surface 5xx/transport as errors. Destroy-time verification is impossible → acc `CheckDestroy` is a documented no-op | `ebook` (`/ebooks`) |
| Blocked by a still-propagating dependency (`406`/`409` while a dependent resource's own deletion catches up — Platform Services) | **Re-issue** the `DELETE` on `406`/`409` until it clears, the object is gone, or the timeout fires (`jamfplatform.PollUntil`). Re-issuing is *correct* here, unlike the accepted-async case | `device_group` |

The two misleading-status patterns look identical on the wire (same `400`) but diverge on GET-sensitivity — **probe it, do not assume** (`/ebooks` and `/mobiledeviceapplications` return the same misleading `400` yet behave oppositely under polling). Likewise never generalise one classic delete's behaviour to a sibling: `/macapplications` is clean-sync while the other two are misleading-async.

## Create semantics: a failed create is not proof nothing was created

The sections above settle what a **delete** error means. A **create** error needs the same care, because the failure that looks most like "nothing happened" can be the one where the service did the work and then failed the response. Treating it as "not created" leaves a live object the next apply collides with, and Terraform skips every dependent of the failed resource.

Two rules, both shipped for `device_group` (the reference) and neither applying to a construct until it is **wire-probed** the way a delete is:

1. **Reconcile, never retry.** The SDK does not resend a `POST` on `500`/`502`/`504` (it does on `429` and `503`), and a resource must not either: a second `POST` is how duplicates are made. When a create fails in a way that leaves the outcome unknown (`isAmbiguousCreateFailure`: a `5xx`, a `422`, an edge page, the create's own deadline; **not** a `400`/`403`/`409`, and not a cancelled context), *look*: list by name and device type, and adopt the group only if it was absent from a snapshot taken before the `POST` and every candidate matches the request field for field. Anything that does not match is somebody else's: report it and adopt nothing. Name uniqueness is what makes the snapshot sound, so a construct without an enforced unique key has no safe reconciliation and must not copy this.
2. **Say so, and never delete.** Adoption is a **warning** that quotes the original failure (rendered with `Error()`, because `helpers.APIErrorDetail`'s gateway advice says nothing changed, which is exactly what the warning contradicts) and lists any identical extras by ID. The provider does not remove them: the delete is irreversible on a shared tenant, and the argument that only this request could have made them is strong but not proof. Extras are not Terraform's, so dependents scope the adopted object and are unaffected by them.

Where the service degrades under concurrent writes, serialise them per provider instance (`providerdata.AcquireDeviceGroupWrite`, one slot, acquired **before** the operation timeout starts so queue time is not charged to the deadline). `-parallelism` cannot do this: it is global, not per type. State the evidence in the doc comment (date, tenant, counts), since a cap is a judgement about someone else's service.

Reference: `internal/resources/device_group/reconcile.go`, `crud_reconcile_test.go`.

## Pro error/retry helpers

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
