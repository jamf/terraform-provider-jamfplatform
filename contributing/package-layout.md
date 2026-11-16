# Package layout and Go conventions

Go style, dependencies, the file split inside a resource or function package, naming patterns, shared abstractions and the plan-time name collision check. Part of the [style guide](../STYLE_GUIDE.md).

## Go Conventions

- Follow standard Go conventions and idiomatic patterns.
- Run `go fmt ./...` and `golangci-lint run` before committing.
- Use clear, descriptive names for variables, functions, and types.
- Every exported constant, function, variable set, and type must have a short comment describing its purpose.
- Do not add comments inside type definitions or function bodies.

## Dependencies

Allowed:

- Go standard library.
- `golang.org/x` packages.
- HashiCorp Terraform Plugin packages: `terraform-plugin-framework`, `terraform-plugin-framework-timeouts`, `terraform-plugin-framework-validators`, `terraform-plugin-go`, `terraform-plugin-log`, `terraform-plugin-testing`.
- `github.com/jamf/jamfplatform-go-sdk` — required for all Jamf Platform API access (auth, HTTP transport, request/response types).
- `howett.net/plist` (BSD-2-Clause) — Apple plist parser/serialiser. Required by `internal/resources/pro/macos_configuration_profile/` to compare user-supplied `.mobileconfig` payloads against the server-canonical form for diff suppression. Use is contained to the configuration-profile resource family; no other code path should import it.

Do not introduce other third-party dependencies without prior discussion.

## Resource Package File Conventions

Resource packages live under `internal/resources/<domain>/<resource>/` and use resource-agnostic filenames:

| File | Purpose |
|------|---------|
| `resource.go` | Schema definition and boilerplate |
| `crud.go` | Create, Read, Update, Delete, and ImportState |
| `model_types.go` | Terraform model structs |
| `schema_types.go` | Attribute type maps for `ObjectValue`/`ListValue` state |
| `mappings.go` | Lookup tables and name mappings |
| `input_builders.go` | Build API request payloads from Terraform model data |
| `state_builders.go` | Map API responses to Terraform state |
| `helpers.go` | Resource-specific helper functions |
| `state_upgrader.go` | Schema version upgraders (when bumping schema version) |
| `plan_modifiers.go` | Schema plan modifiers (if needed) |
| `validators.go` | Schema validators (if needed) |
| `collision.go` | Plan-time name collision check: the uniqueness `collision.Namespace` and the `ModifyPlan` hook (see §Plan-time name collision check) |
| `list_resource.go` | List resource implementation |
| `data_source.go` | Singular (by-id / by-name) data source implementation |
| `datasource_plural.go` | Plural (list) data source implementation, when the resource also exposes one |

### Optional split-outs for complex resources

- `endpoints_builders.go` / `endpoints_state.go` — when endpoint logic dominates.
- `nested_builders.go` / `nested_state.go` — for large nested payloads.

### Data-source-only packages

Packages that only contain a data source use `model_types.go` for their model structs and `data_source.go` for the implementation.

### Plural (list) data sources

A plural data source (`jamfplatform_pro_categories`) lives **in the same package as its singular counterpart** (`category/`), not in a separate `categories/` package. Add `datasource_plural.go` for the implementation; its model structs join the package's existing `model_types.go` alongside every other construct's models. Reuse the singular package's exported schema helpers, state builders, and SDK wiring wherever the shapes overlap (see `app_installer/` and `app_installer_title/` for full reuse).

- The plural read timeout const is `defaultPluralReadTimeout` to avoid colliding with the singular's `defaultReadTimeout`; `minJamfProVersion` is shared (declared once in the package).
- Where the list endpoint returns a strict subset of the singular's fields, the plural carries its own result model (e.g. `UserGroupsDataSourceResultModel`) so the sparse shape is explicit in one package rather than implied across two directories.

## Provider Function File Conventions

Provider-defined functions live under `internal/functions/<function>/`. They are **offline**: called before provider configuration is evaluated, so they take no SDK client and no credentials (the framework does not expose provider config to a function). Each package uses function-agnostic filenames:

| File | Purpose |
|------|---------|
| `function.go` | The `function.Function` implementation: `Metadata`, `Definition` (parameters + return), and `Run`. Also the argument decode + `Profile`/input mapping. |
| `<core>.go` | The offline core that does the actual work (e.g. `assemble.go`, `render.go`) — pure Go, no framework types, independently unit-testable. |
| `function_test.go` | Unit tests for `Definition`/`Metadata` and the input mapping. |
| `<core>_test.go` | Unit tests for the core, including golden output where determinism matters. |
| `function_acceptance_test.go` | `//go:build acceptance` — drives the function through real Terraform (see below). |

Conventions:

- **Decode `types.Dynamic` via `helpers.TerraformDynamicToJSON`.** Declare each argument as a `function.DynamicParameter` and decode with `req.Arguments.Get` → `helpers.TerraformDynamicToJSON` → a `map[string]any`. `DynamicParameter` (not a typed `ObjectParameter`/`ListParameter`) is deliberate: it lets a heterogeneous input (e.g. a `payloads` list whose objects have different key sets) decode as a cty tuple instead of being rejected for non-uniform element types. Guard the top-level type-assert and return `function.NewArgumentFuncError(argIndex, …)` on mismatch.
- **Keep the core framework-free.** The `Run` method should decode, delegate to the offline core, and set the result — nothing else. The core returns `([]byte, error)` / a plain value so it can be unit-tested without the framework.
- **Share a core between related functions** rather than duplicating the logic. Today `mcx_forced_payload` imports the `mobileconfig` package for `Assemble`/`Profile`, because `mcx` is conceptually a specialisation of `mobileconfig`. If a third function needs the same core, lift the neutral assembler into its own package (e.g. `internal/functions/profileassembler`) that each function package imports, rather than importing one function's package from another.
- **Register** in `internal/provider/provider.go` via the `Functions()` method (add it if this is the first function).

### Testing a provider function

Cover both sides of the framework seam:

- **Core unit tests** feed Go values directly to the core (`Assemble`, `render…`) — fast, exhaustive, and where golden-output pinning lives.
- **`Run` seam tests** build a real `types.Dynamic` and call `Run` through `function.NewArgumentsData([]attr.Value{…})`, asserting both the happy path (result set, no error) and at least one argument-error path (e.g. a non-object argument yields a `FuncError`). This is the path Terraform actually invokes; core tests bypass it.
- **Acceptance test** (`//go:build acceptance`) invokes the function from a real Terraform config through an `output`, and asserts the rendered string with `statecheck.ExpectKnownOutputValue` + `knownvalue.StringRegexp`. Because functions are offline, the function acceptance test **must not** call `testhelpers.AccPreCheck` (that gates on tenant credentials and would skip) — call `testhelpers.AccPreCheckOffline(t)` instead, which sets `TF_ACC` without the credential gate so the test runs (rather than silently skipping) under a raw `go test -tags=acceptance`. Use `ProtoV6ProviderFactories` directly and gate only on Terraform version with `tfversion.SkipBelow(tfversion.Version1_8_0)` (provider functions are GA from Terraform 1.8). Reference: `internal/functions/mobileconfig/`.

## Test File Conventions

| File | Purpose |
|------|---------|
| `schema_test.go` | Schema validation (every package) |
| `helpers_test.go` | Helper function tests |
| `input_builders_test.go` | Input builder tests |
| `state_builders_test.go` | State builder tests |
| `state_upgrader_test.go` | State upgrader tests (where present) |
| `resource_acceptance_test.go` | Resource acceptance tests (`//go:build acceptance`) |
| `datasource_acceptance_test.go` | Data-source-only package acceptance tests (`//go:build acceptance`) |
| `schema_plural_test.go` | Plural data source schema validation |
| `datasource_plural_acceptance_test.go` | Plural data source acceptance tests (`//go:build acceptance`) |

## Client Conventions

