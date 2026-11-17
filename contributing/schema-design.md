# Schema design

Attribute naming, enums, Sets vs Lists, secrets, `Optional+Computed`, nested blocks and cross-field validation. Part of the [style guide](../STYLE_GUIDE.md).

## Schema Guidelines

- Keep schemas inline and as flat as possible.
- Favor nested attributes (`SingleNestedAttribute`, `SetNestedAttribute`, `ListNestedAttribute`) over blocks.
- **Terraform attribute names are always snake_case**, whatever the upstream API calls the field. Translate at the boundary in input/state builders: API `categoryId` → TF `category_id`, API `osRequirements` → TF `os_requirements`, API `parameter4` → TF `parameter_4`. The one exemption is **RSQL filter selectors** (`filters.FilterModel.Selector`), which pass through to the API verbatim and keep their API-native spelling.

### Attribute names mirror the Jamf Pro admin UI when the wire name is cryptic

Jamf's wire payloads often use short internal names that do not match what an administrator sees: `cache_last_user` is labelled "Create Mobile Account" in the AD binding screen, `mount_style` is "Network Protocol". A user reading the provider docs should not have to translate from the wire spelling back to the label they know.

When the wire name is cryptic or differs materially from the admin-UI label, name the Terraform attribute after the UI label (in snake_case) and translate in the input/state builders. Examples from `jamfplatform_pro_directory_binding`:

| UI label | TF attribute | Wire element |
|----------|--------------|--------------|
| Create Mobile Account | `create_mobile_account` | `cache_last_user` |
| Network Protocol | `network_protocol` | `mount_style` |
| Map UID to attribute | `uid_attribute_mapping` | `uid` |
| Force local home directory on startup disk | `force_local_home_directory` | `local_home` (bool, AD type) |
| Home Location | `home_location` | `local_home` (string, ADmitMac type) |

