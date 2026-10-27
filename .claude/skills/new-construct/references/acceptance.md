# Acceptance tests

Authoritative: `TESTING.md` §Writing Acceptance Tests — the coverage requirements there are
the contract, and `internal/resources/pro/enrollment_customization/resource_acceptance_test.go`
is the reference enumeration. Copy from it. This file adds the harness traps and the things
`TESTING.md` cannot say for you.

## The contract, in one line each

A single Create-only test is not sufficient. Collectively the suite must exercise: a
multi-step **Update round-trip** (every non-`RequiresReplace` attribute, a nested element add
**and** remove, a computed sibling re-resolving); an **`ExpectError` per declared cross-field
validator**; an **`ImportStateVerify` round-trip** with every `ImportStateVerifyIgnore` entry
justified in a comment; **one happy path per mutually-exclusive shape**; and **drift-recovery**
assertions for self-healing computed attributes; and, for a type with a collision check, a
`collision_acceptance_test.go` that creates the colliding object outside Terraform in
`PreConfig` and asserts the plan-time `ExpectError`, with a step showing the default settings
do not error for a strict-only type. Assert that the error fires: a key path the plan cannot
read makes the check skip silently, which a test that only expects success never notices. `//go:build acceptance` on line 1 or the file
leaks into the unit run.

## Harness traps

- **A `TestStep` cannot set both `Config` and `RefreshState`** — the framework refuses before
  running anything. A drift/disappears test is `{Config: cfg}` then
  `{PreConfig: deleteOutOfBand, RefreshState: true, ExpectNonEmptyPlan: true}`; the refresh
  step reuses the preceding config.
- **`ExactlyOneOf` emits two different summaries** — "Invalid Attribute Combination" when both
  are set, "Missing Attribute Configuration" when neither is. A regex matching one passes for
  the wrong reason on the other. Match the shared detail (`Exactly one of these attributes
  must be configured`).
- **Terraform wraps error output at ~80 columns**, so an `ExpectError` regex must not depend on
  whitespace at the wrap point. Anchor on no-space tokens.
- **Read the failure output before assuming a provider bug.** Six failures on the Security
  Cloud suite's first live run were all stale assertions — the printed state map showed the
  provider writing the correct values.
- **Never pipe a test run through `tail`** when logging in the background: the pipeline's exit
  status becomes `tail`'s, so a run looks green while the package you cared about never
  reported. Redirect the whole log, then grep it.
- **Do not cancel an in-progress acceptance run** to save time — a killed run skips
  `CheckDestroy` and orphans `tf-acc-*` fixtures across the shared tenant.

## Absence is not a result

A fixture that cannot find the tenant object it needs **skips** — it never infers, and it never
substitutes a guess. The same applies to gates: a Pro-only tenant is a legitimate environment
for a Security Cloud suite to skip in, not a failure. And a suite that skips everywhere still
prints green: report skips as skips.

## Async computed values do not round-trip

A `Computed` attribute the server populates asynchronously can read empty on a plain GET even
on a settled object, so it belongs in `ImportStateVerifyIgnore` alongside `timeouts` and
WriteOnly secrets. Do **not** add a readiness poll in `Read` to force it — for a GET-sensitive
value that turns a transient empty into a hard refresh error.

## Tenant-prerequisite fixtures

Some features only exist when the tenant is in a prerequisite state. Stand the prerequisite up
as an in-config fixture the subject `depends_on`, so the apply orders it first. A prerequisite
that needs no live connectivity can be a dummy needing no env var; one that needs real external
infra is env-gated and `t.Skipf`s when unset. Prefer a fixture that mutates the tenant
minimally and reversibly, and remember that many prerequisite singletons have state-only
deletes — teardown leaves the tenant in the fixture's state.

## Who runs them

The maintainer runs the acceptance suite against their tenant; hand off after unit tests and
lint are clean, with the exact command:

```sh
make testacc-run RUN=<regex> PKG=./internal/resources/<path>/
```

Before trusting CI's scope, compute it: `go run scripts/acctargets/main.go origin/main`.
