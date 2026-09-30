# Testing

This document covers the testing strategy and instructions for the Terraform Provider for Jamf Platform.

## Test Categories

| Category               | Build Tag    | Requires API | Command                                                                                       |
|------------------------|--------------|--------------|-----------------------------------------------------------------------------------------------|
| Unit                   | (none)       | No           | `make test`                                                                                   |
| Unit (tooling)         | `acctargets` | No           | `make test-scripts` — `scripts/acctargets`, which `go test ./...` cannot see behind its build tag |
| Acceptance (all)       | `acceptance` | Yes          | `make testacc`                                                                                |
| Acceptance (changed)   | `acceptance` | Yes          | `make testacc-changed` (changed packages + their transitive dependents)                       |
| Acceptance (targeted)  | `acceptance` | Yes          | `make testacc-run RUN=<regex> PKG=<package>` (e.g. `RUN=TestAccResource_ProSite_Basic PKG=./internal/resources/pro/site/...`) |
| Acceptance (functions) | `acceptance` | No           | `make testacc-run RUN=TestAccFunction PKG=./internal/functions/...` — provider functions are offline; their tests need a Terraform binary but no tenant (they call `testhelpers.AccPreCheckOffline`, which sets `TF_ACC` without the credential gate) |

### Unit Tests

Unit tests validate schema definitions, metadata, plan modifiers, state migration, type flatteners/expanders, helper functions, and client HTTP behaviour using a mock server. They do not require network access or API credentials.

```bash
go test -v -cover -count=1 ./...
```

### Acceptance Tests

Acceptance tests create, read, update, and delete real resources against a live Jamf Platform tenant. They are gated behind the `//go:build acceptance` build tag so they never run by default.

```bash
export JAMFPLATFORM_BASE_URL="https://us.api.jamfcloud.com"
export JAMFPLATFORM_CLIENT_ID="your-client-id"
export JAMFPLATFORM_CLIENT_SECRET="your-client-secret"
export JAMFPLATFORM_ENVIRONMENT_ID="your-environment-id" # preferred; or JAMFPLATFORM_TENANT_ID (legacy), never both

# Jamf Security Cloud tests additionally require you to DECLARE that the scope above
# is a Security Cloud one. The value must match the scope actually configured; unset
# or mismatched and every Security Cloud test skips rather than failing.
export JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID="$JAMFPLATFORM_ENVIRONMENT_ID"

# The AD CS OUTBOUND tests need a Jamf Pro API client the provider cannot create
# (the API-client endpoints were withdrawn at the Platform API GA — make one in
# Jamf Account and paste its Client ID here). Unset and those tests skip.
export JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID="client-id-uuid-from-jamf-account"

go test -v -cover -count=1 -tags=acceptance -p=1 ./...
```

**Flags explained:**

- `-tags=acceptance` includes files with the `//go:build acceptance` tag.
- `-p=1` runs packages sequentially to avoid resource naming conflicts.
- `-count=1` bypasses the Go test cache (useful for re-runs).

**Targeted re-runs:** `make testacc-run` accepts `RUN=<Go -run regex>` and `PKG=<package path>` overrides; extra flags can be appended via `TESTARGS`. Example:

```bash
make testacc-run \
  RUN=TestAccResource_ProNetworkSegment_Basic \
  PKG=./internal/resources/pro/network_segment/...
```

**Changed-scope runs:** `make testacc-changed` runs acceptance tests only for the
packages your branch touches plus everything that transitively depends on them.
It shells out to `scripts/acctargets`, which diffs against `origin/main` (override
with `BASE=<ref>`) and walks the package import graph:

- Change one resource (e.g. `internal/resources/pro/category/`) → only that
  package's acceptance tests run.
- Change a shared helper (e.g. `internal/common/scope/`) → that package **and**
  every consumer (`policy`, the apps, `user_group`, the VPP resources, the
  config profiles, the advanced searches, …) run.
- **Add** to a hub package — a new `providerdata` helper, a new `testhelpers`
  fixture, a new shared validator — → only that package. Changed Go files are
  compared declaration by declaration, and no unchanged package can reference a
  declaration that did not exist at the base ref. Any package that gained such a
  reference changed too, and selects itself.
- **Modify or remove** an existing declaration in one of those hubs → the full
  fan-out, because that genuinely can reach every consumer.
- Register or unregister a construct in `internal/provider/provider.go` → only
  the packages whose constructors moved. Any other edit to that file (a
  `Configure` change, a new provider attribute) falls back to the full fan-out.
  Reordering a list changes no constructor and so selects nothing.
- Change a global file — `go.mod`/`go.sum`, the `GNUmakefile`, anything under
  `.github/workflows/`, or `scripts/acctargets/` itself — resolves to the full
  suite (`./...`).

