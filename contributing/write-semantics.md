# Read, write and lifecycle semantics

Import hydration, IDs and import, singletons, create, update (full-replace, merge-patch, classic merges and collections), delete, server and SDK quirks, error handling and retry helpers. Part of the [style guide](../STYLE_GUIDE.md).

Almost every rule here rests on a wire behaviour that neither the SDK types nor the OpenAPI spec reveal. Probe the endpoint during design, record the finding in the `crud.go` annotation block, and do not carry one endpoint's answer over to a sibling.

## First-time hydration detection in Read

`Read` must tell a first-time import hydration apart from an ordinary refresh. On hydration an optional, unmanaged attribute is filled from the wire; on every other read it is only reconciled against what the configuration already declares. Getting the signal wrong is silent: the attribute stays null and unmanaged, and no later plan shows the gap, because a null looks the same as a value the configuration never claimed.

**`req.State.Raw.IsNull()` is not the hydration signal.** It is true only on the identity-only refresh path, where Terraform supplies `req.Identity` and nothing else. It is false on the `terraform import <addr> <id>` path: `ImportStatePassthroughID` seeds state with the id before `Read` runs, so `req.State.Raw` is a populated object with `id` set and every other attribute null.

That check still has one correct use, and most `Read` methods carry it: deciding whether the id comes from `req.Identity` or from `req.State` (`isImport := req.State.Raw.IsNull()` in `pro/category/crud.go`). Use it for that and nothing else. A resource with no optional block or collection to hydrate needs no further signal.

**Gate hydration on a schema-`Required` attribute.** Create, Update and ordinary refresh always set it before `Read` runs; neither import path does, so `state.<RequiredAttr>.IsNull()` can only mean first-time hydration. Name the local `firstHydration` and copy a package that computes it this way (`account_group`, `mac_app_store_app`).

- Sample it off the immutable `req.State`, never off the model `Read` is assembling, which a later assignment can overwrite.
- Where no attribute is `Required`, an unconditionally-assigned `Computed` one works (`computer_prestage_enrollment` and `mobile_device_prestage_enrollment` use `site_id`). Prefer `Required`: the guarantee then comes from the schema, not from every assignment site staying ungated.
- Existing packages spell the same signal `importing` (`security_cloud/uem_connect`) or a package-local `importHydration` predicate feeding a `hydrating` local (`device_group`, `pro/account` and others). Use `firstHydration` in anything new.

**Thread the signal into every flattener that has an ownership gate of its own.** A nested flattener usually derives "did the config declare this" from a `prior` argument, and `prior` is nil on import because the parent gate has only just released. The parent then hydrates while the collection beneath it stays null on that read and every later one. The gate is spelled differently per package (`state.X != nil`, `declared != nil`, a `bool` parameter, a `prior` argument), so find these by reading the `Read` path, not by grepping.

**Adopt an optional collection only when the server returns members.** Adopting an empty one stores `[]` where a create that never declared the attribute stores null. Reference: `flattenSelfService` in `mobile_device_configuration_profile`.

Hydration changes state, so it changes what `ImportStateVerify` compares. See [TESTING.md](../TESTING.md).

### Blocks holding a `WriteOnly` secret

A gated block whose secret `Read` never sees (`keystore_file`, `password`, `wo_version` in `pki_adcs`, `pki_digicert`, `sso_settings`, `user_initiated_enrollment_settings`) needs the same release. Its rotation predicate returns "rotate" whenever the prior block is nil, which is always true after an import, so without hydration the first post-import apply re-uploads the certificate whether or not the practitioner touched the trigger, and with the trigger undeclared no diff says so.

- Release the gate and leave the predicate alone. `wo_version` is an ordinary `Optional` attribute in state, so once the block hydrates the predicate compares it: a config declaring `wo_version = 1` plans `+ wo_version = 1` against null and re-sends, which is what the trigger means; a config omitting it compares null with null and sends nothing.
- Do **not** treat a null prior `wo_version` as "no rotation signal". That either records a version that was never sent, permanently, or leaves a perpetual diff.
- Allocate the block only when the server reports a certificate, and document the `wo_version` diff in the resource's Import section.
- Where a validator requires `wo_version` alongside the secret (`pki_adcs`, `sso_settings` in `UPLOADED` mode, both `user_initiated_enrollment_settings` blocks), the configuration must supply a trigger the server cannot echo, so the post-import re-send is unavoidable and hydration only makes it visible. The pattern that removes it is the one `sso_settings` uses for `GENERATED`: read the wire for prior state (`currentCertSetupType`) and compare against the metadata the server does return.

### Never hydrate state that drives a destructive write

State that records the server, read back later as if the practitioner had asked for it, is the mistake. Before hydrating a block or collection, ask what else reads it.

- **Omitting the block deletes the secret.** `sso_settings` guards that delete with `plan.SigningCertificate == nil && state.SigningCertificate == nil`. Hydrating state on import stops the guard firing, and a practitioner who never declared the block hits `GENERATED → nil = DELETE`. Check what omission does on the wire before hydrating any write-only block and pin it in a test (`TestAssignSigningCertificateState_NeverHydratesAnUnauthoredBlock`).
- **Prior state is the diff that decides what to clear.** `patch_software_title.version_packages` is a managed subset over a full-replacement `packages` array: `Update` reads a prior-state key missing from the plan as an explicit unassign. That holds only while every key in state is one the configuration declared, so the resource hydrates nothing there and accepts the cosmetic addition diff (`TestManagedVersionPackages_NeverHydratesUndeclaredAssignments`).

## ID type handling

`pro/` uses integer IDs for many resources and UUIDs for newer ones; `proclassic/` is almost entirely integer. Terraform's `id` is always `types.String`: stringify integer IDs in state and convert back when calling the SDK.

Helpers in `internal/common/helpers/ids.go`:

- `IntIDToString(id int64) types.String`
- `StringToIntID(s types.String) (int64, error)`
- `StringValueFromIntPtr(*int) types.String`, for ProClassic pointer IDs.
- UUIDs pass through unchanged.

## Import format

