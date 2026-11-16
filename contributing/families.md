# Product family rules

Naming, Configure helpers and recurring shapes for Jamf Account, Jamf Security Cloud and Jamf Pro, including the Jamf Pro version gate. Part of the [style guide](../STYLE_GUIDE.md).

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
   `ScopeOrganization` alone; see [§API integration scope](scope.md#api-integration-scope-every-construct-declares-which-scopes-reach-it).
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
8. **Import by domain name is a sanctioned exception to [§Import format](write-semantics.md#import-format).**
   `jamfplatform_account_sso_domain` imports by name, not id, for two reasons that both have to
   hold before an exception is granted: nothing in the API reads a domain by id (the per-id GET
   is unrouted), and the id is **not stable** — removing and re-claiming a domain mints a new
   one. Where an id is neither usable nor stable, the name is the primary key. Note the
   acceptance harness defaults `ImportStateId` to the `id` attribute, which is exactly the value
   `ImportState` rejects, so a name-imported construct must set `ImportStateId` explicitly.
9. **Canonicalise by validator, not by plan modifier.** The server lower-cases a stored domain.
   Because `domain` is `Required`, [§Plan-modifier rewrites are NOT a valid option for `Required`
   attributes](schema-design.md#plan-modifier-rewrites-are-not-a-valid-option-for-required-attributes) rules out
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
[§Referenced-by-name dependencies](write-semantics.md#referenced-by-name-dependencies--has_dependencies-on-delete):
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
[TESTING.md](../TESTING.md).

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
   - **Prefer the admin-UI label over an API-namespace artifact when they diverge.** The default slug source is the SDK filename (rule 2), but when that name is a wire/namespace artifact absent from the admin UI, name the construct after the UI instead — the same principle as [§Attribute names mirror the Jamf Pro admin UI](schema-design.md#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic), applied to the construct. Examples: `jamfplatform_pro_computer_check_in_settings` (pro V3 SDK namespace `client_check_in` — "client" appears nowhere in the UI, which reads *Computers → Check-in*; this also matches the classic `computercheckin` it supersedes); `jamfplatform_pro_mdm_profile_settings` (pro V1 SDK type `DeviceCommunicationSettings` — the UI panel is "MDM profile settings"). When two endpoints (e.g. a pro and a classic face) are the same settings object, pick one UI-aligned slug and mark the other **superseded** in the inventory.
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
- **Source the value** during the SDK-comparison phase of the resource-addition workflow ([CONTRIBUTING.md §Adding a Jamf Pro Resource](../CONTRIBUTING.md#adding-a-jamf-pro-resource)). Record in `JAMF_PRO_INVENTORY.md`.

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

#### Failure modes

| Condition | Outcome |
|-----------|---------|
| Tenant does not have Jamf Pro provisioned, Pro resource with non-empty `minJamfProVersion` used | `GetJamfProVersionV1` errors → propagated by `GetJamfProVersion` → resource Configure surfaces as "Failed to read Jamf Pro tenant version" |
| Tenant does not have Jamf Pro provisioned, Pro resource with empty `minJamfProVersion` used | Configure passes; first CRUD call surfaces the underlying Jamf API error |
| `GetJamfProVersionV1` returns a transient network/auth error | Same path as above; cached on `ProviderData` until next `terraform` invocation |
| Tenant version parses, `< required` | `RequireMinJamfProVersion` error |
| Tenant version unparseable | `RequireMinJamfProVersion` error with raw string |
| `minJamfProVersion = ""` | Version check skipped; no fetch triggered by this resource |

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