Every uncertainty resolves to the wider answer: an unparseable revision, a
declaration that moved, a registration list that is no longer a plain list. Over-
running costs time; under-running costs a regression. `make test-scripts` runs the
tool's own unit tests, which `go test ./...` cannot see (the tool is behind the
`acctargets` build tag so it stays out of the provider binary).

The graph deliberately **cuts `internal/provider`'s out-edges**. The provider
imports every resource package purely to register it, and `testhelpers` imports
the provider, so without the cut every change would look like it touches the
whole suite. The cut keeps real edges (a package's own deps, shared helpers,
cross-package test fixtures) and drops only the registration fan-out.

## Test File Layout

Test files live alongside the code they test, following Go convention. The Jamf Platform API client lives in the external SDK `jamfplatform-go-sdk` and its tests live in that repo — they are not part of this provider's test suite.

```
internal/
├── common/
│   ├── helpers/                          # helpers_test.go, dynamic_json_test.go, ids_test.go, pro_version_test.go
│   ├── filters/filters_test.go
│   └── scope/                            # builders_test.go, schema_test.go, validators_test.go
├── testhelpers/                          # Acceptance fixtures (mock server, real-client builder, shared fixtures)
├── actions/device/schema_test.go
├── functions/                            # Provider-defined functions (offline — no tenant needed)
│   ├── mobileconfig/                     # assemble_test, function_test (Run seam via real types.Dynamic), function_acceptance_test
│   └── mcx_forced_payload/               # render_test, function_test, function_acceptance_test
├── resources/
│   ├── blueprints/blueprint/             # schema_test, helpers_test, input_builders_test, state_builders_test, state_upgrader_test, resource_acceptance_test
│   ├── cbengine/benchmark/               # schema_test, input_builders_test, state_builders_test, resource_acceptance_test
│   ├── device_group/                     # schema_test, helpers_test, input_builders_test, state_builders_test, state_upgrader_test, resource_acceptance_test
│   ├── devices/                          # schema_test, datasource_acceptance_test (data-source-only)
│   └── pro/                              # Same file shapes per resource, plus marshal_smoke_test for ProClassic XML resources
│       ├── inventory/<resource>/         # category, site, building, department, network_segment, ibeacon, dock_item, directory_binding, disk_encryption_configuration, package, icon, printer
│       ├── policies/{policy,script}/
│       ├── configuration_profiles/macos_configuration_profile/  # + helpers_corpus_test (build tag profile_corpus)
│       ├── settings/self_service_plus_settings/
│       └── users/user_group/
```

Each leaf folder follows the `*_test.go` (unit) + `resource_acceptance_test.go` / `datasource_acceptance_test.go` (acceptance, `//go:build acceptance`) split. Acceptance file names are flat — no per-scenario splits.

### Naming conventions

- `*_test.go` — unit tests (no build tag).
- `*_acceptance_test.go` — acceptance tests (with `//go:build acceptance`).
- `schema_test.go` — schema validation tests present in every resource/data source package.

## Writing Unit Tests

Unit tests use Go's standard `testing` package. Client tests use `httptest.NewServer` to mock the Jamf Platform API.

```go
func TestMyFunction(t *testing.T) {
    // arrange, act, assert
}
```

Schema validation tests use the `terraform-plugin-testing` framework:

```go
func TestSchemaValidity_Resource(t *testing.T) {
    ctx := context.Background()
    schemaResp := resource.SchemaResponse{}
    NewResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)
    assert.False(t, schemaResp.Diagnostics.HasError())
}
```

## Writing Acceptance Tests

Every acceptance test file must include the build tag:

```go
//go:build acceptance

package mypackage_test
```

### Resource acceptance tests

Resource acceptance tests use the `terraform-plugin-testing` framework with `resource.TestCase`:

```go
func TestAccResource_MyResource(t *testing.T) {
    testhelpers.AccPreCheck(t)

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
        CheckDestroy:             testAccCheckMyResourceDestroy(t),
        Steps: []resource.TestStep{
            {
                Config: `resource "jamfplatform_my_resource" "test" { ... }`,
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttrSet("jamfplatform_my_resource.test", "id"),
                ),
            },
        },
    })
}
```

**Requirements:**

- Call `testhelpers.AccPreCheck(t)` at the start of every test.
- Always provide a `CheckDestroy` function that verifies resources are removed after the test.
- Use unique resource names per test to avoid conflicts (e.g. include the test name or a suffix).

**Coverage requirements (every new resource):**

A single Create-only test is **not** sufficient. The acceptance suite for a resource must collectively exercise:

1. **An Update round-trip — required.** At least one test must drive a multi-step `resource.TestCase` that mutates the resource in place. Every Optional/Required attribute that is *not* `RequiresReplace` must be exercised across steps, including:
   - At least one nested-block / list-attribute element add **and** remove (proves list reconciliation, not just element-content mutation).
   - At least one server-derived computed sibling (hash, URL, epoch) re-resolves correctly after the change.
   - This is the highest-value test in the file — it catches `mergePlanIntoServerState`, `UseStateForUnknown`, and lossy-PUT regressions that single-step Create tests cannot.
