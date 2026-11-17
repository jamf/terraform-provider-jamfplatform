# Jamf Security Cloud

Authoritative: `STYLE_GUIDE.md` §Jamf Security Cloud Resource Naming — including
§Security Cloud shapes that recur and §Security Cloud Configure. `CLAUDE.md` carries the
orientation. Read both; this file is the build-order overlay.

## The two things most likely to be got wrong

**Configure is `ConfigureSecurityCloud`, never `ConfigurePro`.** Security Cloud is
continuously deployed, has no customer-tenant version, and a tenant can hold it without
holding Jamf Pro — so a Pro version fetch is both meaningless and fatal. There is no
`minJamfProVersion` in these packages.

**Entitlement is not authentication.** A perfectly valid integration is still refused with
`403 NOT_ENTITLED`. Translate the code into a named diagnostic rather than surfacing the raw
error.

## Diagnostics the server will not write for you

| Server says | Actually means | Point the diagnostic at |
|---|---|---|
| `403 NOT_ENTITLED` | tenant lacks the Security Cloud surface | the construct, naming the entitlement |
| `400 [INVALID_FIELD] Request body is missing or malformed.` | an enum value `jsc-dns` / `jsc-ztna` do not accept — **no field, no value named** | prevented at plan time; validate every enum |
| `422 VALIDATION_FAILED` | the same mistake on `uem-connect`, which leaks Jackson's message and *does* name the accepted values — **so this is not a namespace-wide shape** | same: prevented at plan time. Do not write a diagnostic against the code another construct observed |
| `422 CONNECTOR_MISCONFIGURED` "UEM is misconfigured" | on the activation profile deploy: a Jamf Pro group ID that does not exist, or belongs to the wrong kind of group for the chosen OS. Nothing is misconfigured | the group field, naming which kind of group the OS takes |
| `422 GATEWAY_NOT_FOUND` | a DNS zone written before its gateway exists | `name_servers`, not the zone |
| `409 CONFLICT` (bare) | something still references the object you are deleting | a destroy-ordering diagnostic naming the possible referrers |

The 409 is a Terraform destroy-ordering trap: removing the reference *and* destroying the
target in one apply lets Terraform sequence the destroy first. Say so, because the server
will not.

## Build every enum from the SDK's own helper — error codes included

Because an enum violation is unattributed, validate at plan time — and build **both** the
`OneOf` validator and the documented value list from the SDK's generated `*Values()` helper,
so they cannot drift from each other or from the API.

**The same rule governs the error codes in `mappings.go`,** and this is the half that gets
missed. Alias the SDK constant — `codeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled`
— and restate a literal only for a code the SDK genuinely lacks.

`ApiErrorItemCode` is the DNS namespace's error schema, so it carries no construct-specific
code (`GROUP_ALREADY_EXISTS`, `RESERVED_GROUP_NAME`, …) but **does** carry the generic
`INVALID_FIELD` and `NOT_ENTITLED`. A package that restates those alongside its own codes is
wrong in a way its own "the SDK carries none of these" comment conceals. That shipped twice.
Check each code individually against `ApiErrorItemCodeValues()`; do not reason about the set.

A deliberate subset stays hand-curated — `EmailMappingTypeValues()` is a superset spanning
every UEM vendor, so `uem_connect` narrows it per vendor on purpose — but its elements are
still SDK constants, not literals.

**Pin it with a test rather than trusting review.** Add an `enum_literals_test.go` calling
`internal/common/enumguard` (shape: `internal/resources/security_cloud/device_group/enum_literals_test.go`).
It parses the package's own `const` and `var` declarations, func-body `:=` bindings and any
vocabulary inlined into a `OneOf` call, then fails on every string literal the SDK already
provides — so it catches both a new mistake and a literal a future SDK release promotes into the
enum. Each code the SDK genuinely lacks goes in `Absent` **with its own reason**; a value that
merely shares a spelling with a different vocabulary goes in `Ignore` instead, because `Absent`
is re-checked against the SDK on every release and `Ignore` is not.

Full rule, provider-wide: STYLE_GUIDE §Enum values and error codes come from the SDK.

## Single-element arrays are scalars

Several fields are declared as arrays and rejected at any size but one (the IPsec cipher
suites' `encryption` / `integrity` / `dhGroups`, `left.subnets`). Model as a single value and
collapse at the boundary. A list only offers room the server refuses, and the refusal names
the field's size rather than the mistake.

## Read-only status timestamps stay out of the schema

A field that advances on every server-side re-evaluation (`status.updatedAt`) makes every
refresh report drift about something no configuration can act on. Omit it, say so in the state
builder's doc comment, and keep the fields that settle (`status.state`, `status.tunnelState`)
and those that never move (`createdAt`).

## Acceptance gating

Call `testhelpers.AccPreCheckSecurityCloud`, never bare `AccPreCheck`. It requires the operator
to **declare** that the configured scope is a Security Cloud one —
`JAMFPLATFORM_ACC_SECURITYCLOUD_{ENVIRONMENT,TENANT}_ID` set **and equal** to the corresponding
`JAMFPLATFORM_*` value. Unset, both set, or mismatched → skip. The equality check is what makes
the declaration load-bearing: a stale value from another tenant skips rather than running
against the wrong estate.

Neither variable is set in CI, so every Security Cloud acceptance test skips there. **Green CI
and zero coverage are indistinguishable** — do not report one as the other. Anything needing a
`tenantIds` value (gateways, grouped gateways) additionally cannot run under an
environment-scoped integration at all, because no API exposes an environment's tenants.

Untested, and worth stating in any PR that touches Configure: the Security Cloud surface under
`X-Environment-Id`. Every wire probe used a tenant-scoped integration;
`ConfigureSecurityCloud` admits both scopes on the strength of the spec. If `/securitycloud`
turns out not to answer under an environment header, the fix is dropping `ScopeEnvironment`
from that one call.
