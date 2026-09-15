# Contributing

Thank you for your interest in contributing to the Terraform Provider for Jamf Platform.

## Prerequisites

- **Go** >= 1.26 (see `go.mod` for the exact version)
- **Terraform** >= 1.13.0 (or **OpenTofu** >= 1.6.0)
- **golangci-lint** for linting
- A Jamf Platform tenant with API credentials (for acceptance tests only)

## Getting Started

```bash
# Clone the repository
git clone https://github.com/jamf/terraform-provider-jamfplatform.git
cd terraform-provider-jamfplatform

# Install dependencies
go mod download

# Build + unit tests
make build
make test
```

## Development Workflow

1. Create a feature branch from `main`.
2. Make your changes following the [style guide](STYLE_GUIDE.md).
3. Run go-fix, formatting, linting, and tests before committing:

   ```bash
   make fix
   make fmt
   make lint
   make test
   ```

   Run `make fix` first — it rewrites deprecated Go API usages, so `fmt` and `lint` then operate on the migrated source.

4. Open a pull request against `main`. CI runs build, lint, docs-generation check, and unit tests automatically.
5. CI also runs the Go acceptance test suite against a real tenant via the GitHub `acceptance` environment after a reviewer approves the run. See [TESTING.md](TESTING.md) for details.

## Running a Local Build Against a Real Tenant (Dev Override)

To drive your locally-built provider against a real Jamf tenant — i.e. write ordinary `.tf` files and run `terraform plan`/`apply` with your source changes — use a Terraform [development override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers). A dev override points Terraform at a directory containing your freshly-built binary and **bypasses the registry entirely**, so there is no `terraform init` / version pinning to fight with.