- **Single ID**: `terraform import jamfplatform_pro_<name>.foo <id>`, where `<id>` is the primary key as Jamf exposes it. Usually `resource.ImportStatePassthroughID`.
- **Composite / parent-scoped** (rare): `<parent_id>:<id>`. Parse with `strings.SplitN(req.ID, ":", 2)` and validate both segments before setting state. Document the format in the `MarkdownDescription` and in `examples/resources/<name>/import.sh`.
- **Singletons**: the fixed identifier `singleton`. See [Singleton resources](#singleton-resources).

Avoid name-based imports; use IDs. The one sanctioned exception is a construct with no per-id read, such as `jamfplatform_account_sso_domain`, which imports by domain name ([families.md, Jamf Account rule 8](families.md#jamf-account-resource-naming)).

## Endpoint shape classification

During the SDK-comparison gate ([CONTRIBUTING.md §Adding a Jamf Pro Resource](../CONTRIBUTING.md#adding-a-jamf-pro-resource) step 3), classify the endpoint by the operations the SDK exposes:

| Operations available | Construct type | Notes |
|----------------------|----------------|-------|
| Create + Read + Update + Delete | `resource` (usually with a singular data source and a list resource) | Standard CRUD |
| Read only (single + list, or list only) | data source (plus a plural data source or list resource) | No state-managed object |
| Update only (no Create/Delete) | `resource` flagged as **singleton** | See [Singleton resources](#singleton-resources). E.g. `computer_check_in_settings`, `mdm_profile_settings`, `self_service_plus_settings`. |
| One per tenant, with a real clear | `resource` | See [Tenant singletons with a real clear](#tenant-singletons-with-a-real-clear). |
| Fire-and-forget command (no Read/Update/Delete) | `action` | E.g. the device `erase` and `restart` actions, `maintenance/flush_policy_logs` |

Record the classification in the Notes column of `spike/JAMF_PRO_INVENTORY.md`.

## Singleton resources

A Jamf Pro object that exists once per tenant and is Update-only on the API is a **singleton** resource. Reference template: `internal/resources/pro/self_service_plus_settings/`. Like every Pro resource it is its own flat leaf package named after the Terraform slug.

| Aspect | Rule |
|---|---|
| State ID | `helpers.SingletonID` (`"singleton"`, `internal/common/helpers/singleton.go`). Set it in Create, Read and Update. Never store another identifier. |
| `id` attribute | `Computed` with `stringplanmodifier.UseStateForUnknown()`. Never `Required`. |
| Identity schema | A single `id` string with `RequiredForImport: true`, always `helpers.SingletonID`. |
| Create | There is no create endpoint. Call the same `Update<X>` SDK method as `Update`, then `Get<X>` to capture authoritative state. The post-write GET is mandatory: it picks up server transformations and computed defaults. On a full-replace endpoint, read first and merge ([GET-on-create](#singletons-get-on-create-to-adopt-not-clobber)). |
| Delete | No-op on the tenant: the record cannot be deleted. Mark the unused `req`/`resp` parameters with `_`, emit one `tflog.Trace` line, mutate no state, and say in the doc comment that the missing SDK call is deliberate. |
| Nil-client guard | `Create`, `Read` and `Update` open with `if r.client == nil { resp.Diagnostics.AddError(...); return }` before reading plan or state. Keep the message in one package-local helper so the wording is uniform (e.g. `providerNotConfiguredError` in the Security Cloud DNS packages). `Delete` makes no SDK call and needs no guard. |
| Import | `terraform import <type>.<name> singleton`. `ImportState` must reject any other `req.ID` with an error; silently normalising on the next Read hides a mis-import. Then `resource.ImportStatePassthroughID`. `examples/resources/<type>/import.sh` contains this exact command. |
| List / plural | None. Provide the singular data source only where reading the settings without managing them is useful. |

**State builders.** SDK structs for singleton fields are commonly pointers with `json:",omitempty"`. For a `Required` attribute, fall back to the zero value on a nil response and say why in the doc comment (a `Required` attribute cannot be null in state). For an `Optional` one, use `helpers.ReconcileOptionalBoolPointer` / `ReconcileOptionalStringPointer` so a user-set null is preserved. The assigner must not write `state.ID`; the CRUD handler stamps `helpers.SingletonID` after the assign call.

**ProClassic singletons.** Funnel `Configure` through `providerdata.ConfigureProClassic` and keep the same Create→Update→GET shape (`activation_code` on `/JSSResource/activationcode`).

**Secrets the GET returns in clear.** Where the GET echoes a secret verbatim (the classic activation-code GET returns the licence key), model it as a plain `Required`/`Optional` + `Sensitive` string. Normal drift detection applies and the `WriteOnly` + `_wo_version` machinery in [§Plaintext secrets](schema-design.md#plaintext-secrets--writeonly-with-_wo_version-rotation-companion) is unnecessary; that pattern is for secrets the API will not read back.

**Fields sharing one write call.** Always send the full set from state. A partial PUT can wipe the others (a partial activation-code PUT risks wiping the licence).

Singleton acceptance and unit-test requirements are in [TESTING.md](../TESTING.md).

## Tenant singletons with a real clear

A third shape sits between [§Singleton resources](#singleton-resources) and ordinary CRUD: an object that exists once per tenant with no identifier, but whose `Delete` the API honours and whose absence is observable. Both Jamf Security Cloud custom DNS singletons are this shape (`jamfplatform_security_cloud_dns_search_domain`, `jamfplatform_security_cloud_dns_hostname_mappings`). Do not force one into §Singleton resources: a no-op `Delete` would leave `terraform destroy` not clearing tenant-wide configuration the API is willing to clear.

Follow §Singleton resources for the fixed `helpers.SingletonID`, the identity schema, the `id` attribute, the validated import identifier and the nil-client guards. Diverge on four points:

1. **`Delete` calls the clear endpoint.** Probe whether clearing an already-clear object is idempotent. Both DNS endpoints answer `204` either way, so neither needs more than the usual `helpers.IsNotFoundError` guard.
2. **`CheckDestroy` is the ordinary contract**: assert the object is gone. The inverted singleton check would pass while the provider cleared nothing.
3. **`Read` treats absence as deleted** and calls `resp.State.RemoveResource`. How absence presents is per construct: an unset search domain is a `404` carrying `SEARCH_DOMAIN_NOT_SET`, caught by `helpers.IsNotFoundError`; an empty hostname-mapping set is a `200` with `totalCount: 0`, recognised from the payload.
4. **`Delete` is unconditional.** The clear endpoints take no body and no precondition, so a value changed out of band since the last refresh is discarded without a diagnostic (`terraform destroy -refresh=false` never looks). Probe whether the clear is recoverable. Where the stored value cannot be reconstructed from anything Terraform holds, read before clearing and refuse the delete when what is stored is not what state records. Where it can (both DNS singletons), the unconditional clear is acceptable.

**`Create` refuses to replace a value that already exists.** These endpoints are unconditional upserts that report no conflict, so `Create` reads first and errors with the import command when a value is present. The preflight read is why the resource's declared SDK methods include the read privilege; `permissions_test.go` fails if the list is not updated.

The preflight catches a value that existed before the apply started. It does not close the window against a concurrent write, because the endpoint has no conditional-write primitive (no ETag, `If-Match` or version field), and it is not an ordering barrier between writers. Two blocks of the same resource in one configuration (legal HCL, and walked concurrently when nothing orders them), or two workspaces applying at once, can both read "unset" and both write. Last writer wins, both land in state under the same `helpers.SingletonID`, and every later plan proposes an update on the loser. No in-process lock helps across processes.

**A resource cannot detect a duplicate block in code.** `ModifyPlan`, `ConfigValidators` and `ValidateConfig` each see only their own instance. So the `MarkdownDescription` must say that the object is one per tenant, that declaring it twice (in one configuration or across workspaces) makes the instances fight, and that adopting an existing value means `terraform import`. Do not write a preflight that implies more.

A singular data source is worth providing. Whether an empty result is an error follows the wire: reading an unset search domain errors, because yielding `""` would feed that into whatever referenced it; reading an empty mapping set returns an empty collection.

## Create semantics: a failed create is not proof nothing was created

A create error that looks like "nothing happened" can be one where the service did the work and then failed the response. Treating it as "not created" leaves a live object the next apply collides with. Two rules, shipped for `device_group` (the reference) and applying to another construct only once it is wire-probed:

1. **Reconcile, never retry.** The SDK does not resend a `POST` on `500`/`502`/`504` (it does on `429` and `503`), and a resource must not either: a second `POST` makes duplicates. When a create fails with the outcome unknown (`isAmbiguousCreateFailure`: a `5xx`, a `422`, an edge page, the create's own deadline; **not** a `400`/`403`/`409`, and not a cancelled context), list by name and device type, and adopt the object only if it was absent from a snapshot taken before the `POST` and every candidate matches the request field for field. Anything that does not match belongs to someone else: report it and adopt nothing. Name uniqueness is what makes the snapshot sound, so a construct without an enforced unique key must not copy this.
2. **Say so, and never delete.** Adoption is a **warning** that quotes the original failure, rendered with `Error()` because `helpers.APIErrorDetail`'s gateway advice says nothing changed, which the warning contradicts. It lists any identical extras by ID. The provider does not remove them: the delete is irreversible on a shared tenant. Extras are not Terraform's, so dependents reference the adopted object and are unaffected.

Where the service degrades under concurrent writes, serialise them per provider instance (`providerdata.AcquireDeviceGroupWrite`, one slot, acquired **before** the operation timeout starts so queue time is not charged to the deadline). `-parallelism` cannot do this; it is global, not per type. State the evidence for the cap in the doc comment.

Reference: `internal/resources/device_group/reconcile.go`, `crud_reconcile_test.go`.

## Update semantics at a glance

What omitting a field does depends on the endpoint's write style, and so does the contract the user sees. Classify the endpoint by probe, then follow its section.

| Write style | Server on an omitted field | Schema for a user-settable optional scalar | User contract |
|---|---|---|---|
| [Full-replace JSON `PUT`](#full-replace-endpoints--shared-backing-stores) | Resets it to the default | `Optional+Computed` + state-preserving plan modifier | Omit to leave untouched; `""` (or `[]`) clears |
| [Merge-patch `PATCH`](#merge-patch-patch--204-settings-updates) | Keeps it | `Optional+Computed`, send only what is set | Omit to leave untouched |
| [Classic XML `PUT`, scalar](#classic-xml-put-merge-where-empty-clears-omit--retain) | Keeps it; an empty element clears | Plain `Optional`, always emitted | Omit to clear |
| [Classic XML `PUT`, sub-collection](#classic-sub-collections--omit--empty--present-clear-semantics) | Keeps it; an empty wrapper clears | Always-emit wrapper (authoritative), or conditional emit (opt-out) | Authoritative: omit to clear. Opt-out: omit to leave unmanaged, `[]` clears |

A field with a genuine server default is `Optional+Computed` in every style. The contract differs between JSON and classic resources, so each description states its own; do not describe one model as provider-wide.

## Full-replace endpoints & shared backing stores

Some Jamf Pro `PUT` endpoints are **full-replace**: a field omitted from the body is reset to its server default. Probe it: `GET`, `PUT` with one field changed and several omitted, `GET` again. Probe both directions (omit A, then omit B); `/v4/enrollment` resets every omitted scalar. Record the finding in the `crud.go` annotation block.

Three consequences follow.

### 1. Every write sends a complete body

Build the `PUT` payload from a fresh `GET`, overlaying the fields the resource owns. Never send a sparse body.

### 2. `Optional+Computed` with a state-preserving plan modifier for every user-settable optional scalar

Declared means Terraform owns the field and reverts drift; omitted means the existing value is preserved. Use:

- `stringplanmodifier.UseStateForUnknown()` (or the typed equivalent) for a top-level scalar;
- `UseNonNullStateForUnknown()` inside a nested collection or an optional block;
- `planmodifiers.UseStateForUnknownInExistingBlock*` in an optional block when the resource can collapse a plan ([rule 1a](schema-design.md#server-derived-computed-fields--optionalcomputed-attributes)).

Omitting the field plans it `Unknown`, the modifier carries the prior value in, the input builder (`helpers.OptionalStringPointer`, drop-on-null) re-emits it, and the full-replace keeps it. A declared field is a known plan value, so it still drift-reverts.

**Never ship `Optional+Computed` on a full-replace field without the plan modifier.** The omitted field then drops from the `PUT` and the server clears it while the schema reads as "preserve".

References: `jamfplatform_pro_script`, `jamfplatform_pro_building`. This applies to every full-replace resource, singleton settings included (`jamfplatform_pro_re_enrollment_settings`); `Required` is reserved for fields the API requires.

A field matching one of these is **not** `Optional+Computed`:

| Field | Model as |
|---|---|
| API-required (a create or update is rejected without it) | `Required` |
| Read-only / server-derived (`id`, derived `*_name`, echoes) | `Computed` only. See [§Server-derived computed fields](schema-design.md#server-derived-computed-fields--optionalcomputed-attributes). |
| Write-only secret object (keystore, signing certificate) | `Optional` only, preserve-on-omit. See [below](#write-only-secrets-are-the-exception-to-full-replace). |
| Individually-optional nullable field in a full-replace block | `Optional` only, omitted when unset. See [below](#individually-optional-nullable-fields-inside-a-full-replace-block). |
| Discriminator-gated field | See [below](#discriminator-gated-field-clearing). |

**Description convention.** End each such *string* field's `MarkdownDescription` with: ``Omit to leave any existing value untouched (it is not cleared on update); set to `""` to clear it.`` Sets clear with `[]`. Bool, int and enum fields have no blank: omit preserves, a concrete value changes; word those accordingly.

**Acceptance coverage.** Each full-replace resource carries one split-ownership test proving omit = preserve and `""`/`[]` = clear on a representative field. See [TESTING.md](../TESTING.md); references are `TestAccResource_ProScript_SplitOwnership` and `TestAccResource_ProBuilding_SplitOwnership`.

#### Discriminator-gated field clearing

A field valid only when a sibling discriminator has a given value (`script` only with `input_type = SCRIPT`; see [§Cross-field validation](schema-design.md#cross-field-validation)) splits on whether it is required or optional while its type is active.

- **Required-when-active** (`script` when `SCRIPT`): plain `Optional`. It is always declared when valid, so there is no omit path to protect. `UseStateForUnknown` is wrong here: on a transition (`SCRIPT` → `TEXT`, config omits `script`) it carries the prior value into the plan, the builder sends it, and the server `400`s. The config validator reads `req.Config`, not the modified plan, so it does not catch this.
- **Optional-when-active** (`popup_menu_choices` when `POPUP`): plain `Optional` leaves wipe-on-omit. Convert it only with both of: **(a)** an input builder that sends the companion field only while its discriminator value is active; **(b)** a discriminator-aware plan modifier that behaves as `UseStateForUnknown` while the type is active and predicts null once the discriminator has moved away, so a transition does not fail with "inconsistent result after apply". Reference: `popupChoicesPlanModifier` and the `switch inputType` in the `computer_extension_attribute` input builder.

#### Write-only secrets are the exception to full-replace

A write-only keystore or identity object may be preserve-on-omit: omitting the object keeps the stored secret, and a sibling toggle is what deletes it. Probe the lifecycle (set, keep-on-omit, delete). In the merge, set such pointers to `nil` so `omitempty` drops them; never echo the server's `null` back, and build the object only when the user is uploading. Reference: `applyCertToBody` in `user_initiated_enrollment_settings`. Rotation follows [§Plaintext secrets](schema-design.md#plaintext-secrets--writeonly-with-_wo_version-rotation-companion).

#### Individually-optional nullable fields inside a full-replace block

Some full-replace nested blocks accept each field as independently optional and nullable: the server echoes `null` for an unset field and rejects a blank string or non-positive number on a present one (`jamfplatform_pro_app_installer.notification_settings`).

- Model each such field `Optional` only; there is no server default to absorb.
- Omit unset fields from the request (`nil`, never `""`/`0`). Do not zero-fill "to send a complete body"; that turns an unset field into a `400`.
- Preserve `null` in the flatten.
- Add present-only validators (`LengthAtLeast(1)`, `AtLeast(1)`) so the constraint surfaces at plan time.
- Dropping a previously set field still clears it to `null`.
- Sibling fields the server does default (a boolean defaulting to `false`) stay `Optional+Computed`; one block can mix the two.

#### Singletons: GET-on-create to adopt, not clobber

`UseStateForUnknown` preserves an omitted field only on update. On first create there is no prior state, so an omitted field drops from the write and the server resets it. For a multi-instance resource that is correct. A singleton always pre-exists, so its create is adoption: read the live object in `Create` (inside any write lock) and merge the declared fields over it. Do this where the endpoint allows it; decide per resource from the probe.

Reference: `jamfplatform_pro_re_enrollment_settings`. `Create` reads `GetReenrollmentSettingsV1` and passes it to `buildReenrollmentInput` as the merge base; on update the base is nil because the plan modifier has already filled the plan.

In the state builder, `Optional+Computed` strings reconcile with `ReconcileOptionalStringPointer` (it handles the `""` blank). `Optional+Computed` bools and ints adopt the server value directly (`BoolPointerValueOrNull`): the bool/int `Reconcile*` helpers return null when prior state is unset, which would blank an omitted toggle.

**A singleton with `Optional`-only fields needs the merge base on update too.** An `Optional`-only field plans as null when omitted, and null is a reset on a full-replace endpoint. `sso_settings` is the reference: the enrollment SSO block and the SAML metadata file are `Optional`-only, so `Create` and `Update` both read `/v3/sso` and merge the plan over it. The base is consulted only where the plan has no value, so a declared field still wins and `""` still clears. Where the field does have a server default, convert it to `Optional+Computed` instead (`group_enrollment_access_name`).

### 3. Shared backing store: read-merge-write and a process-shared mutex

When two endpoints or two resources are views of one record:

- **Round-trip non-owned fields.** A resource owning a subset of a full-replace object reads the live `GET`, overlays its own fields and passes the rest back unchanged. `jamfplatform_pro_user_initiated_enrollment_settings` round-trips the six re-enrollment fields it does not own (`mergeEnrollmentSettingsInput`).
- **Serialise with a provider-shared mutex.** Terraform applies dependency-free resources concurrently, so two read-merge-writes against one store can overwrite each other from a stale read. Add a by-reference `sync.Mutex` to `providerdata.Data` with an exported accessor (`EnrollmentWriteLock()`), take it in each resource's `Configure` (type-assert `req.ProviderData.(*providerdata.Data)`, nil-safe), and hold it around the whole `GET → merge → PUT` in `Create` and `Update`. `Read` takes no lock. Calls to a different endpoint run outside it.
- **Document the cross-process gap.** The mutex serialises one provider process. Two `terraform apply` runs against one tenant still race when the object has no version or ETag. Say so in the resource doc comment.

## Merge-patch (`PATCH` → `204`) settings updates

Some Pro settings endpoints update by JSON merge-patch (`Content-Type: application/merge-patch+json`) and return `204 No Content`. Probe to confirm: flip one field, omit the rest, `GET` again.

- **Omitting a field preserves its server value.** Model user-settable fields `Optional+Computed` and send only what the user set: map each attribute to a pointer that is `nil` when the planned value is null or unknown, so `omitempty` drops it. Reference: `optBool` in `computer_inventory_collection_settings/input_builders.go`.
- **`204` carries no body**, so a `GET` after the write is mandatory in Create and Update.
- Probe whether the server **coerces dependent fields** (a parent toggle that disables child options). Handle that with a config validator, not a plan modifier; see [§Cross-field validation](schema-design.md#cross-field-validation). Reference: `jamfplatform_pro_computer_inventory_collection_settings`.

### Merge-patch merges *inside* a nested object, so a discriminated block must send explicit nulls

Merge-patch merges into a nested object field by field. When the object is discriminated (one member selects which siblings are legal), a change of discriminator cannot be expressed by omission. On `jamfplatform_security_cloud_ztna_app`'s `routing` block, whose `mode` decides whether a gateway and a resolution mode are required or forbidden:

- switching to the mode that **forbids** the siblings while sending only the discriminator leaves the old values merged in, and the server refuses the combination;
- switching to the mode that **requires** them while sending the discriminator and one sibling arrives missing the other, and the server refuses that too.

Both draw `400 [INVALID_FIELD] routing: Routing definition is not valid.`, so the failure reads as a validation bug.

- **Send the whole discriminated object on every write, with absent members as explicit `null`s.**
- **A nil pointer with `,omitempty` cannot express `null`.** The fix is in the SDK: list the schema in `emitNullForOptional` in the SDK repo's `tools/generate/config.json`, which drops `,omitempty` from that schema's pointer fields. No provider-side workaround exists, because the generated `*PatchRequest` type is the method's parameter. Probe that the create path also accepts explicit nulls first; on this endpoint it does.

Reference: `routingToWire` in `internal/resources/security_cloud/ztna_app/input_builders.go`, and `TestRoutingToWireEmitsExplicitNulls`, which asserts the marshalled JSON because the struct is identical either way.

## Classic XML `PUT` merge where *empty clears* (omit = retain)

A classic XML `PUT` **merges**: an omitted element keeps the stored value, and an empty element clears it. The `PUT` returns no body, so a GET after the write is mandatory. Probe both halves (`PUT` with the field omitted, then with `<field></field>`). The collection form of the same rule is in [§Classic sub-collections](#classic-sub-collections--omit--empty--present-clear-semantics).

- **Always emit a managed optional scalar, empty when null.** Map null to a pointer to `""`, not `nil`, so a removed value is cleared. Sending `nil` keeps the server value, Read echoes it into state, and the apply fails with "Provider produced inconsistent result after apply". Use `helpers.AlwaysEmitStringPointer` / `helpers.AlwaysEmitBoolPointer` (null → `""`/`false`, unknown → `nil`) with `helpers.ReconcileOptional*Pointer` in the state builder, which turns the echoed `""` back into null. References: `printer/`, `network_segment/`, `directory_binding/`, `user_extension_attribute/` (`description`); `clearable()` in `mobile_device_enrollment_profile/input_builders.go` is the same shape, package-local.
- **This is classic-only.** A Pro JSON full-replace clears an omitted scalar without help.
- **Top-level scalars merge, but a provided nested element replaces its whole subtree** (`<input_type>` in `user_extension_attribute`).
- **`Optional+Computed` fields omit when null or unknown.** A field with a genuine server default (booleans, a defaulted int, a `site_id` defaulting to `-1`) is sent as `nil`; the `Computed` half absorbs the server value, and "cannot unset" is acceptable.
- **The user clears by omission, not `= ""`.** An explicit `""` is a known plan value the server stores as null, giving plan `""` against state null. Note this on the helper and write acceptance clear steps as omission.
- **The clear token is per field, not per endpoint.** An integer reference clears with its own sentinel: `account_group.ldap_server_id` clears with `<ldap_server><id>-1</id></ldap_server>`; `0` is refused `409 Problem with LDAP Server ID`, and an empty `<ldap_server/>` retains. Probe the token before assuming `""` or `0`.
- **A field the server requires under the current mode refuses the empty element** (`webhook.header` under `HEADER` → `409 INVALID_REQUIRED`; `webhook.username` under `BASIC` → `409 Username is required`). Emit the empty element only under modes that do not require the field, and add a plan-time validator for the modes that do.
- **Optional nested blocks the server always returns:** refresh a block only when it is already authored (model pointer non-nil), so server defaults do not fabricate a block the user never declared. The block is then not populated on import, so `ImportStateVerifyIgnore` it.

A null in state cannot distinguish "cleared" from "retained and reconciled away", so every clear is asserted on the wire in acceptance; see [TESTING.md](../TESTING.md).

### Bearer-auth-only gaps

The Jamf Pro `/fileuploads/{resource}/...` attachment endpoint rejects OAuth bearer auth for some resources and accepts it for others: `enrollmentprofiles` returns `403` through the platform gateway and `401` "requires user authentication" direct, while `mobiledevices` uploads succeed (`204`) with the same token. This provider authenticates only by bearer, so such sub-resources are read-only whatever the SDK does. Probe the upload before promising a writable attachment field; if bearer is refused, model the sub-resource as a `Computed` list and document it. Reference: `mobile_device_enrollment_profile` attachments.

## Classic sub-collections — omit / empty / present clear semantics

A classic `PUT` merges at the sub-element level. A repeated wrapper such as `<criteria>` or `<display_fields>` behaves three ways (`/advancedcomputersearches`, `/advancedusersearches`):

| Wrapper in the body | Server |
|---|---|
| Omitted | Keeps the existing collection |
| Present, empty (`<criteria></criteria>`) | Clears it |
| Present, with members | Replaces the whole collection (not merge-by-key) |

Category elements inside `<scope>` are the opposite on omission: they clear once the subtree replace triggers. See the [scope helper](scope.md#scope-helper).

**Authoritative (default).** For the configuration to own the collection, including removing every member, always emit the wrapper, empty when the user clears it. Never rely on `omitempty` or nil. Flatten an empty or absent server wrapper to **null**, not `[]`, so an omitted block round-trips. The user contract is then: omit the block to clear it; `criteria = []` mismatches the null flatten and fails with "produced inconsistent result after apply". Say so in the `MarkdownDescription`, and cover shrink (N→1) and clear (omit) in acceptance. Reference: `internal/resources/pro/advanced_computer_search/` (`buildCriteriaWrapper`, `buildDisplayFieldsWrapper`; `criteria.BuildCriterionSlice` returns a non-nil empty slice).

**Opt-out (wire-natural three-way).** Where leaving a collection unmanaged is useful and its data is costly to recreate (licence serials, purchase orders), emit the wrapper only when the attribute is non-null: null → not sent → server retains; `[]` → empty wrapper → clears; `[items]` → replaces. The framework decodes a null list to a nil slice and `[]` to a non-nil empty slice, so the builder's nil check separates "do not manage" from "clear". On read, refresh only a list whose incoming model is non-nil, leave an unmanaged list null, and flatten an empty managed collection to `[]`. This is the same gating as scope's `scope.RefreshManagedSet`. Trade-off: `terraform import` leaves these lists unmanaged, so `ImportStateVerify` ignores them and users re-declare to take ownership. Reference: `internal/resources/pro/licensed_software/` (`software_definitions`, `licenses`; `buildLicensedSoftwareInput`, `assignLicensedSoftwareResourceModel`).

Top-level classic scalars follow the same merge; the always-emit rule for them is in [§Classic XML `PUT` merge](#classic-xml-put-merge-where-empty-clears-omit--retain).

### The shared `<criterion>` element

The classic `<criterion>` element (smart groups, advanced searches) is shared: `criteria.CriterionAttributes(operators)` for the schema, `criteria.CriterionModel` with `criteria.BuildCriterionSlice` / `FlattenCriterionSlice` for model, build and flatten. Pass `criteria.Operators` for the full set or `criteria.Without(...)` for the user-attribute subset. `device_group` is not a consumer; it is a Platform Services resource with different attribute names.

Model an **ordered** classic collection (`criteria`, where priority and parentheses matter) as a `List`, and an unordered one the server re-sorts (`display_fields`) as a `Set`. See [§Sets vs Lists](schema-design.md#sets-vs-lists).

## Server-managed child collections via create/delete-only endpoints

Some settings objects carry a child collection managed by dedicated create and delete endpoints, with no update endpoint, which the parent settings write cannot change. The inventory-collection custom application paths are the case: the merge-patch body rejects new entries ("Id field is required") because the server mints the id on create. Model these as a flat `Set[String]` of the user-meaningful field, not a `SetNestedAttribute`.

- **Diff by value, not by id.** In Create and Update: value in plan and not on server → `POST`; value on server and not in plan → `DELETE` by id, resolving the id from a `GET` at apply time. Keep the server id out of state. A changed member is remove plus add.
- This avoids both nested-collection traps: `Set` with an unstable `Computed` id, and `Optional+Computed` inside a nested collection.
- **Filter server-managed built-ins** on Read (the built-in application paths carry the sentinel id `-1`), or they diff forever against the declared set.
- **Probe value normalisation.** If the server canonicalises the value (trailing slash, case), canonicalise or suppress. Inventory-collection paths are stored verbatim.
- **Honour endpoint scope limits.** The custom-path create accepts only scope `APP`, so the UI's Fonts and Plug-ins paths are unreachable. Model what the API supports and document the gap in the `MarkdownDescription`.

Reference: `application_search_paths` in `jamfplatform_pro_computer_inventory_collection_settings`.

## Server-*expanding* collections — intersect-on-read, not omit/empty/present

The omit/empty/present model assumes the server stores exactly what it is sent. Some endpoints silently **expand** a submitted collection, adding dependency entries, sometimes into a different sub-collection, and silently **drop** unrecognised entries with a `2xx`. The Jamf Pro admin-account privilege grid is the case: `jss_objects=["Update Buildings"]` is stored with `jss_settings=["Read Activation Code"]` added, and an invalid string vanishes. Storing the server echo diffs forever.

- **Intersect on read.** For each sub-collection the configuration manages (non-null in prior state), store `declared ∩ server`. Server-added extras never enter state; a removal sticks; a declared member the server lost shrinks the intersection and the next plan re-adds it. A sub-collection null in prior state stays null. Import, with no prior state, takes the full server grid.
- **Trust the plan value on write.** Do not GET-after-write and intersect this collection: if the server dropped a value, applied state would differ from the plan and the apply would abort with "provider produced inconsistent result". Writing the planned value and reconciling on the next refresh makes it a soft diff. Scalar fields still GET-after-write as usual.
- **Validate at plan time.** Discover the valid vocabulary from a live source and reject unknown entries in `ModifyPlan`; if discovery fails, warn, do not skip silently. A live list is safe here because the vocabulary is not produced by sibling resources; compare [§Referenced-by-name dependencies](#referenced-by-name-dependencies--has_dependencies-on-delete).
- **Do not use a `state ⊇ config` semantic-equality plan modifier.** After any removal the new config is a subset of state, so the modifier keeps the removed member. It suppresses every removal.
- Model the collection as plain `Optional`. There is nothing for the framework to compute.

Reference: `internal/common/accountprivileges/` (`IntersectIntoState`, `Discover`, `Validate`), used by `internal/resources/pro/{account,account_group}/`.

## Classic membership: author by username, mirror the resolved ID set as Computed

Some classic objects expose one membership in two parallel collections, a username list and an ID list (`jamfplatform_pro_class` returns `<students>` and `<student_ids>`). The server resolves both directions. Author the UI-facing identity and compute the other:

- `students` / `teachers`: `Optional` `Set` of usernames.
- `student_ids` / `teacher_ids`: plain `Computed` `Set`, with no `UseStateForUnknown`, since they recompute when the username set changes (as for any [server-derived field](schema-design.md#server-derived-computed-fields--optionalcomputed-attributes)).
- Do not send the ID collections on write.

Two wire behaviours to probe, not assume:

- **Unknown usernames are created, not rejected.** A username matching no user returns `201` and the server mints a user record. So do not add a plan-time name-existence validator, unlike the [server-managed catalog](schema-design.md#referencing-a-server-managed-catalog-by-name) case. Referenced group IDs are validated (a bogus ID returns `409`); rely on that apply-time error.
- **The server canonicalises usernames** (`Kyle@X` → `kyle@x` when it matches an existing user). Reconcile case-insensitively in the state builder: keep the configured spelling when a returned value matches it ignoring case, otherwise take the server value.

Membership is authoritative: emit every collection in full on every write; an empty wrapper clears it ([§Classic sub-collections](#classic-sub-collections--omit--empty--present-clear-semantics)). Preserve the prior null-versus-empty shape when the server returns nothing.

Reference: `jamfplatform_pro_class`.

## Delete semantics: not-found, async, and propagation-blocked

The SDK transport does not retry 4xx and does not treat `DELETE` → `404` as success. **Every `Delete` branches on `helpers.IsNotFoundError`** and treats an already-absent record as success.

Beyond that, probe the delete: the response status, whether removal is synchronous, and for async deletes whether polling itself interferes. Create a throwaway object, issue one `DELETE` capturing status and body, then `GET` by id over time. Four patterns:

| Wire behaviour | Handler | Reference |
|---|---|---|
| Clean synchronous (`200`/`204`, `GET` → 404 at once) | Plain delete with the not-found-as-success branch | `mac_app_store_app` (`/macapplications`) |
| Accepted behind a **misleading** status (e.g. `400` with an `<id>` body), removal prompt and not GET-sensitive | Poll `GET` until not found with `helpers.ConfirmAsyncDelete`; error only on timeout | `mobile_device_app` (`/mobiledeviceapplications`) |
| Accepted behind a misleading status, slow **and GET-sensitive** (polling delays the removal) | **Fire-and-trust**: one `DELETE`, never `GET`; `helpers.IsClientError` → `AddWarning` and drop from state; surface 5xx and transport errors. Destroy cannot be verified, so acceptance `CheckDestroy` is a documented no-op | `ebook` (`/ebooks`) |
| Blocked by a still-propagating dependency (`406`/`409` while a dependent's own deletion catches up; Platform Services) | **Re-issue** the `DELETE` on `406`/`409` until it clears, the object is gone, or the timeout fires (`jamfplatform.PollUntil`) | `device_group` |

The two misleading-status patterns look the same on the wire and differ only in GET-sensitivity. Never generalise one classic delete to a sibling: `/macapplications` is clean-sync while `/ebooks` and `/mobiledeviceapplications` are not.

## Referenced-by-name dependencies & `HAS_DEPENDENCIES` on delete

Some Jamf Pro objects refuse deletion while another object references them, returning `406 HAS_DEPENDENCIES`. No shipped construct has this shape today (the API role and API client pair that did was withdrawn at the Platform API GA), but the rules hold for the next one. When a resource references another **by name**:

- **Do not validate the referenced name against a live server list at plan time.** A name interpolated from a resource created in the same apply is a known value at plan time, so the validator cannot defer on unknown, yet the object is not on the server, and the plan fails with a false "not found". Rely on Jamf Pro's apply-time error. A live-list validator remains correct for values not produced by sibling resources, such as a fixed privilege list.
- **The delete-while-referenced `406` is a Terraform ordering trap.** Removing the reference and destroying the referenced resource in one apply can sequence the destroy before the dependent's update. Say in the referenced resource's `MarkdownDescription` to remove it from the consumer first, in a separate apply. Acceptance fixtures for this are described in [TESTING.md](../TESTING.md).

## Working around server-broken classic endpoints

Some classic endpoints are broken on the server. `/directorybindings/name/{name}` returns `500` for every name lookup even when the binding exists and is reachable by ID; list and ID paths work.

When the broken call is on a non-critical path (a data source name resolver, an optional fallback), route around it inside the resource package: a private helper that does **List + client-side name match + GetByID**. Its doc comment states the bug and the upstream fix that would let it be deleted, and an SDK issue tracks it ([CONTRIBUTING.md](../CONTRIBUTING.md)).

Reference: `lookupByName` in `internal/resources/pro/directory_binding/data_source.go`.

## SDK type-decode bugs — fix by generator override, not provider hand-decode

This is a broken SDK *type*, not a broken endpoint: the endpoint works, but the generated struct cannot decode the response.

The recurring shape: the spec declares `type: string, format: date-time`, so the generator emits `*time.Time`, but the server sends a timezone-less value (`"2028-05-15T13:41:46"`). Go's RFC 3339 decoder rejects it, and `json.Unmarshal` aborts the whole body on the first field error, so the SDK call returns `nil, err`. The same applies to any spec/server type mismatch (int vs string, object vs map).

**Fix it in the SDK with a generator override. Never hand-decode in the provider.** A bespoke `UnmarshalJSON`, a regex pre-parse or an error-tolerant fallback in a resource package hides the defect from every other SDK consumer and is rejected in review. The override procedure is in [CONTRIBUTING.md](../CONTRIBUTING.md).

On the provider side, model the field as a `Computed` echo string and read the SDK's `*string` straight through. A field the spec declares plain `type: string` (`SsoKeystoreDetails.expiration`) already has that shape. If a resource is blocked on the SDK fix, pin the dependency and wait; do not hand-decode to unblock.

## Field-order-sensitive classic write payloads (rare)

The SDK generator emits struct fields alphabetically, so a marshalled classic body lists child elements alphabetically, not in spec or UI order. Nearly every classic endpoint tolerates this. Do not treat field order as a systemic risk or reorder structs pre-emptively.

A rare endpoint rejects the alphabetical body. Known case: ProClassic `/vppinvitations` `<general>`, where `auto_register_managed_users` marshalled before the core fields returns `500`, and the same body in GET wire order returns `201`.

**Debugging cue:** a classic create or update that `500`s with all fields present and correct. Compare the marshalled child order against a live GET.

**Fix in the SDK generator, per schema, never globally.** A blanket "preserve spec order" switch would reorder every struct and risk the resources that work. Add a per-schema field-order override. Reference: `jamfplatform_pro_vpp_invitation`.

## Form-decoded classic input fields (encode on write)

A few classic text fields are form-URL-decoded by the server on write (`%XX` → byte, `+` → space). A literal `%` not followed by two hex digits is a malformed escape and the write `500`s. Known case: `/vppinvitations` `<message>`, which legitimately contains the `%@` registration-URL placeholder.

`url.QueryEscape` such a value in the input builder. The server decodes it and stores the original, so placeholders, literal `%` and embedded newlines round-trip. Read needs no transform. Reference: `encodedMessagePointer` in `internal/resources/pro/vpp_invitation/input_builders.go`.

## Error Handling

Use the shared helpers in `internal/common/helpers`:

- `helpers.ResolveTimeout(ctx, value, defaultDuration)` resolves a `framework-timeouts` value to a deadline.
- `helpers.ReconcileOptionalBool` / `ReconcileOptionalInt` / `ReconcileOptionalString` / `Reconcile*Pointer` preserve Terraform null semantics when the API returns zero values.
- `helpers.PreserveStringWhenWireEmpty(wire, current)` is for user-authored strings the classic API echoes as empty after a successful write. See [rule 4a](schema-design.md#server-derived-computed-fields--optionalcomputed-attributes).
- The error classifiers and `helpers.APIErrorDetail` are covered in [§Pro error/retry helpers](#pro-errorretry-helpers).
- Wrap errors with `fmt.Errorf("context: %w", err)` to keep the chain.

### Whitespace Jamf Pro strips on save

Tolerance is opt-in, per field. `helpers.ReconcileOptionalStringTrimmed`, `ReconcileOptionalStringPointerTrimmed`, `PreserveStringWhenWireEmptyTrimmed`, `StringPointerValueOrNullTrimmed` and `StringValueTrimmed` keep `current` when the wire value is `current` with its trailing, or leading and trailing, whitespace removed. Any other server value wins, and interior whitespace is compared exactly.

- Use them only for a field whose write reaches an endpoint verified to trim, and cite the probe. The plain helpers stay strict: on an endpoint that stores verbatim, the tolerance would hide an out-of-band trim.
- **Jamf Pro classic** trims every string field on every endpoint probed, on `POST` and `PUT`. A classic state builder uses the `*Trimmed` forms; a raw `types.StringValue(wire)` there is unprotected.
- **Jamf Pro JSON** trims the name on `/v1/departments`, `/v1/distribution-points`, `/v1/computer-extension-attributes` and `/v1/mobile-device-extension-attributes` only, and stores verbatim elsewhere. The full list is on `wireIsTrimmedCurrent` in `helpers.go`.
- A resource with a classic fallback write (`smart_computer_group`, `smart_mobile_device_group`) trims on that path only.

Without the tolerance, a heredoc's trailing `\n`, or a trailing space carried into state from an object created in the Jamf Pro UI, fails apply with "Provider produced inconsistent result".

## Pro error/retry helpers

These apply to every family, not only Jamf Pro.

**Retry is a consumer concern.** The SDK cannot tell a permanent `409` ("Problem with site ID") from a transient one, so it does not retry 4xx. The transport retries only `429`/`Retry-After`. Everything else surfaces to the resource, which owns any eventual-consistency handling (poll, re-issue, or fire-and-trust; see [§Delete semantics](#delete-semantics-not-found-async-and-propagation-blocked)).

**Pacing defaults to none.** The transport can apply an inter-request delay (`WithMinRequestInterval`). The provider always passes its own value, default **0**, because the interval gates request starts: at the SDK's 100 ms default it would cap the whole provider at 10 requests per second regardless of parallelism. Operators restore pacing with the provider `min_request_interval_ms` attribute or `JAMFPLATFORM_MIN_REQUEST_INTERVAL_MS` (attribute wins). Features that fan out reads bound their own concurrency.

Classifiers in `internal/common/helpers/helpers.go`:

| Helper | Meaning | Use |
|---|---|---|
| `IsNotFoundError(err)` | `404`, and the classic `400 INVALID_ID` | Every `Read` and `Delete` |
| `IsClientError(err)` | Any `4xx` | Separates an accepted-but-misleading `4xx` on a fire-and-trust delete from a `5xx` or transport failure that must surface |
| `IsServerError(err)` | `5xx` | Retry decisions |
| `IsGatewayUnrouted(err)` | The gateway's own `404 page not found`: the request reached no Jamf service | Exclude when clearing state |
| `IsEdgeBlocked(err)` | An HTML error page from a CDN, WAF or IP allowlist | Exclude when clearing state |

**Exclude gateway-unrouted and edge-blocked replies wherever a status decides whether to clear state.** Neither came from a Jamf service. Reading one as "the object is gone" removes a live object from state, and across a whole estate a refresh then empties the state file and `terraform plan` reports a clean set of creates. `IsNotFoundError` already excludes both. A resource classifying a status itself must do so explicitly (`isAcceptedAsyncDelete` in `ebook`). An edge page carries whatever status the edge chose, and CloudFront uses `404` among others, so excluding the gateway's plain-text reply is not enough.

`IsEdgeBlocked` is `errors.Is(err, jamfplatform.ErrUnexpectedResponse)`, so the classification stays the SDK's. The SDK marks an edge page and exempts Jamf Pro's own HTML "Status page" template, so a classic `404` still means deleted. Never widen this to "an HTML `404` is never gone": every `/proclassic` `Read` depends on the opposite.

Tests for either need `internal/testhelpers/gatewaystub`; see [TESTING.md](../TESTING.md).

**Render every API error through `helpers.APIErrorDetail(err)` (`api_error_detail.go`), never `err.Error()`.**

```go
resp.Diagnostics.AddError("Error creating Jamf Pro department", helpers.APIErrorDetail(err))
```

- For a response that came from a CDN, firewall or allowlist, it appends what to do: a standing block reports this host's public IP address (`internal/common/egressip`, cached per process); a gateway failure says it was already retried and to run again.
- For an error a Jamf service produced, it returns `err.Error()` unchanged. So apply it at every call site: which call an edge page lands on depends on the network, not the resource, and creates are hit as readily as reads.
- `internal/conformance/api_error_detail_test.go` fails on an error rendered into the detail of `AddError`, `AddAttributeError`, `diag.NewErrorDiagnostic` or `diag.NewAttributeErrorDiagnostic`: an `.Error()` call on any receiver, or an error-shaped identifier passed to `fmt.Sprintf`, through any concatenation.
- The guard has no per-site exemption. A locally produced error (a decode, a version parse) is wrapped like any other, which costs nothing because `APIErrorDetail` appends nothing to an error the SDK never marked.
- The guard does not cover warnings (`AddWarning`, `diag.NewWarningDiagnostic`). Convert those by hand where they report an API call.
- The one deliberate exception is a create-adoption warning; see [§Create semantics](#create-semantics-a-failed-create-is-not-proof-nothing-was-created).

**Shared retry.** For transient states (`429`, `423 Locked` on in-flight async operations, `409` on a stale `PATCH`/`PUT`), keep retry logic in the resource until three or more resources need it, then extract a shared helper ([§Shared abstractions](package-layout.md#shared-abstractions--when-to-extract)). `device_group`'s propagation-delete retry is the in-resource precedent.
