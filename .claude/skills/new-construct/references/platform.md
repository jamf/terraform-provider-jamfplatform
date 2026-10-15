# Platform Services

Blueprints, Compliance Benchmarks (cbengine), devices, device groups. The oldest constructs in
the provider and the loosest-gated.

## Configure

No helper. Hand-roll the type-assert on `*providerdata.Data`, then apply the per-construct
scope gate:

```go
resp.Diagnostics.Append(pd.RequireScope("jamfplatform_<name>",
    providerdata.ScopeEnvironment, providerdata.ScopeTenant)...)
```

Pass the allowed kinds in preference order — environment first, because that is the order the
diagnostic lists them in. **No version gate**: these target continuously-deployed microservices
with no customer-tenant version, so no `minJamfProVersion` and no `ConfigurePro`.

No SDK-endpoints annotation block either — that requirement is Pro / ProClassic only.

## The GA cutover is coming for the scope list

Blueprints and Compliance Benchmarks become **environment-only** at the Platform API GA, at
which point their `RequireScope` call sites drop `ScopeTenant`. That is the one-token narrowing
the per-construct gate was built for — see the `project_platform_api_ga_cutover` memory for the
four breaking changes in that cutover. Do not pre-emptively drop it; do not add a new call site
that would be awkward to narrow.

## Blueprint legacy payloads are validated against Apple's schema

Anything touching `component_blocks[].legacy_payloads` goes through
the SDK's `appleschemas/profiles`. The rules there are wire-established and asymmetric — an
unknown *payload type* is rejected and matched case-sensitively; an unknown *key* is silently
discarded; a key differing only in case is silently stored under Apple's spelling; enum and
range constraints are **not** enforced. `CLAUDE.md` §Apple profile schemas has the orientation;
do not re-derive it by probing.
