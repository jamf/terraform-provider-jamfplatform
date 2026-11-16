# Endpoint adoption, migration and deprecation

Adopting new endpoints, migrating off deprecated ones, the 90-day schema deprecation window, tracking and audit. Part of the [style guide](../STYLE_GUIDE.md).

What users are promised about deprecations and breaking changes is stated in the registry guide, [Versioning, deprecations and supported versions](../templates/guides/versioning-and-support.md). This file is how the provider keeps that promise. If the two disagree, fix one of them in the same PR.

## Endpoint adoption & migration policy

**In scope: Jamf Pro resources**, those backed by the SDK's `pro/` or `proclassic/` package. The Jamf Pro API versions its endpoints (V1, V2, V3, …), Jamf deprecates them, and tenants run different Jamf Pro versions. Every version transition follows the same rules; "V1" and "V2" below stand for "current version N" and "new version N+1".

**ProClassic is unversioned.** `proclassic/` functions carry no version suffix, so the migration timing below does not apply to a ProClassic-backed resource until Jamf deprecates the endpoint itself. It still carries the annotation block, with `Status: current`.

**Out of scope: every other SDK package**: `blueprints/`, `compliancebenchmarks/`, `devicegroups/`, `devices/`, `deviceactions/`, `ddmreport/`, `securitycloud/`, `account/`, `aigovernance/`. These are continuously deployed services with no tenant version. Track the latest stable SDK function: no deprecation buffer, no quarterly audit, no annotation block. When the SDK gains a newer function, migrate the resource opportunistically, typically in the SDK bump that introduces it.

All of the above is about *endpoints*. A change to the **Terraform schema** is governed by the next section, in every family.

### Jamf Pro Terraform schema deprecation — 90-day window

When a Terraform attribute must be renamed, moved, retyped or dropped, users get 90 days of notice. The rule applies to every family. Only the marker differs:

| Family | Marker |
|---|---|
| Jamf Pro, Jamf Security Cloud, Jamf Account, Jamf AI Governance | `PRO-DEPRECATED` |
| Platform Services (`blueprints/*`, `cbengine/*`, `device_group`, …) | `PLATFORM-DEPRECATED` |

1. **Add first.** Ship the replacement attribute in the same release that deprecates the old one, and keep the old one working. Nothing breaks until the removal release.
2. **Mark the old attribute `Deprecated`** with a `DeprecationMessage` that names the replacement and the earliest removal date, for example ``Deprecated: use `script_assignments` instead. The provider will not remove `scripts` before 2027-02-15.``
3. **Annotate it in code** with one marker comment per deprecation, above the constant that holds the message:
   ```go
   // PRO-DEPRECATED remove-after=YYYY-MM-DD replaced-by=<new_attr> — <one-line reason>
   ```
   The line must begin with the marker and carry both `remove-after=` (a calendar date) and `replaced-by=` (an attribute path, or `none` when nothing replaces it). The reason after the dash is optional. `internal/common/deprecations` is the single parser for both markers, and `internal/conformance` fails the build on a marker it cannot parse. Test files are not scanned.
4. **Window: 90 days** from the release that ships the deprecation. The date in the message and in the marker is that release date plus 90 days. `tfplugindocs` does not render a deprecation message, so also open the attribute's `MarkdownDescription` with a `~> **Deprecated.** ...` callout, the way `internal/resources/pro/policy/deprecations.go` does.
5. **Remove in a batch.** On or after `remove-after` the attribute is eligible for removal. The removal PR deletes the attribute, its marker and its callout, bumps the schema `Version`, and adds an `UpgradeState` step that drops the attribute from prior state. Published state exists, and a key that is gone from the schema but still in stored state fails the decode, so the upgrader is not optional. The removal ships in a minor release.

**An attribute that changes type cannot be deprecated under its own name.** A schema maps a name to one type, so `scripts` cannot be an object and a set in the same release. Give the replacement a new name, keep it permanently, and do not plan a later release to rename it back. Replacing the type under the same name makes every existing configuration fail `terraform validate` with a type error that mentions neither a migration nor a deprecation, and no upgrader can help, because an upgrader rewrites state and never configuration.