2. **A negative case per declared cross-field validator.** Every `ConflictsWith`, `AlsoRequires`, `OneOf`, and custom plan-time validator on the resource needs an `ExpectError` test. The positive case alone is not enough — silent validator removal is the easiest regression to ship.
3. **An import round-trip** via `ImportStateVerify: true`. List `ImportStateVerifyIgnore` for any attribute the importer legitimately cannot recover (e.g. write-only inputs, local-path fields) and justify each entry in a code comment.
4. **All "shape" branches** — for any resource with mutually exclusive sub-blocks (auth modes, scope variants, payload kinds), include at least one happy-path test per shape so each input-builder branch is wire-validated.
5. **Server-side drift recovery** where applicable. For self-healing resources (image uploads, hash convergence, lossy GETs), include a multi-step test that mutates a user input and asserts the Computed echo attributes update — not just that they remain `Set`.

The reference enumeration of these patterns lives in `internal/resources/pro/enrollment_customization/resource_acceptance_test.go`. Copy from there when scaffolding a new resource's acceptance file.

### Client acceptance tests

Client-level acceptance tests call API methods directly to validate the client layer:

```go
func TestAcceptance_MyResource_Create(t *testing.T) {
    c := testhelpers.NewAcceptanceClient(t)
    ctx := context.Background()
    // create, verify, cleanup via t.Cleanup
}
```

### Shared fixtures

The `testhelpers.RequireSmartGroupFixture(t)` function provides a shared smart group for tests that need a device group target. The fixture is created once per test binary and cleaned up via `TestMain`.

### Jamf Pro acceptance tests

Pro resources use the same `*jamfplatform.Client` and the same `JAMFPLATFORM_*` credentials as Platform Services resources. Tenant data isolation conventions:

- Use the `tf-acc-` prefix on every resource created.
- Pro resources have richer dependency graphs (a policy needs a category, smart group, script, package). Use `t.Cleanup` for dependency-ordered teardown — register cleanups in reverse-creation order.
- Shared fixtures that span multiple test files live in `internal/testhelpers` and are cleaned up via `TestMain` (precedent: `RequireSmartGroupFixture`). Add new `Require*Fixture` helpers as needed.
- Tests must be independent — never assume another test has run first.

**Acceptance test files MUST declare `//go:build acceptance` on line 1** or they leak into the unit run.

### Tenant-prerequisite fixtures (server-state gates)

Some Pro features can only be created when the tenant is in a prerequisite state — e.g. an enrollment-customization **SSO pane** requires SAML configured (`403 [INVALID_STATE] : SAML must be configured`), and a `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING` extension attribute requires LDAP configured (`400 [INVALID_CONTENT] ... if LDAP is not configured`). Stand the prerequisite up as an in-config **fixture resource the subject `depends_on`**, so the apply orders the prerequisite first. Two flavours:

- **Dummy fixture, no gating** — when the prerequisite resource doesn't validate live connectivity, build a placeholder. `jamfplatform_pro_ldap_server` does not verify the LDAP connection, so a dummy (`Open Directory`, fake `ldap.acc-anon.example.com:389`, `authentication_type = none`) configures LDAP with no real server and **needs no env var**. Reference: `ceaDSAM`/`mdeaDSAM`.
- **Real-credential fixture, env-gated** — when the prerequisite needs real external infra (a SAML IdP metadata URL), gate the test on an env var and **skip** when unset (`t.Skipf`). Reference: the enrollment-customization SSO-pane tests gate on `JAMFPLATFORM_ACC_PRO_SSO_IDP_URL`; the `sso_settings` suite owns the same var.
- **Pre-existing tenant object, env-gated** — when the prerequisite is an object the provider *cannot* create, name it by env var and skip when unset. `jamfplatform_pro_pki_adcs` in `OUTBOUND` mode needs a Jamf Pro API client, and API clients and roles were withdrawn from the Platform API at GA (they are created in Jamf Account), so the OUTBOUND tests take the client's UUID from `JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID` and skip without it.
- **Estate capability the suite must not create — probe, then skip** — when the prerequisite is tenant configuration the suite cannot stand up *or tear down* safely, read it and skip on what the server reports. `testhelpers.RequireJCDSUploads` gates every package-upload test on the tenant having a **Jamf Cloud** distribution point that accepts direct uploads: without one the upload's verification poll can never converge, so the test burns the resource's whole create timeout (30 minutes apiece, two hours across the package suite in the 2026-09-03 `pro` lane) and then reports a timeout that reads like a provider defect. A cloud distribution point cannot be created as a fixture either, because **deleting one permanently wipes every package, in-house app and eBook hosted in Jamf Cloud** — the same reason `EnsurePrincipalCloudDistributionPoint` only ever PATCHes the `master` flag. Probe-and-skip is honest here because both endpoints answer *positively* (`cdnType` names the CDN, `NONE` when there is none; the upload-capability record states whether a direct upload is possible), so nothing is inferred from an absence — and a read that **fails** is a defect and fails the test. Accept the consequence knowingly: on an estate with no cloud distribution point the lane reports green with the whole upload path — hash-convergence poll, size-zero retry, metadata-PUT ordering — unrun, and no `require` token promotes that to a failure, because a tenant without a distribution point is a legitimate estate rather than a missing secret. A green `pro` lane is therefore not evidence that package uploads work; run the package suite against a JCDS-backed tenant before trusting a change to that path.