- When the wire name already matches the UI label reasonably (`encrypt_using_ssl`, `use_unc_path`, `workstation_mode`), leave it alone.
- The `MarkdownDescription` of a renamed attribute leads with the exact UI label in bold quoted form, such as `**"Create Mobile Account"** in the Jamf Pro admin UI.`, so a plan reviewer can match it to the admin screen.
- The wire name does not appear in user-facing text. Record the whole wire mapping as a table in the package doc comment, where a maintainer chasing a field name seen in an API response will find it. It cannot go in a comment beside the attribute, because a schema is built inside a function and [§Go Conventions](package-layout.md#go-conventions) forbids comments there. Reference: `internal/resources/security_cloud/ztna_gateway/resource.go`.
- The rule applies to every product family, not only Jamf Pro.
- Renaming an attribute on a shipped resource is a breaking change. Do not fold it into a feature PR; see [CONTRIBUTING.md](../CONTRIBUTING.md#renaming-a-shipped-attribute).

### Enum values and error codes come from the SDK, not from literals

**If the SDK generates a constant for a value, use the constant.** This applies provider-wide, to two surfaces:

- **Schema value enums.** Build both the `OneOf` validator and the documented value list from the SDK's generated `*Values()` helper, so neither can drift from the other or from the API.
- **Machine-readable error codes** used to translate a failure into a diagnostic. Alias the SDK constant (`codeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled`); never restate the string.

Two shapes are legitimate exceptions:

- **A deliberate subset.** Where a generated set is a superset of what one caller accepts (`EmailMappingTypeValues()` spans every UEM vendor), keep the hand-curated list. Its elements are still SDK constants, not literals.
- **A genuinely absent constant.** Where the SDK documents a set but generates no helper, or carries no constant for a code the wire sends, restate it in the package's `mappings.go` and say why in a comment. The comment must be true of every value beneath it. Jamf Security Cloud's `ApiErrorItemCode` carries no group-specific code, but it does carry the generic `INVALID_FIELD` and `NOT_ENTITLED`, so a package that restates those beside its own codes under "the SDK carries none of these" is wrong.

Review does not catch this reliably, so every package that names an enum value or an error code carries an `enum_literals_test.go` calling `internal/common/enumguard`. The guard parses the package's own source and fails on any string literal the package declares that the SDK's `*Values()` helpers already cover, including one a later SDK release promotes into an enum. It checks declarations, not uses: a literal that is only compared against, returned or passed to an ordinary call is for review to catch.

The guard takes two exemption maps, each with a reason per value:

| Map | Asserts | Checked against the SDK sets |
|---|---|---|
| `Absent` | The SDK carries no constant for this value | Yes. An SDK release that starts generating it fails the test |
| `Ignore` | The value belongs to a different vocabulary that shares the spelling | No |

A cross-vocabulary collision filed under `Absent` reports a spurious promotion. An exemption no literal uses fails, and so does a guard that can assert nothing. What the walker collects, and why, is in the `enumguard` package doc.

References: `internal/resources/pro/ebook/enum_literals_test.go` (plain) and `internal/resources/pro/macos_configuration_profile/enum_literals_test.go` (exemptions with reasons).

### Sets vs Lists

- **Sets** for user-supplied unordered collections where deduplication and order-independent comparison matter (`members`, `raw_component`).
- **Lists** for user-supplied collections whose order is meaningful, and for computed API results that are read-only. Data source attributes returning API data are always lists.

Do not infer the choice from how the admin UI looks. Probe what the server does with order and with repeats, per endpoint: `POST` `["Zebra","Apple"]` and compare the `GET`; write the same entry twice, and two entries that share an id but differ in a secondary field, and `GET` again. Two SDK packages can treat one concept differently.

| Server behaviour | Model |
|---|---|
| Stores the order written | Plain positional `List`. Do not align it, so a reorder made in the admin UI surfaces as drift |
| Re-sorts, entries cannot repeat | `Set`. A `List` fails with "produced inconsistent result after apply" |
| Re-sorts, entries can repeat | `ListNestedAttribute` aligned to the prior list. A `Set` fails with `Duplicate Set Element` on read, and one bad object fails a whole `terraform query` |
| Order carries meaning (positions, joins, parentheses) | `List`, never a `Set` |

Extension-attribute pop-up choices show the first two rows on one concept: the Pro JSON `/v1/computer-extension-attributes` endpoint (and the mobile one) returns choices sorted alphabetically, so they are a `Set`, while ProClassic `/userextensionattributes` keeps the submitted order, so the user EA is a `List`. See `internal/resources/pro/{computer,mobile_device,user}_extension_attribute/`.

**Aligning a re-sorted list.** Align the read to the prior list (the plan on Create/Update, prior state on Read) with `helpers.AlignToPrior`, keyed on the server's sort key and nothing finer, so entries that tie on the key pair first-in, first-out in the order the server keeps them. Use `helpers.AlignToPriorFunc` when the plan can hold an unknown key, such as an id interpolated from a resource created in the same apply. The written order then survives, and a reorder across sort groups plans one update and then plans clean; the probe has to show that update changes nothing stored. Do not use a list-level plan modifier that plans the prior order when the multiset matches (a [rule 1a](#rule-1a-nested-list-elements-and-optional-blocks) collapse), or semantic equality (it never runs at plan time, and on Create the prior value is the plan, whose server-derived names are still Unknown). Reference: `jamfplatform_pro_policy` scripts (sorted by id, then priority) and packages (sorted by name) are aligned; its printers, dock items and directory bindings, and `jamfplatform_pro_inventory_preload_record` `extension_attributes`, are positional.

**Why ordered data cannot be a Set.** Terraform delivers Set elements to the provider in hash order, so anything derived from the index is scrambled, and Set elements correlate plan to state by value, so a positional reconcile (`prev = current[i]`) pairs each server element with an unrelated prior element and fails with `planned set element … does not correlate with any element in actual`. A `ListNestedAttribute` correlates positionally, and the null-preserving `Reconcile*` helpers keep user-omitted fields null. Smart-group `criteria` is the case: `order`, parentheses and `and_or` joins are all positional. See `internal/resources/device_group/`.

**Changing a Set to a List on a shipped resource.** No state rewrite is needed, since both serialise to a JSON array, but an in-place upgrade yields a one-time, self-healing reorder plan. When the List can hold values the Set could not (repeats), bump the schema `Version` and add a pass-through upgrader for the previous version: the bump is what makes a provider still on the old version refuse the newer state instead of failing to decode it. Reference: `jamfplatform_pro_policy` (schema version 3), `jamfplatform_pro_inventory_preload_record` (version 1).

### Plaintext secrets — `WriteOnly` with `_wo_version` rotation companion

A user-supplied plaintext secret (password, API token, shared key) **MUST** be `Optional + Sensitive + WriteOnly`. The plaintext is sent to Jamf on writes and never persisted in Terraform state. `Sensitive` alone is not enough: it redacts CLI output only, and the plaintext still sits in the state file.

Every WriteOnly secret **MUST** carry a sibling `<attr>_wo_version` rotation trigger:

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

WriteOnly values are excluded from Terraform's diff, so changing the password alone plans nothing. Bumping the companion is the only signal that the user wants to rotate; without it, rotating means destroy and recreate. This is HashiCorp's documented `_wo_version` convention.

CRUD wiring:

- Create and Update **MUST** call `req.Config.Get(ctx, &cfg)`. The plaintext lives in `cfg`, not `plan`, because the framework nullifies WriteOnly attributes in the plan.
- Update **MUST** also read prior state and compare `plan.<attr>_wo_version` with `state.<attr>_wo_version`, sending the plaintext only when they differ. Send it on every write only where the server requires the field each time, as `jamfplatform_pro_policy.account_maintenance` does: omitting the password erases it server-side and breaks the next client run.
- State builders **MUST NOT** try to preserve the plaintext across reads. The `_wo_version` companion is an ordinary Optional Int64 and **MUST** round-trip from prior state, since the wire never echoes it.

Related rules:

- **No `*_sha256` or other server-redaction sentinel.** The classic API returns the literal `********************` (20 asterisks) whatever the stored password, so it carries no drift signal, and surfacing it as a Computed sibling invites users to treat it as a real hash. Do not add a Computed `*_sha256` attribute beside a WriteOnly plaintext.
- **`SetNestedAttribute` cannot contain `WriteOnly` children.** The framework refuses to load the schema. If the wire shape is a set of objects carrying a plaintext secret (classic policy `account_maintenance.accounts[].password`), the surrounding attribute **MUST** be a `ListNestedAttribute`. When flattening, reorder wire entries by a stable natural key (username, name) so positional identity survives an unordered server response. Reference: `flattenPolicyAccountMaintenance` in `internal/resources/pro/policy/state_builders.go`.
- **A non-secret write instruction is `WriteOnly` too, without a `_wo_version`.** A field that tells the API how to perform a write, rather than describing a property of the object, is never returned on `GET`. Model it `Optional + WriteOnly` (not `Sensitive`, not `Computed`, no `Default`), read it from `req.Config` in Create/Update, and supply any required wire default in the input builder when config is null. There is nothing to rotate: the instruction rides along with whatever update is already happening, and changing it alone correctly plans nothing. Storing it as a plain `Optional` would put a non-attribute in state. Reference: `manage_existing_data` (`DELETE`/`RETAIN`) on `jamfplatform_pro_computer_extension_attribute`.

### Server-minted secrets — `Computed + Sensitive` with a rotation trigger

`WriteOnly` is for secrets the user supplies. A secret that Jamf Pro generates and returns once is an output, so WriteOnly does not apply. An ephemeral resource is the wrong tool as well: it re-runs `Open()` on every apply, and a generate endpoint with no idempotent read-back would mint a fresh secret each time.

- Model the secret as **`Computed + Sensitive`** (never `Optional`) with `UseStateForUnknown`, so it is sticky across plans. The value is written to state. That is the cost of making a server-minted secret usable by downstream resources and outputs, acceptable only because there is no read-back.
- Drive generation opt-in through an `Optional` trigger attribute, a free-text string or an Int64 in the `_wo_version` idiom. Generate on Create only when the trigger is set; re-mint on Update only when its value changes between state and plan. A null or removed trigger leaves the stored secret alone. Keep the comparison in a small testable helper.
- When the trigger changes, force the secret `Unknown` in `ModifyPlan` so Update can re-mint it. Attribute plan modifiers run before resource `ModifyPlan`, so the override wins over `UseStateForUnknown`.
- **`ModifyPlan` must predict any server-side clearing of the secret.** If a sibling revokes it (an `enabled = false` that also discards the stored credential), null the secret on Read when the server reports it gone, and set it to `types.StringNull()` in `ModifyPlan` whenever that condition is planned. Otherwise the sticky value is planned non-null and applied null, which fails with `Provider produced inconsistent result after apply`.
- A precondition the generation endpoint enforces (rotation requires the integration to be enabled) gets a cross-field plan-time validator that asserts only when the gating attribute is known.
- A data source exposes the secret only where reading it is the data source's purpose, as `jamfplatform_pro_sso_failover_url` does, and marks it `Sensitive`. A list resource never exposes it. `ImportStateVerifyIgnore` lists the secret and the rotation trigger.

Reference: `failover_url` and `regeneration_trigger` in `internal/resources/pro/sso_failover_url/`. `jamf_public_key_rotation` in `internal/resources/pro/pki_venafi/` shows the Int64 trigger and the `ModifyPlan` prediction on a value that is not secret.

### Server-derived computed fields & `Optional+Computed` attributes

Pro endpoints commonly return server-derived values for fields the user did not set: a "no category" sentinel such as `categoryId="-1"` / `categoryName="NONE"`, a default `priority="AFTER"`. The schema, the input builder and the state builder must agree on how such a field is handled, or Terraform fails with `Provider produced inconsistent result after apply`. Reference implementation for rules 1 to 4: `internal/resources/pro/script/`.

#### Rule 1: schema shape

Pick the shape from where the value comes from:

| Field | Shape |
|---|---|
| Optional, and the server fills it in when omitted | `Optional + Computed` with `UseStateForUnknown` (inside a nested list, set or optional block, see [rule 1a](#rule-1a-nested-list-elements-and-optional-blocks)) |
| Read-only server identity that depends on nothing the user sets (`id`, `uuid`, created date) | `Computed` with `UseStateForUnknown` |
| Read-only, derived from a mutable sibling (`category_name` from `category_id`, `site_name` from `site_id`) | Plain `Computed`, **no** `UseStateForUnknown` |
| Read-only, looked up by the server from a sibling this resource sends (`self_service_icon.uri` and `.filename` from `id`) | `Computed` with `planmodifiers.MirrorOfString` |
| Overwritten by the server with another field's value | `Computed`, never sent |

```go
"category_id": schema.StringAttribute{
    Optional: true,
    Computed: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
},
```

**Derived from a mutable sibling.** `UseStateForUnknown` pins the prior name into the plan even when the user changes `category_id`; apply then reads back the new name and Terraform aborts (`.category_name: was "Old", but now "New"`). Without the modifier the field goes Unknown when the source changes. It then shows `(known after apply)` on any plan that touches the source, which is correct. `TestAccResource_ProScript_CategorySwap` in `internal/resources/pro/script/` fails with the modifier present. See also [§Referencing a server-managed catalog by name](#referencing-a-server-managed-catalog-by-name).

**Mirrored from a sibling this resource sets.** For the Self Service icon, the server ignores an authored `uri`/`filename` pair and refuses a filename sent without an id, so neither is `Optional`: an authored value could only match what apply returns by accident. `UseStateForUnknown` and `UseNonNullStateForUnknown` both plan the previous source's value into an apply that moves the source, and with no modifier the attribute is Unknown in every plan that changes anything else. `MirrorOfString(NestedStringSource(block, "id"))` carries the prior value while the source is unchanged in config, and goes Unknown when the source moves or is itself Unknown (the source resource is being replaced in the same apply). Input builders send the source only. Reference: `self_service_icon` in `policy`, `mac_app_store_app` and `mobile_device_app`. The shared unit assertion is `internal/testhelpers/iconmirror`, so a new resource carrying the block adds one line to its schema test.

**Server-forced fields.** Some classic objects overwrite one field with another. Jamf Pro forces `mobile_device_provisioning_profile.display_name` to equal `name`. The collapse depends on serialisation order: the server adopts whichever of the two elements appears last on the wire, and the generated SDK marshals struct fields alphabetically (`<display_name>` before `<name>`), so both land on `name`. Model such a field `Computed`-only and never send it. To probe for it, `POST` two distinct values and compare the `GET`; sending the same value for both hides the collapse. Reference: `internal/resources/pro/mobile_device_provisioning_profile/`.

#### Clear sentinels are per-field

The `-1` "none" sentinel is a convention, not a guarantee. Probe the clear value for every nullable reference field, because it differs between endpoints and between fields on one endpoint:

- `/v3/patch-software-title-configurations` (merge-patch) takes `-1` for site and category and refuses `0` and `""` with 400 `INVALID_ID`.
- On classic endpoints `0` draws a 409. An empty `<site><id></id></site>` and a bare `<site/>` retain the old site. A bare `<category/>` clears a policy's category, but `<category><id></id></category>` draws a 409.

Keep one user-facing sentinel and absorb the differences at the wire layer:

- **Translate.** Map the user's sentinel to the wire clear value in the input builder, and collapse every server "none" echo back to the user-facing sentinel in the state builder. Reference: `internal/resources/pro/patch_software_title/` (`refIDPtr` maps any non-positive id to wire `-1`; `refIDValue` collapses `-1`/`0` echoes to `"-1"`), and `siteIDValue` in `user_initiated_enrollment_settings`, which turns the `"0"` Jamf Pro reports for no site into `"-1"`.
- **Refuse other spellings at plan time** where the wire has one clear value that needs no translation. Left alone, `""` or `0` reaches the API and is dropped (inconsistent result after apply), stored literally, or refused with a 4xx. Reference: `internal/resources/pro/ebook/`.

| Validator (`commonvalidators`) | Accepts | For |
|---|---|---|
| `ClearableReferenceID` | A positive ID or `-1` | The default |
| `ClearableReferenceIDWith` | The above plus extra sentinels | `-2` for the cloud distribution point |
| `ClearableReferenceIDInt64` | Int64 twin of the first | |
| `PositiveReferenceID`, `PositiveReferenceIDInt64` | A positive ID only | A reference Jamf Pro cannot clear (a Self Service icon: replace only) or that clears only by removing the attribute (`webhook.smart_group_id`) |

A reference whose PUT ignores the value on update (`volume_purchasing_notification.site_id`) gets `RequiresReplace`.

**The derived name collapses too.** For the unassigned sentinel the classic GET nondeterministically either echoes `<name>NONE</name>` or omits `<name>`. A state builder that trusts the echo flips `*_name` between `"NONE"` and null across refreshes of the same object, which shows up as an intermittent `ImportStateVerify` diff or inconsistent result. For the sentinel branch (`id` nil or ≤ 0) the derived name **MUST** be null, never the server echo. Use `helpers.DerivedRefName(id, name)` for every Computed `*_name` derived from a reference id, and `scope.FlattenSiteObject` for classic scope blocks. Reference: `siteValues`/`categoryValues` in `internal/resources/pro/patch_software_title/`.

**A classic clear can leave a value the modern endpoint rejects.** Classic and v2 endpoints for one entity write the same record and validate differently. A classic `<category><id>0</id>` clear on a patch software title stores `categoryId` as `"0"`, and `/v2/patch-software-title-configurations` then rejects every write, even one that never mentions `categoryId`, because it re-validates the whole stored object. A resource that writes through both must re-assert the user-facing value in the same modern write. `patch_software_title` now writes only through v3, but a title last written through classic can still carry `"0"`, so `acceptPendingExtensionAttributes` re-sends `categoryId` with the extension-attribute merge-patch.

#### Rule 1a: nested-list elements and optional blocks

`Optional+Computed` scalars inside a `ListNestedAttribute` or `SetNestedAttribute`, and inside an `Optional` `SingleNestedAttribute`, **MUST** use `stringplanmodifier.UseNonNullStateForUnknown()` (and the bool/int siblings), not `UseStateForUnknown`, unless the resource can collapse a plan.

`UseStateForUnknown` copies the prior state value into the plan, including null. For an appended list element, and for every leaf of an optional block that goes from omitted to declared, the prior value is null. When the server then returns a value for that leaf, the apply fails with `Provider produced inconsistent result after apply`. If a `Sensitive` sibling lives on the same element, the error path is redacted to the nearest non-sensitive ancestor and reads `.<parent>: inconsistent values for sensitive attribute`, which blames the wrong attribute. `UseNonNullStateForUnknown` skips the copy when the prior value is null and leaves the plan Unknown.

The non-null variant has one cost. In any plan whose proposed state differs from prior state, the framework marks every Computed attribute with a null configuration Unknown before any modifier runs, and the non-null variant leaves a null prior Unknown. While the plan really changes something that is harmless: the leaf shows `(known after apply)` and settles back to null. It becomes a permanent diff, with `plan -refresh=false` never empty, only when both of these hold:

- **A null-forever leaf.** The leaf stays null in state while its block exists, because Jamf Pro never returns it or the flattener keeps a null (`helpers.ReconcileOptionalString*` turns an empty wire string into null). Most resources have these.
- **A collapse mechanism.** Something restores the change that made the plan differ, so the Unknowns are all that is left. There are exactly two: a plan modifier on an `Optional` or `Required` attribute that plans the prior value in place of the configured one (the CRLF/LF `preferences` equality on `jamfplatform_pro_mobile_device_app`), or a `ModifyPlan` that writes the plan. A modifier that acts only on an Unknown plan value cannot be one: the stock carry modifiers, `UseStateForUnknownInExistingBlock*` and the id-keyed `UseKeyedStateForUnknown*` all return early on a configured value. Semantic equality cannot be one either, because it never runs at plan time.

What to do:

| Resource | Leaves of an optional single-nested block | Schema test |
|---|---|---|
| No collapse mechanism | `UseNonNullStateForUnknown` | `nullprior.AssertNoPlanCollapse(t, schema, ".")` |
| Has a collapse mechanism | `planmodifiers.UseStateForUnknownInExistingBlockString` / `Bool` / `Int64` | |
| Either | | `nullprior.AssertOptionalBlockLeavesPlanUnknown` |

List and set elements use `UseNonNullStateForUnknown` in both cases.

- `AssertNoPlanCollapse` fails on any plan modifier on a configurable attribute that is not proven to act only on Unknown, and on any write to a `Plan` in the package or the module packages it imports.
- `AssertOptionalBlockLeavesPlanUnknown` (`internal/testhelpers/nullprior`) fails on a singleton-block leaf that plans a known value from a null prior.
- The in-existing-block modifiers carry a null prior while the block already exists and leave it Unknown only when the block is new. They plan the carried null where the non-null variant planned Unknown, so a leaf Jamf Pro starts returning when a sibling changes fails the apply (`mobile_device_app` `after_install_button_text`, gated on `make_available_after_install`). Wire-probe every sibling-gated leaf before converting.
- They are not sufficient alone. A plain `Computed` leaf (`category_name`, `site_name`) goes Unknown the same way and needs `planmodifiers.MirrorOfString`, and a leaf of a Set or List element has no block to look up and stays exposed.
- A block that is never null in state when next declared needs neither modifier. `directory_binding` passes `nullprior.ExemptBlocks` with the reason: Jamf Pro populates the typed block whether or not it was sent, and the state builder keeps it. The exemption holds only because `typeBlockConfigValidator` requires the block for its type at plan time and `RequiresReplace` on `type` keeps a type change out of Update. Relax either and the exemption has to go.

References. Without a collapse: `optComputedString` / `optComputedBool` / `optComputedInt` in `internal/resources/pro/policy/resource.go`, and `TestPolicyResource_NothingCollapsesAPlan`. With one: `optComputedString` / `optComputedBool` / `optComputedInt64` in `internal/resources/pro/mobile_device_app/resource.go`. The prestage enrollments and both configuration profiles also have one (a `ModifyPlan` that writes the plan) and use the in-existing-block modifiers.

#### Rule 2: input builders treat Unknown as nil

When the user omits an `Optional+Computed` attribute the plan value is Unknown, not null. `types.String.ValueStringPointer()` returns a pointer to `""` for Unknown, and Jamf Pro often rejects `categoryId: ""` with HTTP 500. Use the helper that nils both null and Unknown:

```go
input := &pro.Script{
    Name:       plan.Name.ValueString(),
    CategoryID: helpers.OptionalStringPointer(plan.CategoryID),
    Priority:   helpers.OptionalStringPointer(plan.Priority),
}
```

Apply `helpers.OptionalStringPointer` to every Optional and Optional+Computed string payload field. Add bool/int variants only when a resource needs them, and match the SDK pointer type in the helper signature so call sites need no casts.

#### Rule 3: refresh state with a GET after Create and Update

Pro PUT responses are routinely lossy: `UpdateScriptV1` returns a `Script` without `categoryName`, which `GetScriptV1` includes. State populated from the PUT response nulls the server-derived fields while `UseStateForUnknown` carried the prior value into the plan, and the apply fails. Create already goes POST → `HrefResponse` → GET; Update must do the same. Keep only the error from the PUT and discard its body.

```go
if _, err := r.client.UpdateScriptV1(updateCtx, plan.ID.ValueString(), buildScriptInput(plan)); err != nil {
    resp.Diagnostics.AddError("Error updating Jamf Pro script", err.Error())
    return
}

got, err := r.client.GetScriptV1(updateCtx, plan.ID.ValueString())
if err != nil {
    resp.Diagnostics.AddError("Error reading updated Jamf Pro script", err.Error())
    return
}
assignScriptResourceModel(&plan, got)
```

#### Rule 4: state builders reconcile against the prior value

Use `helpers.ReconcileOptionalStringPointer(s.X, state.X)` for every Optional and Optional+Computed string field. It keeps the user's prior value, including an explicit empty string, when the API returns nothing. A bare `helpers.StringPointerValueOrNull(s.X)` overwrites Terraform's null/empty distinction on every refresh; use it only for `Computed`-only fields, where there is no user value to reconcile. Do not mix the two across one model's user-settable fields.

#### Rule 4a: `helpers.PreserveStringWhenWireEmpty` for strings the Classic API echoes as empty

Some Classic endpoints return a field as `<elem></elem>` on read even after a successful write of a non-empty value. It is observed on configuration-profile self-service blocks: `self_service_description`, `notification_subject`, `notification_message`, `authorization_password`. `ReconcileOptionalStringPointer` treats the empty echo as "not set" and collapses state to null, and a plan that held a non-empty string then fails the consistency check.

When the attribute shares a block with a `Sensitive` sibling, Terraform masks the error path to the parent block (`.self_service: inconsistent values for sensitive attribute`), which points at the wrong attribute. So the default is: any user-authored string under a block that carries a Sensitive sibling uses `PreserveStringWhenWireEmpty`, not `ReconcileOptionalStringPointer`. Cover it with a unit test that feeds a non-empty configured value and an empty wire echo.

### `SingleNestedAttribute` blocks: Optional-only when the model uses typed-pointer

A nested block modelled as `*StructModel` (a typed pointer to a struct with `tfsdk:` tags) cannot be `Optional+Computed`. The framework decodes an absent-but-Computed block as Unknown, a typed pointer cannot represent Unknown, and apply fails with:

> `Received unknown value, however the target type cannot handle unknown values. Use the corresponding`types`package type or a custom type that handles unknown values.`

Two ways out:

1. **Keep the block `Optional`-only.** This is the default. Inner fields can still be `Optional+Computed`, so the server can populate defaults the user omitted. Tell users to supply the block, even empty (`<type>_block = {}`), to manage that configuration. Reference: `internal/resources/pro/directory_binding/`, five per-type blocks.
2. **Switch the model field to `types.Object`** with an `attrTypes` map. Worth it only when the resource needs the framework to represent the block as Unknown; most do not.

**With an Optional-only block, the flatten MUST gate block population on the model being written to state, never on a separate prior-state reference.** The server returns a populated block in every `GET`, and populating it when the plan said null breaks the framework's consistency contract.

- **Create / Read:** the state builder mutates a single model, and gating on that model is correct.
- **Update:** the target is the new plan. A state builder that takes a separate prior `state` and gates on `state.X != nil` fails when the user removes a previously managed block: the plan pointer is nil, the prior state still holds the block, the flatten repopulates it, and apply fails with `was null, but now cty.ObjectVal(…)`. With a `Sensitive` or `WriteOnly` field in the block the same failure reads `inconsistent values for sensitive attribute`. Pass the target model itself, or capture presence before reassigning (`manageX := plan.X != nil`) and gate on that.

Reference: `internal/resources/pro/policy/` passes a single model to `assignPolicyResourceModel` and gates on it; the `*_prestage_enrollment` resources capture `manageX := plan.X != nil` up front. Only an apply that removes the block exercises this, so it needs an acceptance step; see [TESTING.md](../TESTING.md).

#### Computed nested collections

The same failure applies to a `Computed` nested collection (`ListNestedAttribute`, `SetNestedAttribute`) or nested object. A Computed collection is Unknown at plan time, and a Go typed slice (`[]FooModel`) or `*StructModel` cannot carry Unknown. It fails only under a real plan and apply, so unit tests, lint, vet and `make generate` do not show it.

Model a Computed collection as `types.List` / `types.Set` with an `attr.Type` ObjectType, and build it with `types.ListValueFrom(ctx, objType, slice)` so it returns a known, possibly empty, value. The split is by `Computed`, not by depth:

- User-authored `Optional`/`Required` collections are never Unknown, so Go typed slices are fine.
- Computed scalars inside a typed-slice element are fine.
- Only Computed nested collections and objects need the `types.X` form.

Reference: `internal/resources/pro/licensed_software/`. Optional `software_definitions` and `licenses` are `[]Model`; Computed `computers` and `licenses[].attachments` are `types.List`.

### Asymmetric server normalisation on `type`-style discriminator fields

Some classic endpoints accept a legacy product name on write and return the modern one on read. For PowerBroker directory bindings, `/directorybindings` rejects a create with `type="PowerBroker Identity Services"` (HTTP 409 "Problem with directory binding type") and accepts only `type="Likewise"`, while every read returns `type="PowerBroker Identity Services"`. Put the alias in state and users see drift; send the modern name and Create fails.

Translate one way at the input boundary, so users see the name the server reads back and the alias stays an implementation detail. Keep the mapping in `helpers.go`, route the field through it in `input_builders.go`, and cover it with a unit test (input X → wire Y):

```go
const typePowerBrokerCreateAlias = "Likewise"

func mapType(tfType string) string {
    if tfType == typePowerBroker {
        return typePowerBrokerCreateAlias
    }
    return tfType
}
```

Reference: `mapType` and `typePowerBrokerCreateAlias` in `internal/resources/pro/directory_binding/helpers.go`.

#### Plan-modifier rewrites are NOT a valid option for `Required` attributes

The pattern above works because the swap happens inside the input builder, so the value in the plan equals the value in config. The framework enforces `plan == config` for every attribute that is not `Computed`. A `planmodifier.String` that rewrites `req.ConfigValue` on a `Required` attribute, for example case-folding `Individual And Institutional` to `Individual and Institutional`, fails with `Provider produced invalid plan … planned value … does not match config value`.

The valid options:

1. **Strict wire-form acceptance.** `stringvalidator.OneOf` accepts only the canonical wire spellings, and a wrong spelling is a validation error at plan time. Prefer this for simple discriminator fields.
2. **`Optional + Computed` with a plan-modifier rewrite.** It works, but the user can now omit the attribute. Worth it only when the canonicalisation is more than a case-fold, such as an alias that round-trips.

Do not pair `Required: true` with `OneOfCaseInsensitive` and a canonicalising plan modifier: the validator passes and the plan is rejected.

References: `type` in `internal/resources/pro/directory_binding/resource.go` (Required + `OneOf`, no canonicalising modifier; its `RequiresReplace` is unrelated) and `key_type` in `internal/resources/pro/disk_encryption_configuration/resource.go` (accepts only `Individual and Institutional`, lowercase `and`).

### Cross-field validation

Put a cross-field rule as close to the offending attribute as it can go, and attach its error to an attribute path.

| Rule shape | Use |
|---|---|
| A and B are paired: setting either requires the other, whatever the value | `boolvalidator.AlsoRequires` / `stringvalidator.AlsoRequires` / `int64validator.AlsoRequires` on the attribute |
| A and B are mutually exclusive, whatever the value | `boolvalidator.ConflictsWith` / `stringvalidator.ConflictsWith` / `int64validator.ConflictsWith` on the attribute |
| Enum membership | `stringvalidator.OneOf(values...)` on the attribute |
| Required or forbidden only when one attribute has a specific value, and the rule belongs to that attribute | Custom `validator.Bool` / `validator.String` in the package's `validators.go` |
| A discriminator decides which of several blocks or attributes may be set, or the rule has no single owning attribute | Custom `resource.ConfigValidator` in `validators.go`, returned from `ConfigValidators` |
| Exactly one of, at least one of, or required together, across top-level siblings | `resourcevalidator` / `datasourcevalidator` / `actionvalidator` (`ExactlyOneOf`, `AtLeastOneOf`, `Conflicting`, `RequiredTogether`) from `ConfigValidators` |

- The off-the-shelf `AlsoRequires` and `ConflictsWith` fire whenever the validated attribute is known: `true` or `false`, `"foo"` or `""`. They do not inspect the value. Write a custom validator only when the rule depends on a specific value; using one where an off-the-shelf validator fits is a review reject.
- Path syntax: `path.MatchRoot("other_attr")` for a root-level sibling; `path.MatchRelative().AtParent().AtName("other_attr")` for a sibling under the same nested object.
- A plan modifier cannot enforce a cross-field rule by overriding the configured value. The framework rejects it ("planned value … does not match config value").

References:

| Shape | Where |
|---|---|
| Paired toggles, `AlsoRequires` | `internal/resources/blueprints/blueprint/components/math_settings.go`; `override_buildings` / `override_departments` in `internal/resources/pro/network_segment/resource.go`; `internal/actions/device/erase.go` |
| Mixed `AlsoRequires`, `ConflictsWith`, `RegexMatches`, `Between` | `internal/resources/blueprints/blueprint/components/software_update.go` |
| Attribute-level, fires only when the bool is `true` | `AllFlagConflictsWith` in `internal/common/scope/validators.go` |
| Attribute-level, value-discriminated companions | `internal/resources/pro/sso_settings/validators.go` (`configuration_type`, `metadata_source`, `setup_type`) |
| Attribute-level on the offending child | `requiresAccountCollection` in `internal/resources/pro/computer_inventory_collection_settings/validators.go`: fires when a sub-option is `true` and its parent `collect_local_user_accounts` is `false`, the combination the server silently coerces. A sub-option left unset already resolves to the server's `false` |
| `ConfigValidator`, discriminator against blocks | `typeBlockConfigValidator` in `internal/resources/pro/directory_binding/validators.go` |
| `resourcevalidator.ExactlyOneOf` and `RequiredTogether` | `internal/resources/security_cloud/uem_connect/resource.go` |

Authoring a custom validator:

- Implement it on a struct in `validators.go`.
- Return without error when the validated value is null or unknown, and when it does not match the discriminator (`false` for a true-only rule).
- Read companions with `req.Config.GetAttribute(ctx, path, &target)`.
- Attach the error to the companion's path, not the discriminator's, with `resp.Diagnostics.AddAttributeError(companionPath, summary, detail)`. That is where the user needs to look.

#### Config-time validators MUST defer on unknown values — for EVERY attribute they read, not just the discriminator

Config validation (`ValidateConfig`, any `ConfigValidator`, a schema `Validate{String,…}`) runs with Unknown for anything sourced from a variable, `for_each`, `count` or another resource. Unknown means "not resolvable yet", not "missing". A validator that errors on Unknown makes the resource usable only from hard-coded literals and breaks `terraform validate` and `plan` for every reusable module.

- **Skip on Unknown before treating absence as an error.** A required-when check must return when the dependent attribute is Unknown and error only when it is null. Guarding the discriminator is not enough; guard the companion too.
- **Do not let a Go decode hide Unknown.** `req.Config.Get(ctx, &model)` collapses Unknown to a zero value: a nested block becomes a nil pointer, a list or set has `len() == 0`, a custom "is set" helper returns false. So `if block == nil`, `if len(slice) == 0` and `if !isSet(x)` all raise a false error on Unknown. Read the dependent attribute as its typed value (`types.Object`, `types.List`, `types.String`) with `GetAttribute` and branch on `IsUnknown()` and `IsNull()`.
- **Forbidden-when checks are safe**, because they fire on presence and Unknown read as absent only defers. Required-when checks are the trap.
- Acceptance tests use literal HCL and cannot catch this. Each such validator needs a unit test with an Unknown companion; see [TESTING.md](../TESTING.md#validators-that-read-another-attribute).

Reference body: `internal/resources/pro/ibeacon/validators.go`, which validates only when both values are known.

### Attribute defaults do not work inside `SetNestedAttribute`

A `Default` (`booldefault.StaticBool` and its siblings) on an attribute nested inside a `SetNestedAttribute` overrides a value the configuration set explicitly, which leaves a permanent diff. On `security_cloud/dns_hostname_mappings`, with `Optional + Computed + booldefault.StaticBool(false)`, a configuration saying `connect_to_ztna = true` planned as `false` on every plan. It reproduces with a single element, so it is not a set-element correlation problem.

- Prefer `Required` for such an attribute. Where the admin UI shows the control on every row, every element has a value to state.
- An `Optional` attribute inside a `SetNestedAttribute` with neither `Computed` nor `Default` is unaffected.
- No unit test of the builders catches this. Pin the shape in `schema_test.go`: assert `Required` and `Default == nil`, with the reason.

### Referencing a server-managed catalog by name

When a resource references an object in a catalog the server publishes and the user never creates, expose it by name and keep the ID `Computed`, so the user does not have to supply an opaque server ID.

- `<thing>_name` is `Required`. `<thing>_id` is plain `Computed` with no `UseStateForUnknown`, because it is derived from the mutable name and must recompute when the name changes (see [rule 1](#rule-1-schema-shape)).
- **Resolve name → ID at apply**, in Create and Update. A miss is a clear "not in catalog" error. Use the SDK's `Resolve…IDByName` where one exists; otherwise list the catalog through the service's own filter and decide the match in the provider.
- **Read and import reverse-resolve ID → name** from the catalog, since the object's `GET` returns only the ID. Do not echo the configured name back from state: on import there is no prior config, so the name comes out empty and `ImportStateVerify` fails. Make the reverse-resolve best-effort, keeping the existing state value on a transient catalog error.
- **Match exactly, in the provider.** A case-insensitive match would accept `"jamf composer"`, store it, reverse-resolve to `"Jamf Composer"` and diff forever. Jamf Pro's RSQL `name==` / `titleName==` filter is case-insensitive and treats `*` as a glob, on both the App Installer catalog and its deployments, so the filter only narrows the request. Decide the match on byte equality, and treat two objects sharing one exact name as an ambiguity error, not an arbitrary pick.
- **Plan-time name validation is appropriate here**, unlike for a sibling resource, because a server catalog is never created in the same apply. Check best-effort against a live list or `GET`: warn on a transport error, error on a genuine miss, skip null and unknown. Reference: `validateAppStoreLocale` in `internal/resources/pro/app_request_settings/validators.go`.

Reference: `jamfplatform_pro_app_installer.app_title_name` → Computed `app_title_id`, resolved by `internal/resources/pro/app_installer/name_lookup.go`.