**Accepted exception: a Set becoming a List of the same element type, in place.** The HCL for both is a bracketed list of the same objects, so every existing configuration still validates and plans. Only expressions that relied on set semantics change meaning (set functions such as `setunion`, or `==` against a set value), and the release notes must say so. It is not a licence for any other type change: an object becoming a list, or a list of one element type becoming a list of another, still needs a new name.

**Reads and writes while both exist.** The old and new attributes are both Optional, and a `ConflictsWith` validator forbids setting both. Read populates whichever one state already manages, so a configuration on the old shape keeps planning clean. Config generation (`terraform query -generate-config-out`) and import emit only the new attribute.

**Tracking.** Every marker gets a tracking issue labelled `deprecation-removal`. Nobody opens it by hand. `.github/workflows/deprecation-sweep.yml` runs weekly and opens one per marker whose date has arrived. It finds an existing issue by exact title across open and closed issues, so an issue closed without the removal happening is not refiled. `make deprecation-sweep` prints the same list locally (`TODAY=YYYY-MM-DD` looks ahead). This label is distinct from `deprecation-migration`, which tracks an endpoint Jamf is withdrawing.

### Platform Services *Terraform schema* deprecation — 90-day window

The same procedure as [§Jamf Pro Terraform schema deprecation](#jamf-pro-terraform-schema-deprecation--90-day-window), with the `PLATFORM-DEPRECATED` marker. The endpoint rule for Platform Services ("always track latest, no buffer") does not shorten it: a backward-incompatible change to a Platform Services resource's Terraform surface still ships additively and waits the 90 days.

### SDK side-by-side dependency

The migration timeline assumes the SDK exposes the deprecated version and its successor at the same time. The SDK generator does not retain the old function on regeneration, so:

- **Both versions exposed:** follow the timeline.
- **Only the new version exposed after regeneration:** the migration is driven by the SDK bump. Migrate in that bump. Pinning the SDK to the prior release is a temporary workaround only. Record the constraint in the resource's annotation block.
- **Jamf announces a deprecation for an endpoint in use:** file an issue against `jamfplatform-go-sdk` asking it to retain the deprecated version through the migration window.

### Deprecated with no generated successor

Sometimes the SDK marks version N deprecated while generating no client for N+1, because N+1 is absent from the bundled OpenAPI spec even though tenants serve it. `staticcheck`'s `SA1019` is a lint error, so the build goes red with no migration available.

1. **Verify the successor on the wire first.** The bundled spec is not authoritative about what a tenant serves. Probe the real endpoint and record the status code, whether a `Deprecation` response header is present, and whether the query contract (filters, sections, paging) is unchanged. A spec that deprecates every version of a surface with no replacement is a stale bundle, not Jamf's intent.
2. **Raise it against `jamfplatform-go-sdk`** with that evidence, and reference the issue from every suppression.
3. **Open the migration tracking issue** within the usual 30 days. Being blocked on the SDK does not pause the clock: the deadline is set by Jamf's removal date.
4. **Suppress `SA1019` at the call site**, never repo-wide: `//nolint:staticcheck // SA1019: <why> — see <where the reasoning lives>`. Keep the full reasoning in one place per package and have the per-line comments point at it.
5. **Do not route around the deprecation with a worse endpoint.** An undeprecated alternative that loses server-side filtering, paging or a needed field is usually the worse trade. If an alternative is rejected, say so and why in the annotation.
6. **Record the status** in the annotation block with the `Status: deprecated by Jamf …` form, naming the successor version and the blocking issue.

Suppression is a holding position with a deadline. The hard floor in the timeline still applies.

### Deprecated with no successor at all

A different case from the one above: Jamf deprecates an endpoint and publishes nothing that can replace it. Two packages are in this state and show the two outcomes.

- **The construct cannot outlive the endpoint** (`internal/resources/pro/activation_code/`). The Classic read has no successor, so there is nothing to migrate to. The resource and the data source each carry a whole-schema `DeprecationMessage` and the matching `~> **Deprecated.**` callout at the top of `MarkdownDescription`. The message says the construct is removed with the endpoint and claims no date, because Jamf has published none. A successor that exists but is a worse contract (here, one atomic write split into two) is recorded as deliberately not adopted, with the reason.
- **One call has no successor, the construct survives** (`internal/resources/pro/patch_software_title/`). Everything that could move has moved; the one remaining Classic call is kept and annotated, with the condition under which to revisit it.

In both, every call to the deprecated symbol suppresses `SA1019` and points at one place that holds the reasoning (`deprecation.go`, or the annotation block), and the suppression names the tracking issue or that file, since there is no successor to name. The annotation reads:

```go
// Status: deprecated by Jamf YYYY-MM-DD; no migration is possible and none is planned — <why>. Last reviewed YYYY-MM-DD.
```

These suppressions are permanent for the life of the construct, so the timeline's hard floor and the tracking issue's closing condition, which both assume a migration exists, do not describe them.

### Adoption (new endpoints)

| Scenario | Action |
|----------|--------|
| New resource, SDK exposes V1 only | Use V1. |
| New resource, SDK exposes V2 (V1 still current) | **Use V2.** New resources default to the latest stable version. |
| New resource, SDK exposes V2 marked beta/EAP | Use V1. V2 only if V1 is unworkable; document the choice in the resource. |
| Shipped resource on V1, V2 lands, V1 not deprecated | **Do not migrate.** Wait for deprecation, a fast-track trigger, or a material feature gap. |
| Shipped resource on V1, V2 only adds optional fields | Migrate opportunistically, for example when next touching the resource. |
| Shipped resource on V1, V2 fixes an issue meeting the fast-track criteria | **Fast-track migrate.** |

Migrating a shipped resource costs schema and state-upgrader churn, so it needs a reason. A new resource has no migration cost.

### Deprecation migration timeline

| Trigger | Action |
|---------|--------|
| Jamf marks V1 deprecated | Open a migration tracking issue within **30 days** and schedule the work. |
| **6 months** after the announcement | Migration **should be merged**. Soft target. |
| **3 months before the announced removal date** | Migration **must be merged**. Hard floor: no shipped resource may sit on a deprecated endpoint inside this window. A migration not yet shipped becomes top priority. |
| Removal date | Every migration has shipped. Anything not migrated is a regression. |

The soft target catches the common case. The hard floor catches the rest: Jamf shortens the window, a migration slips, or an announcement was missed.

### Fast-track exception

Skip the 6-month buffer and migrate as soon as feasible when V2 includes one of:

- **Security fix** (auth, scope, data exposure).
- **Data integrity** fix (V1 returns wrong values, truncates silently, or races).
- **Breaking bug** in a documented common use case with no workaround.
- **Critical feature** required by a user-reported issue with no viable workaround.

Maintainer call, recorded in the migration PR description.

### Slow-track exception

When a migration requires a **breaking Terraform schema change**, it ships in a minor release like any other breaking change, behind the [90-day schema deprecation](#jamf-pro-terraform-schema-deprecation--90-day-window) above. It does not wait for a major version. Where the 90 days would overrun the hard floor, start the deprecation early enough that they do not, and record the plan in the resource's `crud.go` annotation.

### Tracking — the migration issue

Every endpoint deprecation gets exactly one tracking issue, opened from the **Deprecation migration** template (`.github/ISSUE_TEMPLATE/deprecation_migration.md`). These conventions are mandatory: they make the open migrations greppable and their deadlines visible.

**Label**: `deprecation-migration`, always. Add `blocked-upstream` while the SDK exposes no successor and remove it when the successor ships. No other label substitutes, because the quarterly audit finds outstanding migrations by label. (An attribute removal is tracked under `deprecation-removal` instead.)

**Title**:

```
deprecation(<sdk-family>): <surface> <from>→<to> — migrate by YYYY-MM-DD
```

- `<sdk-family>` is `pro`, `proclassic` or `platform`.
- `<surface>` is the API path segment, not the Terraform name (`computers-inventory`, not `jamfplatform_pro_computer`). Comma-separate surfaces deprecated in one announcement instead of opening an issue each.
- The date is the **binding** date: the 6-month soft target until Jamf publishes a removal date, then the 3-month hard floor if that lands earlier. Edit the title when the binding date moves.

**Deadlines**: issues carry no due-date field, so the dates live on [GitHub Project #2](https://github.com/orgs/Jamf-Concepts/projects/2) in two Date fields, `Deprecated On` (Jamf's announcement) and `Migrate By` (the binding date, matching the title). Every `deprecation-migration` issue must be on the project with both set. Milestones are reserved for releases.

**PR title**: conventional-commit `refactor`, naming the same surfaces as the issue:

```
refactor(<pkg-or-resource>): migrate <surface> <from>→<to>
```

The PR body closes the issue (`Closes #NNN`) and records the fast-track or slow-track call if either applies. One PR per surface unless the surfaces are coupled, for example by a shared side-channel that cannot move independently.

**While a migration is outstanding**, every call to a deprecated symbol carries a suppression naming the issue:

```go
//nolint:staticcheck // SA1019: no v4 client generated yet — see #NNN
```

**Closing condition**: no call site reaches the deprecated method *and* every `SA1019` suppression for that surface is gone. `grep -rn "SA1019" internal/` is the check. A migration that leaves suppressions behind is not finished, because the next audit cannot tell it from one that never started.

### Tracking — in-code annotation (Pro resources only)

Every Jamf Pro resource's `crud.go` opens with an annotation block listing the SDK endpoints in use, their status and the last maintainer review date. Resources in other families do not carry it.

```go
// SDK endpoints used:
//   pro.CreateScriptV2 (introduced Jamf Pro 11.8)
//   pro.ReadScriptV2   (introduced Jamf Pro 11.8)
//   pro.UpdateScriptV2 (introduced Jamf Pro 11.8)
//   pro.DeleteScriptV2 (introduced Jamf Pro 11.8)
//
// Status: current. Last reviewed 2026-05-18.
```

The `Status:` line takes one of four forms:

| State | Line |
|---|---|
| Current | `// Status: current. Last reviewed YYYY-MM-DD.` |
| Deprecated, migration pending | `// Status: deprecated by Jamf YYYY-MM-DD; migrate by YYYY-MM-DD (6mo soft / 3mo hard floor). Last reviewed YYYY-MM-DD.` |
| Migration in flight | `// Status: migration in progress on V3 in PR #NNN. Last reviewed YYYY-MM-DD.` |
| Deprecated, no successor | See [§Deprecated with no successor at all](#deprecated-with-no-successor-at-all). |

The annotation is the single source of truth. There is no separate endpoint-versions file to keep in sync.

### Quarterly audit cadence (Pro resources only)

Once per quarter, a maintainer:

1. Scans the Jamf Pro release notes and the `jamfplatform-go-sdk` changelog for new Pro endpoint versions and deprecations.
2. Greps the annotations: `grep -rA5 "SDK endpoints used:" internal/resources/pro/`.
3. Reconciles each annotation against current Jamf and SDK state.
4. Updates `Last reviewed YYYY-MM-DD` on every annotation that is still current.
5. Opens migration tracking issues for newly deprecated endpoints, following [§Tracking — the migration issue](#tracking--the-migration-issue).
6. Reviews every open `deprecation-migration` issue (`gh issue list --label deprecation-migration`) and checks its `Migrate By` date on Project #2, recomputing it if Jamf has since published a removal date.
7. Flags any annotation whose `Last reviewed` date is older than 120 days.

The `Last reviewed` date is the drift detector: an annotation more than 120 days stale means an audit was skipped.