Two cautions: (1) prefer a fixture that **mutates the tenant minimally and reversibly** — e.g. SSO uses `OIDC_WITH_SAML`, not pure SAML, so Jamf ID admin login keeps working. (2) Many prerequisite singletons (`sso_settings`, `computer_inventory_collection_settings`) have **state-only deletes**, so teardown leaves the tenant in the fixture's state — idempotent on re-run, but document it and don't let other suites assume a clean SSO/inventory baseline.

### Do not `ImportStateVerify` async server-computed values

A `Computed` attribute that the server populates **asynchronously** (e.g. `computer_prestage_enrollment.profile_uuid` — the "information out of date" window) can read empty on a plain `GET` even on a settled object, so it does not reliably round-trip on import. Add it to `ImportStateVerifyIgnore` (alongside `timeouts` and WriteOnly secrets). It is server-generated, not user-settable, so verifying it on import adds no coverage. **Do not** try to force it to populate inside `Read` (e.g. a readiness poll) — for a GET-sensitive value that can make refresh worse (a transient empty becomes a hard timeout error).

### Guard configuration generation with `testhelpers.GenerateConfigStep`

`terraform query -generate-config-out` and `terraform plan -generate-config-out` write state back
out as configuration, so a `Read` that commits a value the resource's own schema or validators
**refuse** produces a file that cannot be planned. [Issue
#379](https://github.com/jamf/terraform-provider-jamfplatform/issues/379) collected seven
of those across six resources, and none was reachable from an ordinary acceptance test: a step that
applies a configuration and reads it back can only produce a state the configuration already
described. The failures live in states the test never creates — a UEM connector authenticating as a
platform tenant, an SMTP server nobody has set up, a patch title with no packages assigned — and
generation is the one workflow whose whole purpose is meeting them.

Append `testhelpers.GenerateConfigStep(resourceAddr)` to a lifecycle test after at least one
`Config` step has applied, so there is a real object to adopt:

```go
Steps: []resource.TestStep{
    {Config: unconfiguredShape, Check: …},
    testhelpers.GenerateConfigStep(resourceAddr),
},
```

It asserts three things in one pass: Terraform generates configuration from the object's state; the
generated configuration is then **planned**, which fails on any schema or validator diagnostic; and
the plan must be a **no-op import**. That last part is why the step is a plan and not a
`terraform validate` — validate would accept a generated value the very next plan would want to
change. The construct needs an `IdentitySchema`, since the step uses
`resource.ImportBlockWithResourceIdentity`, which is how Terraform's own query-driven generation
addresses an object.

**Two limits worth knowing before relying on it.** A `Query: true` step ignores `GenerateConfig`
entirely — `terraform-plugin-testing` honours it only on the import-block path — so a **list
resource that hydrates on its own code path** is not covered by this step at all. `app_installer`,
`patch_software_title` and `cloud_identity_provider` each build list state through a function the
resource `Read` never calls, and those are guarded instead by `querycheck` assertions on the
hydrated object (`include_resource = true`) that name the specific attribute which must not come
back empty. And the step needs the object to **exist on the estate**, so it cannot be a generic
sweep over every construct: it is applied per resource, to the state that actually broke.

Prefer fixing the resource over passing `ExpectNonEmptyPlan` — a non-empty plan here is the defect
the step exists to report.

### Profile-corpus regression test (opt-in build tag)

`internal/resources/pro/macos_configuration_profile/helpers_corpus_test.go` is gated by `//go:build profile_corpus`. It iterates a 200-profile mobileconfig corpus under `testing/profile_roundtrip/` (gitignored, developer-machine-only) and asserts the mask-and-compare diff suppression is stable. Run with:

```bash
go test -tags profile_corpus ./internal/resources/pro/macos_configuration_profile/...
```

Not part of CI. Regenerate the corpus before running by replaying `/tmp/sample_titles.py` + `/tmp/roundtrip.py` against a tenant. Background on the diff classes the mask covers: [STYLE_GUIDE.md §Configuration profile payload diff suppression](STYLE_GUIDE.md#configuration-profile-payload-diff-suppression-mask-and-compare).

### CI scaling

CI scales acceptance along two independent axes: **fewer packages** per run, and
**per-product lanes** that run at the same time.

Scoping comes first. The `plan` job in `.github/workflows/acceptance.yml` runs
`scripts/acctargets` to resolve the change set to the affected packages, so a PR
touching no acceptance package runs nothing. The scheduled run, a manual
`workflow_dispatch`, and any PR carrying the `full-acceptance` label take the
whole suite instead, so nothing hides behind scoping forever.

`scripts/acclanes` then splits that scope into lanes, reading
`.github/acceptance-lanes.json`. A lane is a product space, and membership is a
package path prefix:

| Lane | Packages | Credentials | `require` |
|---|---|---|---|
| `account` | `internal/{resources,actions}/account/` | organization | `organization` |
| `securitycloud` | `internal/{resources,actions}/security_cloud/` | environment | `securitycloud` |
| `aigovernance` | `internal/resources/ai_governance/` | environment | `aigovernance` |
| `platform-env` | `internal/resources/{blueprints,cbengine}/` | environment | `environment` |
| `pro-tenant` | `internal/provider/`, `internal/resources/pro/tenant_id/`, `internal/resources/devices/`, `internal/resources/device_group/` | tenant | `pro-tenant` |
| `pro` (default) | everything unclaimed | environment | `platform` |

`protect`, `school` and `android` are reserved with `planned: true` and must
match zero packages; the moment one matches, both `acclanes` and the conformance
test fail and name the wiring to finish.

Two of the `pro-tenant` lane's packages are claimed for a reason that is not
product shape. `internal/providerdata/scopes.go` derives every construct's
permitted scopes from the SDK privilege registry and then widens three Platform
families — `Platform devices`, `Platform device groups`, `Platform device
actions` — back to tenant scope in its `gatewayWidenings` table, because the
Platform API GA deleted `X-Tenant-Id` from six Platform specs while the gateway
went on serving it. A widening asserts that those constructs work **end to end**
on a credential the spec says should not reach them, which no status code can
show, so it holds only while some lane actually sends `X-Tenant-Id` at them:
hence `internal/resources/devices/` and `internal/resources/device_group/` — the
only Platform device packages that hold acceptance tests — run on the tenant
credential rather than falling through to the environment-scoped `pro` lane. The
environment path for the same constructs stays covered incidentally, by the
blueprint, benchmark, policy, class, webhook, patch-policy, app-installer and
device-action tests that each build or read one. `internal/actions/device/` is
deliberately left in `pro`: its commands are destructive (erase, unmanage) and
were never run under tenant scope, so the `Platform device actions` widening
rests on its wire probe alone.

**Preview the split locally with `make acclanes-preview`**, which prints the
matrix the `plan` job would build for your current change set — lane, package
count, credential set and `require` token per lane. It is the companion to
`make testacc-changed`: that target says which packages run, this one says which
lane each lands in and on whose credentials, which is what an edit to
`.github/acceptance-lanes.json` needs checking against. `BASE=<ref>` overrides
the diff base as it does for `testacc-changed`, and `SCOPE=./...` previews the
full suite:

```bash
make acclanes-preview                # this branch's change scope
make acclanes-preview SCOPE=./...    # the whole suite
```

Each lane also carries its own `timeout_minutes` — 240 for `pro`, 30-90 for the
rest — because `go test -timeout` cannot be the lane's ceiling: that flag is per
test *binary*, and `go test` builds one per package, so at `pro`'s 110 packages
it bounds a single package and lets the run continue. Without a job ceiling the
only real bound is GitHub's 6-hour default, and a wedged lane holds its
concurrency group for all six of them.

Only `pro` carries the estate lock, so only `pro` serialises against itself. The
others authenticate with a different credential set or a different entitlement
declaration and share no fixtures with it, which is the whole reason to split:
under the previous single `acceptance-tenant` group a 38-test Jamf Account run
queued behind a ~2.5 h Pro suite. Within a lane the suite still runs serially
(`-p=1`) — parallel runs against one estate cause naming and ID collisions — so
inside a lane the scaling is still "test fewer packages".

A running suite is **never** cancelled. There is deliberately no workflow-level
`concurrency` block: a workflow-level group with `cancel-in-progress: true`
kills a running acceptance job, and cancelling mid-suite SIGKILLs `go test` so no
`CheckDestroy` or `t.Cleanup` handler runs, orphaning real objects on a shared
estate. Superseded runs queue instead, and a `Skip if superseded` step lets a
queued run for a stale commit exit in seconds once it reaches the front.

### Benchmark-specific considerations

CBEngine benchmarks deploy asynchronously. The benchmark must reach `SYNCED` state before it can be safely deleted — deleting while in `PENDING` state causes the benchmark to get stuck in `DELETING`. Use `testhelpers.EnsureBenchmarkDeletedByID` or `testhelpers.EnsureBenchmarkDeleted` for cleanup, which handle sync-waiting automatically.

## CI/CD

Two workflows, neither calling the other.

**`.github/workflows/integration-tests.yml`** — PR CI, on PRs to `main` and via
`workflow_dispatch`. Cheap, hermetic, credential-free.

| Job | What it does | Gating | Timeout |
|---|---|---|---|
| `build` | `go build` + `golangci-lint run` | — | 5 min |
| `generate` | Runs `make generate`, fails if `git diff` is non-empty | — | default |
| `vet` | `go vet` untagged, `-tags acceptance`, and `-tags acctargets,acclanes` over `scripts/`; formatting and `go fix` drift in all three build contexts; and the meta-check that the acceptance credentials sit on the step that runs `go test` | — | 10 min |
| `unit` | `go test -v -race -cover -count=1 ./...` | Needs `build` | 10 min |
| `scripts` | `make test-scripts` — `acctargets` and `acclanes`, each behind its own build tag and so invisible to `./...` | Needs `build` | 10 min |

**`.github/workflows/acceptance.yml`** — the acceptance pipeline, on PRs to
`main`, a `0 2 * * *` cron and `workflow_dispatch`. It plans its own scope, so it
has no caller.

| Job | What it does | Gating | Timeout |
|---|---|---|---|
| `plan` | `go vet -tags acceptance ./...`, then `scripts/acctargets` → `scripts/acclanes` → the lane matrix | — | 15 min |
| `acceptance` | One job per lane, `fail-fast: false`, `-p=1`, in the GitHub `acceptance` environment, against Terraform `1.15.*`. Ends with a per-lane summary that **fails** a lane which planned packages and ran no tests | Needs `plan`; skipped when the matrix is empty | 6 h (default) |
| `acceptance-gate` | The single required check for branch protection. Asserts the result rather than inferring it: a skipped matrix passes only when the plan said there was nothing to run | Needs `plan` + `acceptance`, `if: always()` | default |

Branch protection should require **`Acceptance`** (the `acceptance-gate` job),
not the per-lane checks — those are named after the lane and change whenever a
lane is added.

Three things went when the reusable workflow did. `acceptance-full.yml` is gone,
its cron absorbed into `acceptance.yml`. The `organization_scope` input and both
`*-account` caller jobs are gone: they existed only because
`AccPreCheckAccount` requires **both** scope variables unset while `AccPreCheck`
requires at least one set, and one env block cannot satisfy both — the account
lane now *is* the organization-scope run. And the `needs: unit` edge is gone,
replaced by `plan`'s own `go vet -tags acceptance ./...`, because a
cross-workflow dependency cannot exist for a workflow that also runs on a
schedule.

### `JAMFPLATFORM_ACC_REQUIRE`: a skip locally, a failure in CI

Unset credentials must **skip** locally — a contributor with no estate has to be
able to run `make testacc`. In a pipeline that wired the secret, absence means
the secret is missing or misnamed, and a skip there is invisible: the package
prints `ok` and the check goes green having asserted nothing.

`JAMFPLATFORM_ACC_REQUIRE` closes that. The matrix sets it per lane from
`matrix.require`, and `accrequire.SkipOrFailUnset` promotes that lane's
unset-credential skips to `t.Fatalf`. The unit is one token per lane, not a
boolean, so the `pro` lane never fails for an organization secret it does not
use. Credentials that were supplied and **refused** always fail, whatever the
token — a different condition, reported by `accrequire.CredentialRejectedMessage`
with the timestamp, instance URL and runner egress IP that Jamf Support asks for
and that cannot be recovered once the runner is gone.

Locally, to reproduce a lane exactly, export that lane's credential set and its
`require` token:

```bash
JAMFPLATFORM_ACC_REQUIRE=securitycloud make testacc-run \
  RUN=TestAccDataSource_SecurityCloudContentCategories PKG=./internal/resources/security_cloud/content_categories/
```

### The lane table is checked, not trusted

`internal/conformance/acc_lanes_test.go` carries **no build tag**, so it runs in
`make test` with no credentials and no network. It asserts that each package's
lane agrees with the precheck helper its tests actually call; that every lane's
`require` token is claimed by some precheck and vice versa (the two vocabularies
differ — lane `account` uses token `organization`, lane `pro` uses `platform` —
and a one-character disagreement would be silent *and* green); that no two lanes
claim the same package and no prefix is dead; that a `planned` lane matches
nothing; and that any test reaching credentials outside a precheck helper is
allow-listed by name with its reason. A misfiled test therefore fails at PR
time rather than on a live estate.

### Required GitHub Secrets

Bound to the `acceptance` environment:

Secret names follow `JAMFPLATFORM_ACC_<PRODUCT>_<SCOPE>_<FIELD>`, matching
`jamfplatform-go-sdk` — verbatim where that repo already names the same secret, so
one value serves both. The product token appears only where the scope is
product-bound: tenant is per-product, while environment and organization span a
customer's tenants.

Note what that does **not** cover. `JAMFPLATFORM_BASE_URL`, `_CLIENT_ID`,
`_CLIENT_SECRET`, `_TENANT_ID` and `_ENVIRONMENT_ID` are the **provider's own**
configuration — read by the provider schema at Configure and documented for users,
so they are public API and cannot be renamed. The alignment therefore happens at
the **secret** layer: the secret carries the aligned name, and `acceptance.yml`
maps it onto the provider's variable, per lane, by dynamic indexing on
`matrix.secret_prefix`. That mapping is the one deliberate divergence from the SDK
and is commented where it happens.

**Credential sets** — three, one per scope. `acceptance.yml` selects one per lane.

| Secret | Description |
|---|---|
| `JAMFPLATFORM_ACC_ENVIRONMENT_BASE_URL`, `_CLIENT_ID`, `_CLIENT_SECRET`, `JAMFPLATFORM_ACC_ENVIRONMENT_ID` | Platform-environment scope, the workhorse. Wire-probed 2026-09-03: reaches `pro`, `blueprints`, `compliance-benchmarks`, `ai/governance` **and** `securitycloud`. Authenticates the `pro`, `platform-env`, `aigovernance` and `securitycloud` lanes |
| `JAMFPLATFORM_ACC_PRO_TENANT_BASE_URL`, `_CLIENT_ID`, `_CLIENT_SECRET`, `JAMFPLATFORM_ACC_PRO_TENANT_ID` | Tenant scope, Jamf's legacy method for targeting an integration without a platform environment. Kept for **coverage, not data**: it authenticates the `pro-tenant` lane alone, because `tenant_id` is a supported public surface of this provider and a regression in the `ScopeTenant` path would otherwise ship unnoticed. Wire-probed 2026-09-03: reaches `pro` only — blueprints, benchmarks, AI Governance and Security Cloud all answer `403 BAD_PERMISSIONS`, and it names a **different** Jamf Pro tenant from the environment credential's |
| `JAMFPLATFORM_ACC_ORGANIZATION_BASE_URL`, `_CLIENT_ID`, `_CLIENT_SECRET` | Organization-management scope, for the `account` lane. Three members, not four: an organization request carries **no scope header** — the gateway resolves the organization from the token — so there is no ID to send. The base URL must be the **US gateway**; `/sso/v1` is absent from the EU one |

**Entitlement declarations and fixtures.**

| Secret | Description |
|---|---|
| `JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID` | Declares that the configured `JAMFPLATFORM_ENVIRONMENT_ID` belongs to a Jamf Security Cloud tenant. Must equal it. Part of the `securitycloud` lane's `require` token, so in that lane unset or mismatched **fails**; outside a lane it skips. A separate secret rather than a copy of the scope value on purpose — writing the scope inline would satisfy the equality check by construction and assert an entitlement nobody verified |
| `JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID` | Same, for `JAMFPLATFORM_TENANT_ID`. Set at most one of these two |
| `JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID` | Declares that the configured environment holds Jamf AI Governance. Must equal `JAMFPLATFORM_ENVIRONMENT_ID`. Part of the `aigovernance` lane's `require` token, so in that lane an unset value **fails** rather than skipping — it was referenced by no workflow at all before this pipeline, and all 18 AI Governance tests skipped green on every run |
| `JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID` | Declares that the organization credentials really are organization-scoped. Nothing can be compared against it — an organization request sends no scope header, so no `JAMFPLATFORM_*` variable holds the organization — which is exactly why it is declared: without it, a Pro-only credential set with both scope variables blank looks identical to a real organization integration. Provider-only; the SDK's organization client needs no ID by design |
| `JAMFPLATFORM_ACC_ORGANIZATION_SSO_VERIFIED_DOMAIN`, `JAMFPLATFORM_ACC_ORGANIZATION_SSO_UNVERIFIABLE_DOMAIN` | Optional Jamf Account SSO domain fixtures: a real, already-verified domain the operator owns, and a throwaway `.example` one that can never verify. Each covers a verification outcome the suite cannot manufacture. Unset → those two tests skip |
| `JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID` | Client ID (UUID) of a pre-existing Jamf Pro API client, created in Jamf Account, permitted to read and update AD CS certificate jobs — the privileges Jamf Pro called *Read AD CS Certificate Jobs* and *Update AD CS Certificate Jobs* before the Platform API GA, which this provider has no Jamf Account permission recorded for. Unset → the two `pki_adcs` OUTBOUND tests skip |

### Jamf Security Cloud coverage is opt-in, and partly tenant-scope-only

`JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID` **is** now set in CI, wire-verified
against the environment credential on 2026-09-03 (`/securitycloud` categories and
`dns/zones` both answered 200), and it is part of the `securitycloud` lane's
`require` token — so that lane can no longer skip green.

The tenant form is still unset, and nothing depends on it any more. A ZTNA gateway's
create requires `tenantIds`, and the tests leave `tenant_ids` unset for the provider to
default. `acceptance.yml` still carries the four dedicated Security Cloud tenant
credential lines **commented out**, for covering the tenant-scope path itself.

The same reading applies, on a much smaller scale, to `JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID`:
it is not set in CI either, so the two AD CS `OUTBOUND` tests skip there. They used to
mint their own API client and cannot any more, so this is coverage that moved from
self-provisioned to operator-provided rather than coverage that was removed.

Turning them on is just setting `JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID` to the same
value as `JAMFPLATFORM_TENANT_ID`. The gate requires the two to match, so a value
left over from a different tenant skips rather than running against the wrong estate.

**Under an environment-scoped integration the ZTNA gateway tests need a UEM Connect
integration on the environment.** `tenantIds` is required on every gateway and grouped
gateway, and the tests leave `tenant_ids` unset, so the provider defaults it to the Security
Cloud tenant's own ID. Under environment scope the only place the Jamf API reports that ID is
the UEM Connect connector's `customerId` (wire-probed 2026-09-30, with the connector deleted
and recreated), so an environment without one fails the gateway tests with the provider's
named `tenant_ids` error. The internet-gateway tests also need a dedicated IP entitlement: an
environment allowed none answers every create with `409 DEDICATED_IPS_LIMIT` ("0 of 0"), and
those tests fail rather than skip. Custom DNS zones and the shared-gateway catalogue have no
such field and are unaffected.

The whole Security Cloud surface also answers under `X-Environment-Id`: the lane ran
green there on 2026-09-30 apart from the dedicated-IP entitlement above, so
`providerdata.ConfigureSecurityCloud` admitting both scopes now rests on a run as well as
the spec.

### Jamf Account needs an organization-scoped integration, and therefore its own lane

The `jamfplatform_account_*` family is reachable only under **organization scope**, which
sends no scope header at all. So `testhelpers.AccPreCheckAccount` deliberately **inverts**
`AccPreCheck`'s requirement: it skips unless *both* `JAMFPLATFORM_ENVIRONMENT_ID` and
`JAMFPLATFORM_TENANT_ID` are **unset**, where `AccPreCheck` — which every other family
routes through — skips unless at least one of them *is* set. A scope header of either kind
makes `providerdata.ConfigureAccount` refuse every construct in the family at Configure, so
the honest outcome is a skip rather than a failure, and the precheck says which variable to
unset.

The two preconditions cannot hold in one environment block, so one `go test` invocation
cannot cover both. That used to be expressed as an `organization_scope` workflow input plus a
duplicate caller job per trigger (then named `acceptance-account` and `full-account`), each branch of the
env block carrying a `|| ''` because the unselected side otherwise reached the environment as
the string `"false"`. The lane matrix says it once instead: the **`account` lane** *is* the
organization-scope run. It matches `internal/{resources,actions}/account/`, authenticates
with the `JAMFPLATFORM_ACC_ORGANIZATION_*` credential set — for which no `_ID` secret exists,
so both scope variables resolve empty by construction — and holds no estate lock, because it
shares no fixture with Jamf Pro and has no reason to queue behind it.

Two further conditions skip the family, both legitimate:

- **A non-US base URL.** `/sso/v1` is served only from the US gateway; on the EU gateway even
  a bogus route under it returns the gateway's own bare 404.
- **`JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID` unset.** This is the declaration that the
  configured credentials are organization-scoped, mirroring the Security Cloud pattern —
  except that there is nothing to compare it against, because an organization-scoped request
  carries no scope header and so no `JAMFPLATFORM_*` variable holds the organization. That is
  exactly why it is required: without it, a tenant credential set with both scope variables
  blank looks identical to a real organization integration and fails deep in an apply.

The organization credential set and its declaration **are** now wired
(`JAMFPLATFORM_ACC_ORGANIZATION_BASE_URL`, `_CLIENT_ID`, `_CLIENT_SECRET`,
`JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID`), and `organization` is the account lane's
`require` token, so the family can no longer self-skip green — verified live on 2026-09-03,
where `TestAccDataSource_AccountSSODomains_ListsAClaim` passed rather than skipped. It was
previously wired in **zero** places, exactly as the SDK's own organization set had been, and
the entry points those suites are the only live cover for — the two domain data source
`Read`s, the list resource's `List`, and the verify action's `Invoke` — carry stub-server
unit tests as well, so the family was never resting on the skipping job alone.

The two SSO domain fixtures remain unset, so the tests covering the two verification
outcomes the suite cannot manufacture still skip. That is a fixture gap, not a credential
one, and it shows in the lane summary as a skip.