This requires the Go toolchain and Terraform (or OpenTofu) from [Prerequisites](#prerequisites) to be installed and on your `PATH`. Confirm before starting:

```bash
go version          # >= 1.26
terraform version   # >= 1.13.0  (or: tofu version, >= 1.6.0)
```

### 1. Build and install the provider

From the repository root, with the branch you want to test checked out:

```bash
cd terraform-provider-jamfplatform   # the repo root, if you aren't already there
git checkout feat/my-branch          # the branch whose changes you want to exercise
make install
```

`make install` runs `go install ./...`, which compiles **the currently checked-out code** and writes the `terraform-provider-jamfplatform` binary to your Go install directory (`$(go env GOPATH)/bin`, e.g. `~/go/bin`). The binary is not branch-aware — whatever is checked out when you run `make install` is what Terraform will use. Re-run it after every branch switch or source change you want to test.

Find the directory the binary landed in:

```bash
echo "$(go env GOPATH)/bin"
```

### 2. Create a Terraform CLI config with a dev override

Create (or edit) `~/.terraform.d/terraform.tfrc` and point the provider's source address at the directory from step 1. Use an **absolute path** — `~` and `$GOPATH` are **not** expanded here:

```hcl
provider_installation {
  dev_overrides {
    # Replace with the absolute output of: echo "$(go env GOPATH)/bin"
    "jamf/jamfplatform" = "/Users/you/go/bin"
  }

  # For all other providers, install from the registry as normal.
  direct {}
}
```

Tell Terraform to use this file (only needed if it is not at the default `~/.terraform.d/terraform.tfrc` location):

```bash
export TF_CLI_CONFIG_FILE="$HOME/.terraform.d/terraform.tfrc"
```

> The dev-override directory must contain a binary named exactly `terraform-provider-jamfplatform`. Pointing at `$(go env GOPATH)/bin` (where `make install` puts it) satisfies this. Avoid pointing at the repository root — `go build ./...` does not reliably emit the binary there.

### 3. Write config and run Terraform

In a scratch directory (outside the repo, or under the gitignored `local-testing/`), create the usual config. With a dev override active you **omit `version`** and **do not run `terraform init`** for this provider:

```hcl
terraform {
  required_providers {
    jamfplatform = {
      source = "jamf/jamfplatform"
    }
  }
}

provider "jamfplatform" {
  # Or supply via JAMFPLATFORM_* environment variables (see below).
  base_url      = "https://us.api.jamfcloud.com"
  client_id     = "..."
  client_secret = "..."
  environment_id = "..." # preferred; or tenant_id (legacy), never both
}

# ... resources / data sources to exercise ...
```

Export your tenant credentials and run Terraform directly — skip `init`:

```bash
export JAMFPLATFORM_BASE_URL="https://us.api.jamfcloud.com"
export JAMFPLATFORM_CLIENT_ID="..."
export JAMFPLATFORM_CLIENT_SECRET="..."
export JAMFPLATFORM_ENVIRONMENT_ID="..." # preferred; or JAMFPLATFORM_TENANT_ID (legacy), never both

terraform plan
terraform apply
```

Terraform prints a yellow **"Provider development overrides are in effect"** warning on every command — that is expected and confirms the override is live. After changing provider source, re-run `make install` and just re-run `terraform plan` (no `init` needed); Terraform picks up the new binary on the next invocation.

## Adding a New Resource

1. Obtain the OpenAPI specification or request/response examples for the Jamf Platform endpoint.
2. Confirm the required client methods exist in [`jamfplatform-go-sdk`](https://github.com/jamf/jamfplatform-go-sdk). If not, add them upstream (versioned naming, e.g. `CreateMyResourceV1`) and bump the dep in `go.mod`.
3. Create the resource package under `internal/resources/<domain>/<resource>/` following the [file conventions](STYLE_GUIDE.md#resource-package-file-conventions).
4. Register the resource in `internal/provider/provider.go` (`Resources()`, `DataSources()`, `ListResources()`, or `Actions()` as applicable).
5. Add unit tests in the same package: `schema_test.go`, `input_builders_test.go`, `state_builders_test.go`, plus helpers/upgrader tests where relevant.
6. Add `resource_acceptance_test.go` (or `datasource_acceptance_test.go` for data-source-only packages) with `//go:build acceptance` on line 1. Use factories from `internal/testhelpers` (`AccPreCheck`, `AccTestProtoV6ProviderFactories`, `NewAcceptanceClient`, `RequireSmartGroupFixture`). Acceptance coverage must be **comprehensive**, not minimal — Create-only tests are not sufficient. Every resource acceptance file must include: (a) a **multi-step Update round-trip** that mutates every non-RequiresReplace attribute including nested-list add/remove, (b) a negative `ExpectError` test for **every declared cross-field validator** (`ConflictsWith`, `AlsoRequires`, `OneOf`, custom), (c) an **import round-trip** with `ImportStateVerify: true`, (d) a **happy-path test per mutually-exclusive shape** (auth mode, payload kind, scope variant), and (e) **drift-recovery** assertions for any self-healing computed attribute (hash, URL, epoch). Full rules + reference impl: [TESTING.md §Writing Acceptance Tests — Coverage requirements](TESTING.md#writing-acceptance-tests).
7. Add example `.tf` files under the matching `examples/` subdirectory (`examples/resources/<name>/`, `examples/data-sources/<name>/`, `examples/list-resources/<name>/`, `examples/actions/<name>/`). For resources, include an `import.sh` showing the exact `terraform import` command.
8. Run `make fix fmt lint test` and confirm clean (zero lint issues, all unit tests pass). `fix` rewrites deprecated API usages; run it before `fmt`/`lint`.
9. Run `make generate` to regenerate copyright headers, format examples, and rebuild `docs/`. **Mandatory** for every new resource, data source, list resource, or action — `docs/` must reflect the new construct before the PR opens. Commit the generated docs alongside the source.

See [TESTING.md](TESTING.md) for full testing guidance.

## Adding a Jamf Pro Resource

Jamf Pro resources (sourced from the `pro/` or `proclassic/` packages of `jamfplatform-go-sdk`) follow the same file conventions as Platform Services resources **plus** a planning gate. Use this workflow for every Jamf Pro construct. Step order below is the recommended order — wire evidence drives schema decisions, so collect it before sketching the schema, not after.

1. **Pick the API namespace** from `JAMF_PRO_INVENTORY.md` (gitignored, at repo root). Update its status to `in-design`.
2. **Audit `pro/` vs `proclassic/`** for the namespace. Default to `pro/`. Switch to `proclassic/` only when `pro/` is missing or materially less feature-complete.
3. **ProClassic only — SDK payload audit.** If the chosen namespace is `proclassic/`, run the audit in [§ProClassic SDK payload audit](#proclassic-sdk-payload-audit) below before writing any Terraform code. Any SDK gap (`*[]any`, missing field, divergent write shape) **must be fixed upstream and a new SDK tag cut before the Terraform resource is built**. Do not work around SDK defects in the provider with custom decoders.
4. **UI evidence — collect admin-UI screenshots.** Ask the maintainer to screenshot every relevant admin-UI tab/panel for the resource (Options / Scope / Self Service / User Interaction etc., as applicable). Save under `spike/screenshots/` (gitignored). Two roles for the screenshots:
   - **Attribute naming** — the Jamf Pro admin UI is canonical for user-facing strings per [STYLE_GUIDE.md §Attribute names mirror the Jamf Pro admin UI](STYLE_GUIDE.md#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic). Wire names like `cache_last_user` need renaming to UI labels (`create_mobile_account`) at the schema layer. Without the UI evidence, you'd rename to the wrong thing or skip the rename and ship a cryptic schema.
   - **Surface coverage** — confirm the schema models every field the UI exposes for the resource. Phase 2.6 used this pass to add full UI parity to `jamfplatform_pro_policy` and drop wire-dead fields (`trigger_logout`, `maintenance.heal`, `maintenance.prebindings`) the UI didn't expose.
5. **Produce a one-page comparison** in the PR description: SDK package, function set proposed (CRUD + helpers), Terraform construct name (default: derived from SDK filename per [STYLE_GUIDE.md §Jamf Pro Resource Naming](STYLE_GUIDE.md#jamf-pro-resource-naming); override only if needed), **endpoint shape classification** (resource / data source / singleton / action — see [STYLE_GUIDE.md §Endpoint shape classification](STYLE_GUIDE.md#endpoint-shape-classification)), schema sketch driven by the UI evidence, examples of similar shipped resources.
6. **Maintainer approval** locks in: SDK function set, Terraform construct name (and any override), leaf package name under `internal/resources/pro/<resource>/`, and the **minimum Jamf Pro version** (`minJamfProVersion` const — see [STYLE_GUIDE.md §Minimum Jamf Pro version check](STYLE_GUIDE.md#minimum-jamf-pro-version-check)). Source the version from Jamf release notes, the SDK function's `// Available since` comment, or hand-research; record it in `JAMF_PRO_INVENTORY.md`. Mark the row `in-progress`.
7. **Build the package** at `internal/resources/pro/<resource>/` per the [file conventions](STYLE_GUIDE.md#resource-package-file-conventions). Mirror the closest reference implementation from the table in `CLAUDE.md`. Configure **must** funnel through `providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_<name>")` — do not hand-roll the type-assertion / version-fetch / version-gate / floor-warning boilerplate (see [STYLE_GUIDE.md §Pro Configure](STYLE_GUIDE.md#pro-configure-use-the-providerdataconfigurepro-helper)). Credentials are the same `JAMFPLATFORM_*` set used by Platform Services resources — there is no separate Pro credentials gate. **`crud.go` must carry the SDK-endpoints annotation block** at the top of the file with `Status: current. Last reviewed YYYY-MM-DD.` See [STYLE_GUIDE.md §Endpoint adoption & migration policy](STYLE_GUIDE.md#endpoint-adoption--migration-policy). **All `MarkdownDescription` / `Description` strings on the new resource, data source, and list resource must follow [STYLE_GUIDE.md §User-facing descriptions are UI-aligned, not wire-aligned](STYLE_GUIDE.md#user-facing-descriptions-are-ui-aligned-not-wire-aligned)** — no XML element names, no endpoint paths, no SDK function names, no HTTP method names. Endpoint references and SDK annotations belong in the `crud.go` annotation block and Go comments, never in user-facing schema text. How that text *reads* is governed too — [STYLE_GUIDE.md §Description and example prose](STYLE_GUIDE.md#description-and-example-prose-the-terse-reference-voice): one em dash per description at most, no bolded run-in labels, no stock clause copied verbatim from another package, and the same voice in the `examples/` comments.
8. **Reuse shared abstractions only when they exist**. See [STYLE_GUIDE.md §Shared abstractions](STYLE_GUIDE.md#shared-abstractions--when-to-extract): schemas extract at 3 consumers, code helpers at 2 when the logic is non-trivial. When a trigger fires, extract into `internal/common/<topic>/` in a dedicated refactor PR before the new resource lands.
9. **Tests + examples + docs**: schema/input/state/upgrader unit tests, `resource_acceptance_test.go` with `//go:build acceptance` and `internal/testhelpers` factories. Acceptance coverage must satisfy [TESTING.md §Writing Acceptance Tests — Coverage requirements](TESTING.md#writing-acceptance-tests) — at minimum a multi-step Update round-trip, an `ExpectError` test per declared cross-field validator, an `ImportStateVerify` round-trip, one test per mutually-exclusive shape, and drift-recovery assertions for self-healing computed attributes. Examples under `examples/resources/<name>/` (or the matching `data-sources/`, `list-resources/`, `actions/` subdirectory). Then run `make fix fmt lint test` (must be clean) followed by `make generate` (mandatory — rebuilds `docs/` for the new construct). Commit the generated `docs/` files alongside the source.
10. **Quality gate — `advisor()` consult.** Before declaring the PR ready, call the `advisor()` tool. The advisor sees the full diff and is the cheapest second opinion for missed conventions, missing acceptance coverage, MarkdownDescription gaps, or a hidden wire-shape assumption you propagated from the SDK type without verifying. Cheap enough to use every time; expensive to skip on the build that ships the bug.
11. **Update inventory** to `shipped` after merge. Remove any spike doc the build used — lift every load-bearing finding into STYLE_GUIDE / CONTRIBUTING / the resource's own comments first.

## ProClassic SDK payload audit

The `proclassic` package in `jamfplatform-go-sdk` is generated from a spec that does not always describe nested collections precisely. The generator falls back to `*[]any` when it cannot infer a concrete item shape, and sometimes omits fields entirely. **You cannot trust the SDK type to be a complete and accurate model of the wire payload.** Run this audit before any Terraform code for a new ProClassic resource. Skip for `pro/` (JSON, OpenAPI-generated) and Platform Services.

### What to collect

Save all bodies under `local-testing/<endpoint>/` (gitignored). Sanitise PII and tenant identifiers first.

1. `GET /{endpoint}` list — raw XML.
2. `GET /{endpoint}/id/{id}` for at least two structurally different instances (e.g. static vs smart group; populated vs empty nested collections).
3. `GET /{endpoint}/name/{name}` — only if the maintainer believes it diverges from the id path.
4. `POST /{endpoint}/id/0` request + response — surface write-shape divergence (precedent: `ComputerGroupPost` vs `ComputerGroup`).
5. `PUT /{endpoint}/id/{id}` request + response — same.

**Preferred collection: `jamf-cli` against a maintainer profile.** `jamf-cli config list` shows available profiles. `jamf-cli pro classic-<resource> --help` lists subcommands (`apply / create / get / list / update / delete`). Use `-o raw` for raw XML and `-vvv` for full HTTP cycle (URL, headers, request body, response body). Ask the maintainer to seed example objects via the Jamf UI first — programmatic creates miss UI-only field initialisation and default-value nuances. After seeded GETs, run behavioural probes on agent-created throwaway objects to surface what type inspection cannot:

| Probe | Why |
|---|---|
| `list -o raw -vv` | List-item shape (`[]IDName` vs full object) — drives N+1 vs full-populate in the TF list resource. |
| `create` with only required fields | Server-populated defaults (`category` sentinels, default booleans, server-derived paths). |
| `get {id}` after minimal create | Confirms what the server filled in. |
| `create` omitting each spec-optional field in turn | **Server-required fields the spec/SDK types as optional** — a classic create can return `500` on a missing field that looks optional (e.g. `computer_invitation` 500s without `ssh_username`, for every invitation type). Model such a field `Required` so the gap surfaces at plan time instead of as a confusing `500` at apply. |
| `create` with every field populated | Write-shape acceptance + round-trip fidelity. |
| `update {id}` with a subset of fields | **Partial-merge vs full-replace** PUT semantics — critical for TF state handling. |
| `update {id}` with an empty `<field/>` | **Clear vs no-op vs server-sentinel** behaviour. |
| `update {id}` omitting a field entirely | Omitted-tag preservation. |
| `create` with a bogus referenced name | Whether the server validates references (409) or silently accepts. |
| `create` toggling a known interacting field (e.g. `use_generic` for printers, `is_smart` for groups) | Server-side overrides of sibling fields — drift pure GETs won't show. |
| `delete {id} --yes -vvv` then `get {id}` over time | **Delete behaviour**, not just cleanup: response status (some classic deletes return a *misleading* `400`/`409` on an accepted delete), whether removal is synchronous, and — for async deletes — whether *polling itself interferes* (re-`GET` every few seconds vs a single quiet `GET`). Picks the delete handler per [STYLE_GUIDE §Delete semantics](STYLE_GUIDE.md#delete-semantics-not-found-async-and-propagation-blocked). `--yes` required under `--no-input` (or set `JAMF_CLI_ARGS='--yes'`). |

Name saved artifacts after the probe that produced them (`get-id-69-minimal.xml`, `put-merge-clear-category.xml`). Delete agent-created throwaway resources before declaring the audit complete; the maintainer-seeded objects stay.

**Fallback: maintainer paste.** If the agent cannot run `jamf-cli`, request the static GET set in one message and flag every behavioural question (PUT merge semantics, sentinel handling, reference validation) as an explicit open question.

### Audit checklist

Open `proclassic/types.go` and walk every field on each type used by the SDK CRUD methods (`Get*ByID`, `Create*`, `Update*`, list response).

| Symptom in SDK | Meaning | Action |
|---|---|---|
| `*[]any` on a nested collection | Generator could not infer item shape. | File SDK PR with strongly-typed item struct. Mirror `ComputerGroup` (`*[]ComputerGroupCriteriaItem` + reused `Criterion`). |
| Field present in payload, absent in struct | SDK gap. | File SDK PR adding the field. |
| Field present in struct, never in payload | Likely phantom. | Leave it; do **not** expose in TF schema until evidence appears. |
| Write payload differs from read payload | Spec-level divergence. | File SDK PR adding sibling `*Post` type. Mirror `ComputerGroupPost`. |
| Polymorphic root element (e.g. `<smart_user_group>` vs `<static_user_group>`) | Handled by `XMLName xml.Name` + `MarshalXML` override. | Confirm the override exists on the root struct. |

### Outcome gate

- **No gaps** → proceed to TF resource build.
- **Any gap** → file SDK PR first, wait for tag, bump `go.mod`, then build the TF resource. No custom decoders, no ad-hoc `xml.Unmarshal`, no `any`-to-map conversions in the provider — the fix belongs upstream.

Record each audit in the resource's design PR description:

```
SDK audit — proclassic.UserGroup (types.go:11477)
  Collected: GET list, GET id (static), GET id (smart), GET name (same as id)
  Gaps found:
    - Criteria typed *[]any → needs UserGroupCriteriaItem + reuse Criterion
    - Users typed *[]any → needs UserGroupUsersItem + UserGroupUsersItemUser
  SDK PR: jamfplatform-go-sdk#NN, merged 2026-MM-DD, tag v0.X.Y
```

## Adding a New Data Source

Follow the same pattern as resources, but implement `datasource.DataSource` instead of `resource.Resource`. Data source packages that are standalone (not part of a CRUD resource) use `model_types.go` for their model structs.

## Adding a Provider Function

Provider-defined functions are **offline** — they run before provider configuration, take no SDK client and no credentials, and expose pure `terraform` value transformations under the `jamfplatform::` namespace. Use this workflow.

1. Create the package under `internal/functions/<function>/` following the [file conventions](STYLE_GUIDE.md#provider-function-file-conventions): `function.go` (the `function.Function`), a framework-free `<core>.go`, and their tests.
2. Declare arguments as `function.DynamicParameter` and decode with `helpers.TerraformDynamicToJSON` (see the file conventions for why `DynamicParameter` over a typed parameter). Keep `Run` thin: decode → delegate to the core → set result.
3. Register the function in `internal/provider/provider.go` via the `Functions()` method (add the method if this is the first function).
4. Add unit tests for the core (with golden output where determinism matters) **and** `Run` seam tests that build a real `types.Dynamic` via `function.NewArgumentsData` — covering the happy path and at least one argument-error path.
5. Add `function_acceptance_test.go` (`//go:build acceptance`) that invokes the function from a real Terraform config through an `output` and asserts the rendered string with `statecheck.ExpectKnownOutputValue` + `knownvalue.StringRegexp`. Because the function is offline, **do not** call `testhelpers.AccPreCheck` (it gates on tenant credentials) — call `testhelpers.AccPreCheckOffline(t)` instead (it sets `TF_ACC` without the credential gate, so the test actually runs rather than silently skipping under a raw `go test -tags=acceptance`), use `AccTestProtoV6ProviderFactories`, and gate on `tfversion.SkipBelow(tfversion.Version1_8_0)`.
6. Add an example under `examples/functions/<function>/function.tf`.
7. Run `make fix fmt lint test` (must be clean), then `make generate` (mandatory — rebuilds `docs/functions/<function>.md`). Commit the generated docs alongside the source.

Reference implementation: `internal/functions/mobileconfig/` (and `internal/functions/mcx_forced_payload/`, a thin wrapper over the shared `mobileconfig.Assemble` core).

## Project Structure

See `CLAUDE.md` for the full project structure and conventions. Key directories:

| Directory | Purpose |
|-----------|---------|
| `internal/provider/` | Provider configuration and resource registration |
| `internal/resources/` | Resource, data source, and list resource implementations |
| `internal/common/` | Shared helpers and RSQL filter utilities |
| `internal/actions/` | Fire-and-forget device management commands |
| `internal/functions/` | Provider-defined functions (offline; no SDK client or provider config) |
| `internal/testhelpers/` | Acceptance test utilities (provider factories, mock server, fixtures) |
| `examples/` | Example `.tf` configurations (resources, data-sources, list-resources, actions, functions, provider) |
| `docs/` | Auto-generated provider documentation |
| `tools/` | `go:generate` entrypoint for `copywrite`, `terraform fmt`, and `tfplugindocs` |

The Jamf Platform API client lives in the external SDK `github.com/jamf/jamfplatform-go-sdk` — not in this repo.

## Dependencies

See [STYLE_GUIDE.md §Dependencies](STYLE_GUIDE.md#dependencies) for the allowed dependency set. In short: Go stdlib, `golang.org/x`, the HashiCorp Terraform Plugin family, and `jamfplatform-go-sdk`. Do not introduce other third-party dependencies without prior discussion.

## Release Versioning

The provider does **not** follow strict semver. Until further notice:

- **Patch** (`X.Y.Z+1`): bug fixes only, no schema or behavior changes.
- **Minor** (`X.Y+1.0`): everything else — new resources, new attributes, deprecations, **and breaking changes** (renames, removals, attribute-type changes, behavior changes).
- **Major** (`X+1.0.0`): reserved. Bumped only as a deliberate coordinated cleanup release (e.g., wide-scale removals or restructuring). Not required for individual breaking changes.

Document any breaking change explicitly in the release notes (generated via goreleaser — see [TESTING.md](TESTING.md) and the release workflow). When the policy changes (e.g., on adopting strict semver post-Pro-rollout), update this section.

## Commit Messages

Use [conventional commit](https://www.conventionalcommits.org/) style messages:

- `feat: add device_group import support`
- `fix: handle nil ODV in benchmark rules`
- `test: add schema validation for blueprint components`
- `refactor: extract common polling logic to helpers`
- `chore: update CI workflow action versions`
- `docs: add TESTING.md`

Endpoint-deprecation migrations use a fixed form — `refactor(<pkg-or-resource>): migrate <surface> <from>→<to>` — and their tracking issues have their own title, label and deadline conventions. See [STYLE_GUIDE.md §Tracking — the migration issue](STYLE_GUIDE.md#tracking--the-migration-issue).

## Pull Requests

- Keep PRs focused — one feature or fix per PR.
- Include unit tests for new code.
- Include acceptance tests for new resources and data sources.
- Update `examples/` for new Terraform constructs.
- Run `make generate` if schema descriptions changed (to update docs and copyright headers).
- CI must pass before merge.

## Acceptance Tests on Fork PRs (Maintainers)

Fork PRs never receive repository secrets — GitHub withholds them from every
workflow triggered by a `pull_request` from a fork (the run log shows `Secret
source: None`). So the `Acceptance` job on a fork PR runs credential-less and
every suite self-skips: a green check there means "skipped", **not** "passed".

Do **not** wire the tenant credentials into a fork run (e.g. via
`pull_request_target`). The PR's test code executes with those creds and could
exfiltrate them or mutate/erase the shared tenant, and `pull_request_target`
is not covered by the "require approval for external contributors" gate that
protects ordinary fork `pull_request` runs. Instead, a maintainer re-runs the
PR's code from a branch **inside this repo**, where secrets resolve normally:

1. **Review the entire diff first — including `.github/` and `go.mod`.** The
   steps below run the PR's code (and its workflow files) against the live
   tenant, so this review is the security gate. If anything looks untrustworthy,
   stop here.

2. Fetch the PR head into a throwaway, clearly-named branch and push it to this
   repo (replace `283` with the PR number):

   ```bash
   git fetch origin pull/283/head:acc/pr-283
   git push origin acc/pr-283
   ```

3. Open an internal PR from that branch to `main`. It is a *same-repo* PR, so
   the pipeline gets secrets and the `Acceptance` job runs the scoped subset for
   real:

   ```bash
   gh pr create --base main --head acc/pr-283 \
     --title "[acc] validate #283" --draft \
     --body "CI-only validation of fork PR #283. Do not merge."
   ```

4. Watch the `Acceptance` checks on the internal PR, then copy the outcome back
   to the original fork PR.

5. Clean up — **close (do not merge)** the internal PR and delete the branch:

   ```bash
   gh pr close acc/pr-283 --delete-branch
   ```

> A manual `workflow_dispatch` of *Integration Tests* does **not** work for this:
> the acceptance scope is computed by diffing against the PR base branch, which a
> dispatch run doesn't have, so the `Acceptance` job is skipped. The internal PR
> above is the trigger that runs it.

## Tracking Work

**The GitHub project board is the source of truth for project status.** Local docs hold the detail behind each status.

- **GitHub project board** — <https://github.com/orgs/Jamf-Concepts/projects/2/views/1>. Canonical for **status** (`Todo` / `In Progress` / `Done`). Every non-trivial piece of work has a card. Card move rules:
  - `Todo` → `In Progress` when you start work and push the feature branch.
  - `In Progress` → `Done` when the PR is **merged**, not when it opens (so reviewers can find in-flight cards by status).
  - One card per logical PR. Multi-PR initiatives use sub-issues linked to the parent epic.
- **`PRO_ROLLOUT_PLAN.md`** (gitignored) — north-star plan: phase-level status table, in-flight phase notes, things-still-deferred list, locked-in decisions catalogue. Update the phase status table when a phase opens, closes, or shifts.
- **`JAMF_PRO_INVENTORY.md`** (gitignored) — per-row SDK namespace adoption status with full per-resource Notes. Update the row + Notes when a resource ships, when scope reshapes (e.g. an attribute splits out into a follow-up), or when an upstream blocker resolves.
- **Per-build spike docs** (gitignored, transient). When a build is large enough that the design discussion does not fit in a PR description (multi-section resource, schema prune, payload normalisation), draft a `<topic>_SPIKE.md` in the repo root, link it from the board card, and **delete it once the work ships** — every load-bearing finding goes into `STYLE_GUIDE.md` / `CONTRIBUTING.md` so a fresh contributor can build the next resource without reading the spike.
- **Auto-memory** under `~/.claude/projects/.../memory/project_*.md` — Claude Code's per-project memory captures the "why" behind decisions and surfaces it in future sessions. Update an existing `project-*` memory when its status changes; add a new one only when the topic doesn't fit an existing entry. Stale memories are worse than missing memories — see <https://github.com/anthropics/claude-code> docs.

**Drift rule.** If a local doc disagrees with the board on **status**, the board is right and the doc is stale — fix the doc. If the board lacks the **detail** (rationale, blocker, dependency, design notes), `PRO_ROLLOUT_PLAN.md` / `JAMF_PRO_INVENTORY.md` / the linked spike doc is canonical for that detail. The board carries cards, not novels; the local docs carry novels, not status.

**Keeping the board current is non-optional.** Every PR description should reference the board card it closes. The card moves before the PR opens (`Todo` → `In Progress`) and after the PR merges (`In Progress` → `Done`); both moves are the responsibility of whoever is doing the work, not the reviewer.

## Reporting Issues

Open an issue on GitHub with:

- Provider version and Terraform version.
- Relevant Terraform configuration (redact credentials).
- Expected vs actual behaviour.
- Any error messages or logs.
