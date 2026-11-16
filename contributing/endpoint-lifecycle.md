# Endpoint adoption, migration and deprecation

Adopting new endpoints, migrating off deprecated ones, the 90-day schema deprecation window, tracking and audit. Part of the [style guide](../STYLE_GUIDE.md).

## Endpoint adoption & migration policy

**Scope: Jamf Pro resources only** (those backed by `jamfplatform-go-sdk/jamfplatform/pro/` or `proclassic/`). Pro APIs versionize endpoints (V1 / V2 / V3 / ...), can be deprecated by Jamf, and run on customer-provisioned Jamf Pro tenants with their own version drift. This policy governs when Pro resources move between endpoint versions. **All version transitions follow the same rules** — V1→V2, V2→V3, V3→V4, and so on. References to "V1" and "V2" below are generic placeholders for "current version N" and "new version N+1".

**Out of scope: Platform Services SDK packages** — `blueprints/`, `compliancebenchmarks/`, `devicegroups/`, `devices/`, `deviceactions/`, `ddmreport/`. These are continuously-deployed Jamf Platform microservices, not customer-versioned Jamf Pro endpoints. Always track the latest stable function in the SDK; no deprecation buffer, no quarterly audit, no annotation block required. When a Platform Services SDK function is updated (e.g., a new V2 added), migrate the corresponding resource opportunistically — typically as part of the SDK dependency bump that introduces it.

### Platform Services *Terraform schema* deprecation — 90-day window

The endpoint-version policy above governs *SDK/endpoint* moves and does not cover **Terraform schema** changes. When a Platform Services resource (`blueprints/*`, `cbengine/*`, `device_group`, …) must reshape its Terraform surface in a backward-incompatible way (rename/retype/supersede an attribute), ship it **additively first**, then remove after a short window:

1. **Add** the new attribute; keep the old one working.
2. **Mark the old attribute `Deprecated`** with a user-facing `DeprecationMessage` that names the replacement **and the earliest removal date**.
3. **Annotate it in code** with a greppable marker so removals can be batched later:
   ```go
   // PLATFORM-DEPRECATED remove-after=YYYY-MM-DD replaced-by=<new_attr> — <one-line reason>
   ```
4. **Window: 90 days** from the release that ships the deprecation (Platform microservices move fast and users track the latest — no Pro-style 12-month buffer). On or after `remove-after`, the field is eligible for **batch removal**: `grep -rn "PLATFORM-DEPRECATED remove-after=" internal/resources/`, drop every field whose date has passed, and land the removal with the schema `Version` bump + `UpgradeState` that migrates old state (see [[feedback-no-state-upgraders]] — published state exists, so removals are breaking and need an upgrader).