The Jamf Platform API client lives in the external SDK `github.com/jamf/jamfplatform-go-sdk` (package `jamfplatform`). This repository imports it; it is not vendored. To consume new endpoints, bump the SDK dep in `go.mod` (or coordinate changes upstream first).

The SDK follows the conventions below — match them when contributing upstream.

### Versioned naming

Client types and functions use explicit version suffixes to support multiple API versions:

```go
// V1 functions
func (c *Client) CreateDeviceGroupV1(ctx context.Context, req *DeviceGroupCreateRepresentationV1) (*DeviceGroupCreateResponseV1, error)

// V2 functions added alongside V1
func (c *Client) CreateCBEngineBenchmarkV2(ctx context.Context, req *CBEngineBenchmarkRequestV2) (*CBEngineBenchmarkResponseV2, error)
```

When endpoints are upgraded, add new versioned types/functions alongside existing ones. Resources migrate at their own pace.

### Type naming

Request and response types include the version suffix and follow the pattern `<Domain><Entity><Purpose><Version>`:

```go
type DeviceGroupCreateRepresentationV1 struct { ... }
type CBEngineBenchmarkResponseV2 struct { ... }
```

## Naming Patterns

### Resources

Terraform construct names follow `jamfplatform_<domain>_<entity>` (or `jamfplatform_<domain>_<entity>_<verb>` for actions):

- `jamfplatform_device_group` (resource, data source, list resource)
- `jamfplatform_blueprints_blueprint` (resource, data source, list resource)
- `jamfplatform_cbengine_benchmark` (resource, data source, list resource)
- `jamfplatform_device_erase` / `_restart` / `_shutdown` / `_unmanage` (actions)
- `jamfplatform_security_cloud_dns_zone` / `_ztna_gateway` / `_ztna_grouped_gateway` (resource, data source, list resource)

### Test names

Test functions use the pattern `TestAccResource_<Resource>_<Scenario>` for acceptance tests and `Test<Function>_<Case>` for unit tests:

```go
func TestAccResource_DeviceGroup_SmartComputer(t *testing.T) { ... }
func TestAccDataSource_Baselines(t *testing.T) { ... }
func TestBuildBlueprintInput_MinimalConfig(t *testing.T) { ... }
```

### Acceptance test resource names

Use the `tf-acc-` prefix for all resources created during acceptance tests:

```
tf-acc-static-computer
tf-acc-benchmark-all-rules
tf-acc-bp-scope-passcode
```

## Shared abstractions — when to extract

Two extraction triggers apply, one for schemas and one for code helpers. They differ because the failure modes differ.

**Schemas — 3-consumer rule.** Many Jamf Pro resources expose similar shapes (scope, site, category, criteria, self-service payload). **Do not extract these into shared schemas upfront** — superficially similar Jamf APIs often differ in field names, ID types (int vs UUID string), and null semantics. Premature abstraction produces helpers with per-resource branching that is harder to read than the original duplication. Refactor trigger: **3 or more** shipped resources with verified-identical shape (same fields, types, null semantics — checked against the SDK structs, not eyeballed). Extract under `internal/common/schemas/`. Reference precedent: `internal/common/scope/` (scope sub-blocks extracted at sub-block granularity; the top-level scope shape stayed per-resource because it diverged across the eight scope-bearing resources).

**Code helpers — 2-consumer rule when logic is non-trivial.** Parsers, normalisation/diff-suppression functions, identifier injectors, and similar non-trivial code with no per-resource branching extract at the **second** consumer, not the third. Two copies of a 200-LOC plist mask or a `WriteOnly` rotation comparator means two places to chase the next server-side mutation; a duplicated bug now lives twice. The schema 3-consumer rule was written for cases where premature abstraction produces ugly branching — that risk does not apply to code helpers that compose without branching.

