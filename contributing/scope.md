# Scope

The API integration scope gate and the shared Jamf Pro scope helper. Part of the [style guide](../STYLE_GUIDE.md).

## API integration scope: every construct declares which scopes reach it

A Jamf API integration is created against one of three scopes, and the provider exposes all three:

| Scope | Provider attribute | Wire |
|---|---|---|
| Platform environment (preferred) | `environment_id` | `X-Environment-Id` |
| Tenant (legacy) | `tenant_id` | `X-Tenant-Id` |
| Organization management | neither | no scope header; the gateway resolves the context from the access token |

The two attributes are mutually exclusive and **both optional**, so a construct cannot assume a scope was configured at all. Every construct's Configure therefore gates on `providerdata.RequireScope`, naming the scopes it can be reached under:

```go
resp.Diagnostics.Append(pd.RequireScope("jamfplatform_<name>", providerdata.BlueprintsScopes...)...)
if resp.Diagnostics.HasError() {
    return
}
```

**Never write the kinds out at a call site.** Pass one of the derived family sets in `internal/providerdata/scopes.go`: `ProScopes`, `SecurityCloudScopes`, `AccountScopes`, `AIGovernanceScopes`, `BlueprintsScopes`, `ComplianceBenchmarksScopes`, `DevicesScopes`, `DeviceGroupsScopes`, `DeviceActionsScopes`. Each is resolved at initialisation from the SDK privilege registry's `MethodPrivileges.Scopes`, so a spec ingest that moves a family arrives as one changed value with a pinned expectation to agree with, instead of going stale at every call site. `scopes_test.go` pins every set, asserts the family's methods agree with each other, and fails on a widening entry the registry has caught up with.

Who writes the call:

- **Pro and ProClassic constructs: no call site.** The gate is applied once inside `configureSub`, so `ConfigurePro` / `ConfigureProClassic` enforce `ProScopes` for every `pro/` package and Pro action.
- **Security Cloud constructs: no call site.** `providerdata.ConfigureSecurityCloud` applies `SecurityCloudScopes` (see [§Jamf Security Cloud Resource Naming](families.md#jamf-security-cloud-resource-naming)).
- **Jamf Account constructs: no call site.** `providerdata.ConfigureAccount` applies `AccountScopes` (see [§Jamf Account Resource Naming](families.md#jamf-account-resource-naming)).
- **Platform Services and AI Governance constructs wire it explicitly**, immediately after the `*providerdata.Data` type assertion.

Rules for the gate:

- **Argument order is presentation order.** It is the order the diagnostic lists the scopes in, so environment comes first. The derived sets are already ordered that way; do not re-order them at a call site.
- **Do not gate in provider Configure instead.** The answer differs per API family (Blueprints and Compliance Benchmarks are environment-only, Pro is not), and a provider-level assertion would also block the organization-level constructs that legitimately run without a scope header.
- **Organization scope is not a rejected special case.** It is the only scope that reaches the `jamfplatform_account_*` family, and it is rejected everywhere else. Enforcing both directions turns an opaque `403` mid-apply into a named diagnostic at Configure.

### Widening and narrowing a family

**A spec-declared set is not always what the gateway serves.** Where the gateway answers a scope the spec no longer declares, `scopes.go` carries a `gatewayWidenings` entry: a family, a scope, and the dated wire evidence. The table in the code is the record of which families are widened and why; do not restate it here. An entry needs end-to-end evidence (the construct working under that scope, not a status code), because narrowing such a family would break working configurations. An entry is an assertion about the gateway and is **deleted rather than edited** when either half of its justification goes.

**A narrowing needs evidence of its own, and a `403` is not it.** A `403 BAD_PERMISSIONS` does not distinguish an unrouted path from a missing privilege (see [§Security Cloud shapes that recur](families.md#security-cloud-shapes-that-recur)), and two credentials that each lack the capability settle it no better than one. Security Cloud answers the same `403` and is reachable under both scopes.

Blueprints and Compliance Benchmarks are environment-only on different evidence: Jamf Account's permission picker does not offer the `blueprints` or `compliance-benchmarks` capabilities when the integration being created is tenant-scoped. No tenant credential can hold them, so refusing tenant scope takes away no configuration a practitioner could write. What falsifies that is a change to the picker, not a `200` from either route.

When you narrow a family, record which evidence you have and where it came from. An ungrantable capability is a decision you can act on; an ambiguous `403` is not.

### Where the code lives

- `internal/provider/scope.go`: scope resolution (config beats environment, both-at-once is an error, a shadowed environment variable warns).
- `internal/providerdata/scope.go`: the `ScopeKind` vocabulary and the gate.
- `internal/providerdata/scopes.go`: the derived family sets and `gatewayWidenings`.

`providerdata.New` derives the configured scope from the SDK client's own `Client.Scope()` instead of taking it as a parameter, so a caller cannot build a `Data` whose declared scope differs from the header its client sends.

Scope is also documented: `permissions.Section` opens its "Required Jamf permissions" lead-in with the integration scope the endpoints are published at, from the same registry. That line reports what the **spec** declares and omits the widenings. A published table carries Jamf's declaration, and `RequireScope`'s diagnostic is the authority on what a given release accepts.

## Scope helper

The `<scope>` block of the Jamf Classic-API resources (policies, ebooks, Mac and mobile device apps, macOS and mobile device configuration profiles, patch policies, restricted software, VPP assignments and invitations) shares its target categories: buildings, departments, computers, computer groups, mobile devices, mobile device groups, network segments, Jamf Pro users and user groups, iBeacons, and the directory-service name-only siblings. Shared helpers live under `internal/common/scope/`.

### Block layout

Every `scope` block mirrors the Jamf Pro admin UI's three Scope tabs: `targets`, `limitations`, `exclusions`.

- The all-flags (`all_computers`, `all_mobile_devices`, `all_jss_users`) and every per-category target set live inside a `targets` `SingleNestedAttribute`. `limitations` and `exclusions` are its siblings.
- **Everything is `Optional`-only**: `targets`, the category sets and the all-flags. Nothing may be `Computed`, carry a state-forwarding plan modifier, or have a static default, because the null/`[]` (or null/`false`) distinction carries the [ownership contract](#omission-semantics). An Optional+Computed `SingleNestedAttribute` over a `*struct` model also fails Unknown decode at apply.
- Sub-block struct allocation gates on `<target>.Targets != nil`, where `<target>` is the model being written to state (the mutated parameter, which is the new plan on Update), **not** a separate prior-state reference. See [§`SingleNestedAttribute` blocks](schema-design.md#singlenestedattribute-blocks-optional-only-when-the-model-uses-typed-pointer) for why gating on the prior state crashes the remove-on-update transition.
- In the model, target fields sit in a `…ScopeTargetsModel` reached via `Targets *…ScopeTargetsModel`. The build side reads them through a `TargetsOrZero()` value accessor, which returns all-null fields when the block is omitted, so input builders need no nil guards.

### Item shape and naming

Categories are a flat `schema.SetAttribute{ElementType: types.StringType, Optional: true}` carrying only the numeric Jamf Pro classic ID, or the name for the directory-service categories. The server-augmented `<name>` and `<udid>` wire fields are discarded on read; only IDs round-trip through state. Authoring uses interpolation: `computer_ids = [for c in data.jamfplatform_pro_computers.example: c.id]`.

| Kind | Suffix | Examples |
|---|---|---|
| ID-bearing | `_ids` | `computer_ids`, `computer_group_ids`, `building_ids`, `department_ids`, `jss_user_ids`, `jss_user_group_ids`, `network_segment_ids`, `ibeacon_ids`, `class_ids` |
| Directory-service name-only | `_names` | `directory_service_or_local_user_names`, `directory_service_user_group_names`, `limit_to_user_group_names` |

Limitations and exclusions share the same attribute names. On the wire a limitations user is name-only and an exclusions user is id+name, but both sides write `<user><name>…</name></user>`, so the response-side `<id>` is discarded like every other server-augmented field.

**A numeric Jamf Pro group ID does not identify a group on its own: always pair it with the estate.** Jamf Pro numbers computer groups and mobile device groups in independent sequences, so the same integer can name one of each. A map, cache or resolver keyed on the ID alone silently loses one of every colliding pair, and the survivor is then rejected as the wrong type, which reads as "group not found" for a group that exists. Key on `{deviceType, id}` (see `internal/common/impact/cache.go`), or thread an estate discriminator through the lookup the way `internal/common/criteria`'s `GroupResolver` threads `ObjectType`. Platform group UUIDs are unique tenant-wide, so a UUID-keyed index needs no discriminator and UUID → Jamf Pro ID resolution (`device_group`'s `resolveJamfProID`) is unambiguous. A unit test catches a regression here only if its fixture contains a cross-estate ID collision.

### Composition

Per-resource glue assembles the `scope` schema from `scope.IDSetAttribute` / `scope.NameSetAttribute` calls. There is **no** single `ScopeAttribute()` factory across all scope-bearing resources: their top-level field sets differ, so one factory would either leak unsupported fields or become per-resource branching. The 3-consumer extraction rule ([§Shared abstractions](package-layout.md#shared-abstractions--when-to-extract)) fires at **category granularity**, not at the top-level scope shape.

Where several resources share one platform's shape (targets + limitations + exclusions), that shape is a composite: `scope.ComputerScopeAttributes(scope.ComputerScopeOptions{…})`, `scope.MobileScopeAttributes(scope.MobileScopeOptions{…})` and `scope.UserScopeAttributes()`. Each returns the full attribute map; the caller wraps it in its own `schema.SingleNestedAttribute` with a resource-specific description. Files: `computer_{model,schema}.go`, `mobile_{model,schema}.go`, `user_{model,schema}.go`.

- **Per-resource deltas go through an `Options` struct, not a fork.** The only axis today is `IncludeIbeacons bool`. Endpoints that silently drop iBeacon limitations and exclusions (`macapplications`, `mobiledeviceapplications`) set it `false`, so the attribute is absent instead of producing a permanent diff.
- **Each `IncludeIbeacons` variant needs its own model struct**, because the framework matches model fields to schema attributes exactly. Hence `ComputerScopeModel` / `ComputerScopeModelNoIbeacons` and `MobileScopeModel` / `MobileScopeModelNoIbeacons`. A resource references whichever matches its option as `Scope *scope.<Variant>`.
- **A composite may be extracted ahead of three consumers** when a same-platform sibling is imminent. That is a maintainer call.
- Resource-specific constraints (restricted software rejects `limitations` entirely) stay in the resource package.

### All-flag validators

`scope.AllFlagConflictsWith` is a `validator.Bool` that fires only when the flag is `true` and attaches one attribute error per populated conflicting set. The off-the-shelf `boolvalidator.ConflictsWith` triggers on any value and cannot express "only when true".

- Paths are relative (`path.MatchRelative().AtParent().AtName("…")`). The flag and the sets it conflicts with all live inside `targets`, so nesting does not affect them.
- The conflict set is the flag's **paired** categories only (see the [pairing table](#all-flag-pairing)): `all_computers` conflicts with `computer_ids` / `computer_group_ids`, never with `building_ids` / `department_ids`.
- A conflict that exists only when several flags are on together is `scope.AllFlagsConflictWith(others, conflicts...)`, attached to **one** of the flags so the diagnostics are not repeated (`ebook` attaches it to `all_computers`).
- Every all-flag description ends with `scope.OmitAllFlagNote(paired)`, which states the one exception to omit-means-preserve.

### Omission semantics

The user-facing contract for every scope category, and the invariant everything below serves:

- **Declared (including `[]`) ⇒ Terraform owns the category.** Members drift-revert on refresh; `[]` clears the category.
- **Omitted (null) ⇒ the category is left as configured outside Terraform.** It never enters state or plans, and updates preserve it.

The classic wire cannot express the second half directly. Every scope-bearing classic endpoint behaves the same way on `PUT` (an ebook's computer and mobile union is one replace unit):

| PUT body | Server behaviour |
|---|---|
| no `<scope>` element | scope untouched (top-level classic merge) |
| `<scope></scope>` with no child elements | ignored, untouched |
| `<scope>` with only all-flags at their current value | ignored, untouched |
| `<scope>` with ≥1 category element, **even an empty one** | **entire subtree replaced**: every category not in the body, across targets, limitations and exclusions, is cleared |
| `<scope>` with only an all-flag set `true` | flag set, whole scope cleared |
| all-flag `true` alongside category elements | flag set; only its paired categories are wiped; every other category follows the body |

#### All-flag pairing

| Flag | Paired categories the server wipes | Kept alongside a true flag |
|---|---|---|
| `all_computers` | `computers`, `computer_groups` | `buildings`, `departments`, `jss_users`, `jss_user_groups` (and the mobile union and `classes` on ebooks) |
| `all_mobile_devices` | `mobile_devices`, `mobile_device_groups` | `buildings`, `departments`, `jss_users`, `jss_user_groups` (and the computer union and `classes` on ebooks) |
| `all_jss_users` | `jss_users`, `jss_user_groups` | everything else the endpoint stores |

Buildings, departments and ebook classes are never wiped by any flag or combination, including all three flags on an ebook and `all_computers` on restricted software, where the admin UI offers none of them but the server stores them regardless. Limitations and exclusions always coexist with a true flag. `patchpolicies` silently drops `jss_users` / `jss_user_groups`; the endpoint has no such category.

#### Consequences, all mandatory

1. **Update is read-merge-write, scope-only.** When the plan declares a `scope` block, `Update` GETs the live object before building the PUT, flattens **only its scope** with the hydrate-all path (`includeUnmanaged=true` into a zero scope model), overlays the declared categories with the shape's `scope.Merge*` helper (a local scope model implements an equivalent local merge), and builds the wire payload from a shallow-copied plan whose `Scope` is the merged model. The **original** plan is what goes to state. No other section of the GET response is echoed into the PUT; that would turn omitted non-scope sections into Terraform-asserted ones. `Create` never merges: undeclared categories are absent from the POST.
2. **Merged output emits the full explicit skeleton.** Every merged category is non-null. `BuildIDSlice` / `BuildNameSlice` return a **non-nil empty slice** for `[]` (declared clear) and `nil` for null or unknown (unmanaged, element omitted). An empty category therefore marshals as an explicit empty element (`<computer_groups></computer_groups>`), the wire's only reliable clear: a merged body with no category elements would hit the ignored rows above and a final clear would silently do nothing. For scope, an empty parent element **is** emitted.
3. **Read-side gating is per category.** Scope flatten functions refresh each category through `scope.RefreshManagedSet` / `scope.RefreshManagedBool`, gated on the field of the model being written (the plan on create and update, prior state on read; never a separate prior-state reference). Wire-flatten helpers return `scope.EmptyStringSet()`, never a null set, for absent wire elements, or a managed category would be nulled and fail the post-apply consistency check. `includeUnmanaged=true` bypasses the gate for import hydration, `terraform query` config generation, and the Update merge base.
4. **All-flag precedence in the merge follows the pairing table, in two halves.**
   - A merged all-flag that is `true` empties exactly its **paired** categories and nothing else. Emptying more than the server does destroys UI-owned members the configuration never mentioned.
   - **A declared non-empty paired category beats an unmanaged (null) flag.** `scope.PickAllFlag` sends the flag `false` instead of inheriting a `true` the admin UI set, which would wipe the members just declared and fail the post-apply consistency check. A declared flag always wins. A declared `[]` carries no members and does not flip a preserved flag. The flag stays null in plan and state, and the plan shows the category diff that states the intent.
   - A declared member alongside a *declared* `true` flag is still rejected at plan time by the validator; precedence only affects preserved server values.
   - Every merge site calls `PickAllFlag` with the **plan's** sets for the paired categories: `mergeComputerTargets`, `mergeMobileTargets`, `MergeUserScope`, and the resource-local merges in `ebook`, `restricted_software` and `patch_policy`. A new scope shape must do the same.
5. **Honour the admin UI where it is stricter than the wire: restricted software and ebooks only.** The UI offers no target once All Computers is selected on restricted software, and none of buildings, departments or classes once all three flags are on for an ebook, although the server keeps them. A member stored that way could be neither seen nor removed in the UI, so those two shapes pair the extra categories:
   - `restricted_software` pairs `building_ids` / `department_ids` with `all_computers` in the validator, the merge emptying and `PickAllFlag` alike.
   - `ebook` adds `scope.AllFlagsConflictWith` (fires only when all three flags are `true`), empties `building_ids` / `department_ids` / `class_ids` in the merge when all three end up true, and turns every *unmanaged* flag off when a residual category is declared while all three would be true.
   - Do not widen this to other resources. Their UI offers buildings and departments alongside the flag.
6. **No plan-time warning about categories managed outside Terraform.** It would fire on every plan of a deliberate co-management setup, Terraform has no per-resource way to mute it, and none of the provider's other omit-means-preserve surfaces warn. The read-merge-write never destroys unmanaged categories. Do not add one without at least gating on plans that modify scope.
7. **Each scope-bearing resource carries a split-ownership acceptance test**, and import steps over a subset-scope configuration ignore `scope`. See [TESTING.md](../TESTING.md).

#### `classes` needs two writes on Update (ebooks only today)

On top of the replace rule, Jamf Pro stores `<classes>` only while the **stored** category is empty. A populated-to-populated transition clears the category whatever the body carries: re-sending the identical `<classes>` clears it, and so does omitting it, whether the child is `<id>`, `<name>` or both. Combined with the wholesale replace, no single body preserves a stored class across another scope change. Two bodies do: the first carries the whole desired scope with `<classes>` emptied, the second the same scope with the classes populated.

- **Gate on the category, not the resource.** Gate on "the merged desired scope carries class members". A scope with none takes the single-request path, and a resource that gains the category later needs no further change. Reference: `ebookScopeClassesCleared` in `internal/resources/pro/ebook/input_builders.go`.
- **The first request carries the change.** An apply interrupted between the two then leaves the change applied and the classes empty, restored by the next apply. Never order it the other way.
- **`Update` only.** A new object's category starts empty, so `Create` stores classes first time.
- **Clearing needs one request.** A declared `[]` against a populated category is the one populated-to-X transition the server honours.

#### Non-scope sections: nil-pointer sub-blocks suppress wire emission

The classic API tolerates absent sections. A `POST` / `PUT` body need not include `<reboot>`, `<self_service>` and so on when the caller does not manage them: block absent (null) → SDK field nil → `,omitempty` omits the element. The `all_*` booleans use `helpers.OptionalBoolPointer` so an unset flag is nil; `false` marshals as a real element, distinct from omission.

### Directory-service user-group preflight

Mandatory for every resource whose scope has `directory_service_user_group_names`. The classic server matches each entry against the tenant's configured LDAP or cloud identity provider and rejects an unknown name at *apply* with an opaque `409 "Problem matching limitation user group"`. Surface that at *plan*.

- **Helper**: `scope.ValidateDirectoryServiceUserGroupNames(ctx, searcher, set, attrPath)` in `internal/common/scope/ldap_preflight.go`, backed by `ldapgroups.ResolveByName` (exact-name match across servers; scope groups carry a name only). Unknown name → attribute **error**. Search or transport failure, or no directory service configured → attribute **warning**, never blocking a plan. Null or unknown set or element → skipped.
- **Wire it in `ModifyPlan`, not a `ConfigValidator`.** The search needs the configured client, and config validators run before `Configure`. The resource holds a `pro.Client` (the `ldapgroups.Searcher`) by **also** calling `providerdata.ConfigurePro` in `Configure`, alongside its `ConfigureProClassic` CRUD client. `ConfigurePro` returns `(nil, nil)` before configuration, which leaves the preflight a no-op.
- **Call it once per set**, for `scope.limitations` and `scope.exclusions`, at `path.Root("scope").AtName("limitations"|"exclusions").AtName("directory_service_user_group_names")`.
- **Run it first in `ModifyPlan`**, before any payload-diff early return (those return on create, where the preflight must still fire). It runs on every non-destroy plan; guard `req.Plan.Raw.IsNull()` for destroy.
- **Only `directory_service_user_group_names` is covered.** There is no equivalent lookup for users, so `directory_service_or_local_user_names` (local users, stored verbatim) is not validated.

Reference implementations: `mac_app_store_app`, `policy`, and `macos_configuration_profile` / `mobile_device_configuration_profile`, which add a `preflightScopeGroups` helper because their `ModifyPlan` already carries payload-diff logic.