This section covers Platform Services resources. Jamf Pro, Jamf Security Cloud and Jamf Account schema changes follow [§Jamf Pro Terraform schema deprecation](#jamf-pro-terraform-schema-deprecation--90-day-window) below, which is the same procedure with its own marker.

### Jamf Pro Terraform schema deprecation — 90-day window

When a Jamf Pro Terraform attribute must be renamed, moved, retyped or dropped, give users 90 days of notice. The rule applies to every `jamfplatform_pro_*` construct and to the Security Cloud, Jamf Account and AI Governance families. It governs the **Terraform surface**. A Jamf endpoint moving between versions is a separate matter, covered by the migration timeline further down.

1. **Add first.** Ship the replacement attribute in the same release that deprecates the old one, and keep the old one working. Nothing breaks until the removal release.
2. **Mark the old attribute `Deprecated`** with a user-facing `DeprecationMessage` that names the replacement and the earliest removal date, for example ``Deprecated: use `script_assignments` instead. The provider will not remove `scripts` before 2027-02-15.``
3. **Annotate it in code** with one marker comment per deprecation, above the constant that holds the message:
   ```go
   // PRO-DEPRECATED remove-after=YYYY-MM-DD replaced-by=<new_attr> — <one-line reason>
   ```
   `PLATFORM-DEPRECATED` is the same marker for the Platform Services rule above, and both are read by the same tool. The line must begin with the marker and carry both `remove-after=` (a calendar date) and `replaced-by=` (an attribute path, or `none` when the attribute goes with no successor). The reason after the dash is optional. `internal/conformance` fails the build on a marker it cannot parse. Test files are not scanned.
4. **Window: 90 days** from the release that ships the deprecation. The date in the message and the marker is that release date plus 90 days. A deprecation message is not rendered by `tfplugindocs`, so the registry page says nothing: put a `~> **Deprecated.** ...` callout at the top of the attribute's `MarkdownDescription` as well, the way `internal/resources/pro/activation_code/deprecation.go` does for a whole construct.
5. **Remove in a batch.** On or after `remove-after` the attribute is eligible for removal. The removal PR deletes the attribute, its marker and its callout, bumps the schema `Version`, and adds an `UpgradeState` step that drops the attribute from prior state, because a key gone from the schema but still in stored state fails the decode. Published state exists, so removal is breaking and needs the upgrader.

**An attribute that changes type cannot be deprecated under its own name.** A schema is a map from name to type, so `scripts` cannot be an object in the old shape and a set in the new one in the same release. Give the replacement a new name, keep it permanently, and do not plan a second release to rename it back. The alternative, replacing the type under the same name, makes every existing configuration fail at `terraform validate` with a type error that mentions neither a migration nor a deprecation, and no upgrader can help, since an upgrader rewrites state and never configuration. That outcome is the reason this section exists.

**Accepted exception: a Set becoming a List of the same element type, in place.** The HCL for both is a bracketed list of the same objects, so every existing configuration still validates and plans; only expressions that relied on set semantics (set functions such as `setunion`, or `==` against a set value) change meaning, and the release notes must say so. Smart-group `criteria` on `jamfplatform_device_group` set the precedent, and `jamfplatform_pro_policy` and `jamfplatform_pro_inventory_preload_record` followed it for the collections Jamf Pro lets repeat (issue #568). It is not a licence for any other type change: an object becoming a list, or a list of one element type becoming a list of another, still needs a new name.

**Reads and writes while both exist.** The old and new attributes are both Optional, and a `ConflictsWith` validator forbids setting both. Read populates whichever one state already manages, so a configuration that uses the old shape keeps planning clean. Config generation (`terraform query -generate-config-out`) and import emit only the new attribute, so a deprecated attribute is never written into new configuration.

**Tracking.** Every marker gets a tracking issue labelled `deprecation-removal`. Nobody opens it at deprecation time. `.github/workflows/deprecation-sweep.yml` opens one per marker whose date has arrived, runs weekly, and finds an existing issue by its exact title across open and closed issues, so an issue closed without the removal happening is not refiled. `make deprecation-sweep` prints the same list locally (`TODAY=YYYY-MM-DD` looks ahead). This is distinct from the `deprecation-migration` label below, which tracks an endpoint Jamf is withdrawing and carries dates on Project #2.

**ProClassic is unversioned**: `jamfplatform/proclassic/` functions do not carry V1/V2 suffixes. Migration timing (below) does not apply to ProClassic-backed resources. Annotation block is still recommended for documenting which SDK function is in use, but `Status:` simply tracks "current" against the SDK release in use.

### SDK side-by-side dependency

The buffered migration timeline below assumes the SDK exposes both the deprecated version (N) and the new version (N+1) simultaneously during the deprecation window. **This applies to every version transition** — V1→V2, V2→V3, V3→V4, etc. — not just V1→V2.

The SDK does not currently retain side-by-side versioned functions on regeneration (the upstream generator change requested in [jamfplatform-go-sdk#19](https://github.com/jamf/jamfplatform-go-sdk/issues/19) closed without merging). Practical consequences:

- When the SDK exposes both versions of an endpoint: follow the buffer policy below.
- When the SDK only exposes the new version on regeneration: migration is **SDK-bump-driven** — migrate at the SDK bump or pin the SDK to the prior release (only as a temporary workaround). Document the constraint in the resource's annotation block.
- When a Jamf deprecation is announced for an endpoint we use (at any version pair), file an issue against `jamfplatform-go-sdk` requesting retention of the deprecated version through our migration window.

### Deprecated with no generated successor

The case above assumes the successor reaches us. Sometimes it does not: the SDK marks version N deprecated while generating **no** client for N+1, because N+1 is absent from the bundled OpenAPI spec even though a live tenant serves it. `golangci-lint` runs `staticcheck` by default and `SA1019` is an error, so this combination turns a green build red with no migration available.

When that happens:

1. **Verify the successor on the wire before concluding anything.** The bundled spec is not authoritative about what a tenant serves — probe the real endpoint and record the status code, the presence or absence of a `Deprecation` response header, and whether the query contract (filters, sections, paging) is unchanged. A spec that deprecates every version of a surface with no replacement present is a stale-bundle symptom, not Jamf's intent.
2. **Raise it upstream against `jamfplatform-go-sdk`** with that wire evidence, and reference the issue from every suppression.
3. **Open the provider-side migration tracking issue** within the usual 30 days (see the timeline below). Being blocked on the SDK does not pause the clock — the deadline is set by Jamf's removal date, not by our dependency.
4. **Suppress `SA1019` at the call site**, never repo-wide, with `//nolint:staticcheck // SA1019: <why> — see <where the reasoning lives>`. Keep the full reasoning in one place per package (a package doc or a note beside the shared type) and have the per-line comments point at it.
5. **Do not route around the deprecation with a worse endpoint.** An undeprecated alternative that loses server-side filtering, paging, or a needed field is usually the worse trade — a tracked deprecation beats an unbounded per-call cost. If an alternative is rejected, say so and say why in the annotation, so the next maintainer does not re-litigate it.
6. **Record the status** in the resource's annotation block using the `Status: deprecated by Jamf …` form below, naming the successor version and the blocking issue.

Suppression is a holding position with a deadline attached, not a resolution. The hard floor in the timeline below still applies.

### Adoption (new endpoints)

| Scenario | Action |
|----------|--------|
| Building a brand-new resource, SDK exposes V1 only | Use V1. |
| Building a brand-new resource, SDK exposes V2 (V1 still current) | **Use V2.** Default to latest stable on new resources — no migration cost. |
| Building a brand-new resource, SDK exposes V2 marked beta/EAP | Use V1. V2 only if V1 is unworkable; document the choice in the resource. |
| Existing **shipped** resource on V1, new V2 lands (V1 not deprecated) | **Do not migrate.** Wait for deprecation, fast-track trigger, or material feature gap. |
| Existing shipped resource on V1, V2 only adds optional new fields | Migrate opportunistically, e.g., when next touching the resource. Not urgent. |
| Existing shipped resource on V1, V2 fixes an issue meeting fast-track criteria | **Fast-track migrate** — see below. |

Rationale: migrating shipped resources costs schema/state-upgrader churn. Don't pay it without reason. New resources have no migration cost.

### Deprecation migration timeline

Jamf's typical window: ~12 months from deprecation announcement to removal.

| Trigger | Action |
|---------|--------|
| Jamf marks V1 deprecated | Open a migration tracking issue within **30 days**. Schedule the work. |
| **6 months** after deprecation announcement (soft default) | Migration **should be merged**. Soft target. |
| **3 months before announced removal date** (hard floor) | Migration **must be merged**. Hard floor — no shipped resource may sit on a deprecated endpoint inside this window. If migration is not yet shipped, it becomes top priority and bumps the buildout roadmap. |
| Removal date | All migrations must already be shipped. Anything not migrated is a regression. |

The 6-month soft default and 3-month hard floor work as a pair: 6-month catches the common case, 3-month catches edge cases (Jamf shortens the window, a migration slips, a deprecation announcement was missed).

### Fast-track exception

Skip the 6-month buffer and migrate as soon as feasible when V2 includes one of:

- **Security fix** (auth, scope, data exposure).
- **Data integrity** issue (V1 returns wrong values, silent truncation, race conditions).
- **Breaking bug** affecting a documented common use case where no workaround exists.
- **Critical feature** required by a user-reported issue with no viable workaround.

Maintainer call, documented in the migration PR description.

### Slow-track exception

When migration would require a **breaking Terraform schema change**, it ships in a minor release like any other breaking change, behind the 90-day schema deprecation below. It does not wait for a major version. Where the 90 days would overrun the hard deadline above, start the deprecation early enough that they do not, and record the plan in the resource's `crud.go` annotation (see below).

### Tracking — the migration issue

Every deprecation gets exactly one tracking issue, opened from the **Deprecation migration** issue template (`.github/ISSUE_TEMPLATE/deprecation_migration.md`). The conventions below are mandatory — they are what makes the set of open migrations greppable and their deadlines visible.

**Label**: `deprecation-migration`, always. (A Terraform attribute removal is tracked separately, under `deprecation-removal`; see [§Jamf Pro Terraform schema deprecation](#jamf-pro-terraform-schema-deprecation--90-day-window).) Add `blocked-upstream` while the SDK exposes no successor, and remove it when the successor ships. No other label substitutes: `grep`ping the label is how the quarterly audit finds outstanding migrations.

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

### Tracking — in-code annotation (Pro resources only)

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

### Quarterly audit cadence (Pro resources only)

Once per quarter, a maintainer:

1. Scans Jamf Pro release notes and the `jamfplatform-go-sdk` changelog for new Pro endpoint versions and deprecations (Pro / ProClassic only — Platform Services microservices are out of scope).
2. Greps the repo for endpoint annotations: `grep -rA5 "SDK endpoints used:" internal/resources/pro/`.
3. Reconciles each annotation against current Jamf + SDK state.
4. Updates `Last reviewed YYYY-MM-DD` on every annotation that's still current.
5. Opens migration tracking issues for any newly-deprecated Pro endpoints, following [§Tracking — the migration issue](#tracking--the-migration-issue).
6. Reviews every open `deprecation-migration` issue — `gh issue list --label deprecation-migration` — and checks each one's `Migrate By` date on Project #2 against the current window, recomputing it if Jamf has since published a removal date.
7. Flags any annotation whose `Last reviewed` date is older than 120 days (means a prior audit was skipped).

The `Last reviewed` date is the drift detector — any annotation more than a quarter stale signals the audit hasn't happened.
