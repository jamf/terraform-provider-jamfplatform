# Product family rules

Naming, Configure helpers and recurring shapes for Jamf Account, Jamf Security Cloud and Jamf Pro, including the Jamf Pro version gate. Part of the [style guide](../STYLE_GUIDE.md).

Two rules hold for every family and are stated once elsewhere:

- Every enum, and every discriminator-to-payload pairing, is validated at plan time from the SDK's generated `*Values()` helper, never a restated list: [§Enum values and error codes come from the SDK](schema-design.md#enum-values-and-error-codes-come-from-the-sdk-not-from-literals). The servers in these families do not attribute an enum mistake, which is why.
- Which integration scopes reach a family is gated in Configure: [§API integration scope](scope.md#api-integration-scope-every-construct-declares-which-scopes-reach-it).

Jamf AI Governance (`jamfplatform_ai_governance_<x>`, `internal/resources/ai_governance/<x>/`) has no section here. It is a Platform Services family: hand-written Configure with an explicit `RequireScope(…, providerdata.AIGovernanceScopes...)`, no version gate.

## Jamf Account Resource Naming

Resources backed by the `account/` package of `jamfplatform-go-sdk` carry the `jamfplatform_account_` prefix. This is the provider's **organization-level** family: what a practitioner manages at *account.jamf.com → Organization*, above and across individual Jamf Pro, School, Protect and Security Cloud tenants.

Three layers name this family differently: the SDK package is `account`, the gateway prefix is `sso`, and the spec bundle is `account-sso`. The Terraform name follows the **product surface**, Jamf Account, because the gateway prefix is a routing artifact and the family is not only SSO (platform environments and tenants are in it too).

### Rules

1. **Terraform construct name**: `jamfplatform_account_<resource>`. The SSO constructs are `jamfplatform_account_sso_connection` and `jamfplatform_account_sso_domain`; the `sso_` qualifier comes from the UI section, because "connection" and "domain" are ambiguous on their own.
2. **Do not confuse it with Jamf Pro's SSO.** `jamfplatform_pro_sso_settings` and `jamfplatform_pro_sso_failover_url` configure a single Jamf Pro tenant's own SSO: a different product, endpoint and scope. The prefix keeps them apart, which is why the family is not named `jamfplatform_sso_*`.
3. **Go package path**: `internal/resources/account/<resource>/`, flat, one leaf package per construct family, folder name equal to the Terraform slug minus the `jamfplatform_account_` prefix. Actions live under `internal/actions/account/<resource>/`, one package per resource with a file per verb, as in `internal/actions/pro/mdm/`.
4. **Organization scope only, via `providerdata.ConfigureAccount`.** Do not hand-roll Configure and do not route through `ConfigurePro`: there is no customer-tenant version to read, and an organization may hold no Jamf Pro tenant at all. The gate is `ScopeOrganization` alone.
5. **No organization ID travels anywhere.** An organization-scoped integration authenticates on client id and secret alone, and the gateway resolves the organization from the access token. `/sso/v1` ignores `X-Environment-Id` and `X-Tenant-Id` when either is sent. There is no `organization_id` provider attribute and no SDK `WithOrganizationID`. An `X-Organization-Id` header exists and is parsed by the gateway, but nothing needs to send it.
6. **US gateway only.** The namespace is absent from the EU gateway: a bogus route under `/sso/v1` there returns the gateway's bare `404 page not found`, versus `403 BAD_PERMISSIONS` in US. Acceptance tests skip on a non-US base URL, and the region limitation belongs in user-facing documentation.
7. **Acceptance tests use `testhelpers.AccPreCheckAccount`, never `AccPreCheck`.** See [TESTING.md](../TESTING.md).
8. **Import by domain name is a sanctioned exception to [§Import format](write-semantics.md#import-format).** `jamfplatform_account_sso_domain` imports by name, not id. Both conditions must hold before such an exception is granted: nothing in the API reads the object by id (the per-id GET is unrouted), and the id is **not stable** (removing and re-claiming a domain mints a new one).
9. **Canonicalise by validator, not by plan modifier.** The server lower-cases a stored domain. `domain` is `Required`, so [§Plan-modifier rewrites are NOT a valid option for `Required` attributes](schema-design.md#plan-modifier-rewrites-are-not-a-valid-option-for-required-attributes) rules out rewriting the value. Accept only the canonical spelling and name it in the diagnostic. Lookups stay case-insensitive so an import still forgives mixed case.

### Jamf Account shapes that recur

- **A collection may answer with either an envelope or a bare array.** The SDK's `UnwrapResults` tolerates both. Do not assume the declared envelope.
- **No RSQL filter exists on any account collection GET**, so a list resource here is an unfiltered stream, as in AI Governance.
- **`403 BAD_PERMISSIONS` on a spec-advertised route means unrouted, not unprivileged**, the same law as [§Security Cloud shapes that recur](#security-cloud-shapes-that-recur). The one case where it can be read as a real authorization refusal is when a *different* credential answers `200` on the same URL in the same region; then the route is provably mapped.
- **`errors[].field` is populated only for top-level required fields.** An invalid enum comes back as `MALFORMED_REQUEST_BODY` with `field: null` and the value never named, and a discriminator/payload mismatch is an unattributed `500`.

## Jamf Security Cloud Resource Naming

Resources backed by the `securitycloud/` package of `jamfplatform-go-sdk` carry the `jamfplatform_security_cloud_` prefix. The SDK generates all five Security Cloud API namespaces (`jsc-categories`, `jsc-dns`, `jsc-ztna`, `securitycloud-devices`, `uem-connect`) into that one package. Those names are gateway routing artifacts that appear nowhere in the admin UI, so they are not part of the Terraform name.

### Rules

1. **Terraform construct name**: `jamfplatform_security_cloud_<resource>`, regardless of which API namespace backs it. Same reasoning as [§Jamf Pro Resource Naming](#jamf-pro-resource-naming): the API-layer split is not the user's problem.
2. **Slug source**: the admin-UI label, not the SDK filename, wherever the two diverge. `zones.go` under *Integrations → Custom DNS → DNS zones* becomes `security_cloud_dns_zone`; the `dns_` qualifier comes from the UI section, because "zone" alone is ambiguous beside ZTNA gateways and device groups.
3. **Singular vs plural**: as for Jamf Pro. Singular resource and by-id/by-name data source, plural data source and list resource.
4. **Go package path**: `internal/resources/security_cloud/<resource>/`, flat, one leaf package per construct family, folder name equal to the Terraform slug minus the `jamfplatform_security_cloud_` prefix. Code shared across two or more Security Cloud packages lives under `internal/common/<topic>/`, never in a sibling.
5. **No tenant version gate.** Security Cloud is continuously deployed and has no customer-tenant version. These constructs declare no `minJamfProVersion` and must **not** be routed through `ConfigurePro` / `ConfigureProClassic`, which fetch the Jamf Pro version; a Security Cloud-only tenant cannot answer that.
6. **Entitlement is separate from authentication.** A tenant can authenticate and still have no Security Cloud surface; the API says so with `403 NOT_ENTITLED`. Translate that code into a named diagnostic.

### Security Cloud shapes that recur

**A single-element array is a scalar.** Several fields are declared as arrays but rejected at any size but one: the IPsec cipher suites' `encryption`, `integrity` and `dhGroups`, and the Jamf-side `left.subnets`. Model each as a single string or number and collapse at the boundary in the input and state builders. A list would offer room the server refuses, and the refusal names the field's size, not the mistake.

**An enum violation is unattributed, and not identically across services.** `jsc-dns` and `jsc-ztna` answer `400 [INVALID_FIELD] Request body is missing or malformed.` with no field and no value. `uem-connect` answers `422 VALIDATION_FAILED` with a message that does name the accepted values. Do not write a diagnostic against a code another construct observed. Where the SDK documents a set but generates no helper (grouped gateway's `recoveryDelayInSec`), restate it in the package's `mappings.go` and say why in the doc comment.

**Whether a referenced object can be deleted is per construct: probe it.**

- *Gateways* refuse. Deleting a gateway that a DNS zone's name server or a grouped gateway's membership still names answers `409 CONFLICT` with no detail identifying the referrer. That is the destroy-ordering trap in [§Referenced-by-name dependencies](write-semantics.md#referenced-by-name-dependencies--has_dependencies-on-delete): removing the reference and destroying the target in one apply lets Terraform sequence the destroy first. Translate the 409 into a diagnostic that names the possible referrers and says to split the applies.
- *Device groups* do the opposite. Deleting a group that a ZTNA app's `assignments.inclusions.groups` still names returns `204`, and the app is left at `allUsers: false` with an empty group list, a combination the API rejects on write. There is no 409 to translate and nothing for Terraform to sequence, so the hazard goes in the resource's description.

Do not carry either behaviour from one construct to another.

**An unmapped route answers `403 BAD_PERMISSIONS`, not `404`.** A `GET` on a deliberately bogus Security Cloud path returns exactly the body an unprivileged call does. The status is ambiguous by construction and **must not be translated into a "grant this privilege" diagnostic**.

- When the spec advertises an endpoint the gateway 403s on, presume the route unmapped. Confirm by probing a bogus path with the same token and by checking that a sibling call under the same privilege still succeeds.
- This is not the entitlement failure. `403 NOT_ENTITLED` is a different code and gets its own diagnostic (rule 6).

**A `400` from the gateway is not always a bad request.** Under a tenant-scoped credential with no Security Cloud entitlement, `GET /securitycloud/v2/groups` answers `400 [URI_NOT_TEMPLATED] Failed to (fully) template URI to the downstream service`, while `GET /securitycloud/v1/ztna/apps` on the same credential answers the usual `403 BAD_PERMISSIONS`. This third failure shape is per route, not per service, and says nothing about the request body. Do not translate it into a validation diagnostic. An operator without the entitlement can see either code from one construct, depending on which endpoint runs first.

**"Unrouted" is a snapshot, not a property of the endpoint.**

- A `403` clearing is not the same event as the capability arriving. A route that stops 403ing still needs its success probed, and the change read back through a *different* operation, because a handler that accepts a write and discards it is worse than a refusal.
- A suppression that keeps a deprecated endpoint in use because its successor is unrouted is temporary. It names a date and an upstream report, and it is re-probed at the next SDK bump.

**A read-only status timestamp does not belong in the schema.** The gateway's `status.updatedAt` and the grouped gateway's `updatedAt` advance on every server-side re-evaluation. A Computed attribute over such a value makes every refresh report a change no configuration can act on. Omit it and say so in the state builder's doc comment. Keep the fields that settle (`status.state`, `status.tunnelState`) and the ones that never move (`createdAt`).

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

It type-asserts `*providerdata.Data`, applies the `RequireScope` gate (`SecurityCloudScopes`, environment or tenant) and returns a `*securitycloud.Client`. It deliberately does not go through `configureSub`; see rule 5.

Acceptance tests gate on `testhelpers.AccPreCheckSecurityCloud`; see [TESTING.md](../TESTING.md).

## Jamf Pro Resource Naming

Resources backed by the `pro/` or `proclassic/` packages of `jamfplatform-go-sdk` share one naming scheme that hides the API-layer split. Both layers expose Jamf Pro functionality, and users should not care which one is wired up.

### Rules

1. **Terraform construct name**: `jamfplatform_pro_<resource>` for every Jamf Pro resource, data source, list resource and action, whether it is sourced from `pro/` or `proclassic/`.
2. **Slug source**: derive `<resource>` from the SDK filename, snake_case and singular.
   - `pro/scripts.go` → `jamfplatform_pro_script`
   - `pro/smart_computer_groups.go` → `jamfplatform_pro_smart_computer_group`
   - `proclassic/networksegments.go` → `jamfplatform_pro_network_segment`
   - `proclassic/osxconfigurationprofiles.go` → `jamfplatform_pro_macos_configuration_profile` (override; modern terminology)
3. **Singular vs plural**:
   - Resource: singular (`pro_script`).
   - Data source: singular for ID or name lookup (`pro_script`), plural for filtered or list lookups (`pro_scripts`).
   - List resource: plural (`pro_scripts`).
   - Action: named for the verb (`pro_flush_policy_logs`, `pro_send_blank_push`).
4. **Go package path**: `internal/resources/pro/<resource>/`, flat, no domain tier. The folder name is the **Terraform slug minus the `jamfplatform_pro_` prefix**, and the Go package declaration matches it verbatim: `jamfplatform_pro_self_service_plus_settings` → `pro/self_service_plus_settings/`. Do not drop descriptive suffixes (`_settings`, `_group`), so that one name greps to the other. Code shared across two or more Pro packages lives under `internal/common/<topic>/`, never in a sibling resource package.
5. **Pro vs ProClassic**: default to `pro/`. Use `proclassic/` only when `pro/` has no equivalent endpoint or is materially less complete (read-only where classic offers CRUD, missing required fields). When a resource uses both, say why in its package comment.
6. **Overrides**: where the SDK filename is awkward, outdated or ambiguous, override the slug and record it in `spike/JAMF_PRO_INVENTORY.md` when the batch is approved. There is no upfront override table.
   - **Prefer the admin-UI label over an API-namespace artifact.** When the SDK name is a wire or namespace artifact absent from the admin UI, name the construct after the UI, the same principle as [§Attribute names mirror the Jamf Pro admin UI](schema-design.md#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic). Examples: `jamfplatform_pro_computer_check_in_settings` (SDK namespace `client_check_in`; the UI reads *Computers → Check-in*) and `jamfplatform_pro_mdm_profile_settings` (SDK type `DeviceCommunicationSettings`; the UI panel is "MDM profile settings").
   - When a pro and a classic endpoint are the same settings object, pick one UI-aligned slug and mark the other **superseded** in the inventory.
7. **Inventory tracking**: every Jamf Pro construct (planned, in design, in progress, shipped, skipped) is tracked in `spike/JAMF_PRO_INVENTORY.md`, which is gitignored.

### Minimum Jamf Pro version check

The provider builds **one** `jamfplatform.Client` from the standard `JAMFPLATFORM_*` credentials; there is no separate set of Pro credentials. Jamf Pro is the only family versioned at the tenant, so only Pro constructs carry a version check. Everything else stays usable on a tenant with no Jamf Pro.

Two values are involved, and they are independent:

| | Per-construct floor | Provider build target |
|---|---|---|
| Declared as | unexported `const minJamfProVersion` in the construct's package | `ProviderMinJamfProVersion` in `internal/providerdata/providerdata.go` |
| Meaning | this construct cannot work below this version | the Jamf Pro API version this release was built against |
| Tenant below it | **error** at Configure | **warning**, once per provider instance |
| Value | `""` unless an endpoint needs a newer version | tracks the SDK's `jamfplatform.JamfProAPIVersion` |

**The per-construct floor.**

```go
const minJamfProVersion = ""  // or the version the construct's endpoints first shipped in, e.g. "11.25.0"
```

- **Co-locate it** with the construct: `resource.go`, or `data_source.go` / `helpers.go` in a package with no resource. Do not centralise.
- **`""` means no floor**, which is the normal case. Set a version only when an endpoint the construct calls does not exist below it.
- **Source the value** during the SDK-comparison phase of [CONTRIBUTING.md §Adding a Jamf Pro Resource](../CONTRIBUTING.md#adding-a-jamf-pro-resource) and record it in the inventory.

**The provider build target** says which API the release was built for, not what a tenant must run. It is **not** the maximum of the per-construct floors; those are almost all empty, so that rule would move it backwards. Bump it with the SDK dependency to match `jamfplatform.JamfProAPIVersion`, then run `make generate`: the provider description interpolates the constant, so `docs/index.md` follows. When a further product gains a tenant version, it gets its own `Provider<Product>MinVersion` constant and row in the provider description table.

The warning a tenant below the build target gets:

> Jamf Pro tenant older than provider build target
>
> This provider release was built against the Jamf Pro API as of version `A.B.C`. The tenant reports `X.Y.Z`. Some Pro resources may rely on endpoints or fields that did not exist in the tenant's version and could fail at apply time. Upgrade Jamf Pro or pin an older provider release.

#### Pro Configure: use the `providerdata.ConfigurePro` helper

Every Jamf Pro resource, data source, list resource and action funnels Configure through `providerdata.ConfigurePro` (or `ConfigureProClassic`, identical but returning the classic client). Do not hand-roll it.

```go
func (r *Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_<name>")
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }
    r.client = client
}
```

The helper type-asserts `*providerdata.Data`, applies the scope gate, reads the tenant version, runs the per-construct floor when one is set, and appends the build-target warning. When `req.ProviderData` is nil (early framework lifecycle) it returns `(nil, nil)`: leave `r.client` unset and let the next Configure call populate it. Data sources, list resources and actions use the same shape with their own request and response types.

When the version is read:

- The first Pro Configure in a provider instance reads the version through `pd.GetJamfProVersion(ctx)`, whatever its floor, because the build-target warning needs it. A successful read is cached on the `Data` value; an error is not, so the next call retries.
- Once the build-target warning has been considered, a Configure with an empty floor skips the read entirely.
- A configuration with no Pro construct never reads it.

#### Failure modes

| Condition | Construct with a floor | Construct with `""` |
|---|---|---|
| Version read fails (no Jamf Pro on the tenant, network or auth error) | Configure error "Failed to read Jamf Pro tenant version" | Configure passes; the first API call surfaces the real failure |
| Tenant version below the floor | `RequireMinJamfProVersion` error naming both versions | n/a |
| Tenant version unparseable | `RequireMinJamfProVersion` error quoting the raw string | no error |
| Tenant version below the build target | warning, once | warning, once |

#### `providerdata.Data` lives in its own package

`req.ProviderData` is a `*providerdata.Data`, defined in `internal/providerdata/`, **not** `internal/provider/`. It carries the SDK client, the configured scope and the cached Pro version.

- It is a separate leaf package because `internal/provider/` imports every resource package to register it; a resource importing `internal/provider` back would be an import cycle.
- Platform Services constructs do not use `ConfigurePro`. They type-assert `req.ProviderData.(*providerdata.Data)`, read `.Client`, and gate on integration scope with `RequireScope`.

### Provider overall minimum Jamf Pro version (advisory warning)

This is the provider build target, `ProviderMinJamfProVersion`, described under [§Minimum Jamf Pro version check](#minimum-jamf-pro-version-check). What users are told about it is in the registry guide, `templates/guides/versioning-and-support.md`.