Canonical example: when `jamfplatform_pro_mobile_device_configuration_profile` landed as the second consumer of the mobileconfig mask / compare / identifier-injection helpers, they were lifted out of `macos_configuration_profile` into the shared `internal/common/payloadhelpers/` package (`MaskPayload`, `PayloadsSemanticallyEqual`, `ThreeWayCompare`, `InjectTopLevelIdentifiers`) at the second consumer — not duplicated, not deferred to a third. The `pro/` tree is flat, so any code shared across two or more Pro packages lives under `internal/common/<topic>/` (`payloadhelpers`, `availabletitles`, `invitationcommon`, …) — never in one resource package imported by a sibling.

Trigger summary:

| Kind | Trigger | Destination |
|---|---|---|
| Schema (shape, field set) | 3+ verified-identical SDK shapes | `internal/common/schemas/` (or domain-specific package like `internal/common/scope/`) |
| Code helper (non-trivial, no per-resource branching) | 2 consumers | `internal/common/<topic>/` |
| Trivial code helper (1-line wrapper) | Stays in-resource; extract only on demonstrated need | `internal/common/helpers/` |

## Plan-time name collision check

A resource whose create or rename Jamf refuses (or silently accepts) for a duplicate key wires the shared check from `internal/common/collision` into `ModifyPlan`. Issue #533 holds the wire-probed uniqueness table; probe any type that is not in it before wiring.

- **One `collision.go` per package**, holding the `collision.Namespace` builder and the `ModifyPlan` method that calls `collision.ReportPlan`. Fetch the checker in `Configure` with `providerdata.ConfigureCollision(req.ProviderData)`; a nil checker is the disabled state, so there is no flag check.
- **The namespace states the rule Jamf applies, not the rule that is convenient.** `Enforced` is whether Jamf refuses the duplicate. `Collation` is one of the named collations (`CollationFolded`, `CollationAccentSensitive`, `CollationCaseSensitive`, `CollationExact`) taken from the probe table; the three axes differ by type, so never reuse one by guess. A key that is not a name (`domains`, `hostnames`, `serialNumber`, `fileName`) gets its own namespace and `KeyLabel`.
- **Types sharing one pool of names share one namespace.** Jamf Pro groups are the case: smart groups, static groups and Platform device groups of one device type draw on one pool, built once in `internal/common/groupcollision` so the five constructs do not import each other. A `Source` names the Terraform resource that adopts it in `ResourceType`; the import snippet is offered only when that matches the resource being planned. An object two resource types manage under different identifiers (a Jamf Pro group is also a Platform device group) lists the other identifiers in `Entry.AltIDs`, which both the snippet and self-exclusion read.
- **The collation is the write API's, not the object type's.** The same group pool compares differently depending on which API writes: the Pro v3 and Platform computer-group writes are case-insensitive only, the Pro v2 and Platform mobile-group writes also fold trailing spaces and accents, and the Classic API trims the XML text of every field first and compares accents exactly. Model the API the resource writes through, and re-probe when it changes.
- **A key in one top-level string attribute uses `collision.ReportAttributePlan`**, which reads the attribute from the plan and state directly. `collision.ReportPlan` decodes the whole model, which fails on an unknown nested value such as a smart group's criteria, and a failed decode skips the check.
- **List with the SDK list method the resource's `Read` path already needs**, one call per namespace. Where a type's list endpoint has dropped rows (Classic mobile device provisioning profiles on 11.32.0, fixed in 11.33.0), give the `Source` a `Find` function instead of `List`: it queries each planned key directly (v1 and Platform RSQL `name==`, or Classic `/name/{name}`, which refuses a name containing `/`) at the cost of one read per key. Add the method to the package's `resourceSDKMethods` so the permissions table lists it, and have the `permissions_test.go` assertion read `collision.go` as well as `crud.go`.
- **Record the rule in `crud.go`'s endpoints annotation** on a `Uniqueness:` line, for example `Uniqueness: name, enforced (409), case- and trailing-space-insensitive, tenant-wide across sites`.
- **Acceptance test per family**: `collision_acceptance_test.go` creates the colliding object outside Terraform in `PreConfig` and asserts the plan-time error with `ExpectError`. Terraform wraps the message, so the regex uses `\s+` between words.
- **A listing that fails is a warning, never an error.** The check must not fail a plan that would otherwise succeed.
