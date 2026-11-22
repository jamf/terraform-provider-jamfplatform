# Package layout and Go conventions

Go style, dependencies, the file split inside a resource or function package, naming patterns, shared abstractions and the plan-time name collision check. Part of the [style guide](../STYLE_GUIDE.md).

## Go Conventions

- Follow standard Go conventions and idiomatic patterns.
- Run `make fix fmt lint test` before committing. `fmt` is `gofmt -s`, `fix` is `go fix`, `lint` is `golangci-lint run`.
- Use clear, descriptive names for variables, functions and types.
- Every exported constant, function, variable set and type has a short comment describing its purpose.
- Do not add comments inside type definitions or function bodies. This applies to test files too. Rationale goes in the doc comment above the declaration.

## Dependencies

Allowed:

- Go standard library.
- `golang.org/x` packages.
- HashiCorp Terraform Plugin packages: `terraform-plugin-framework`, `terraform-plugin-framework-timeouts`, `terraform-plugin-framework-validators`, `terraform-plugin-go`, `terraform-plugin-log`, `terraform-plugin-testing`.
- `github.com/jamf/jamfplatform-go-sdk`, for all Jamf Platform API access (auth, HTTP transport, request and response types). It is an external module, not vendored. To consume a new endpoint, bump it in `go.mod`, or fix the SDK upstream first.
- `howett.net/plist` (BSD-2-Clause), the Apple plist parser and serialiser. Import it only from `internal/common/payloadhelpers` and `internal/common/plisthelpers`; resource packages go through those two.
- `github.com/aws/aws-sdk-go-v2` with its `service/s3` client and `github.com/aws/smithy-go`, the SDK's own transport module (all Apache-2.0), and `github.com/hashicorp/aws-sdk-go-base/v2` (MPL-2.0), which resolves AWS credentials the way Terraform's `s3` backend does. They read `s3://` upload sources. Import them only from `internal/common/filesources`; a resource goes through that package's `Resolver`. `aws-sdk-go-base` has only ever published `beta` tags, so pin it and read its changelog before a bump.
- `github.com/hashicorp/go-azure-sdk/sdk` (MPL-2.0), for its `auth` and `environments` packages alone: they obtain a Microsoft Entra token the way Terraform's `azurerm` backend does. They read `azblob://` upload sources. Import them only from `internal/common/filesources`. The blob requests themselves are plain HTTP, so no Azure storage SDK is a dependency, and none should be added for them. The module publishes a dated pseudo-version almost daily; Dependabot groups its updates into one pull request that it keeps current.

Do not introduce other third-party dependencies without prior discussion.

## Resource Package File Conventions

A resource package is one leaf folder under `internal/resources/`. In the flat families (`pro/`, `security_cloud/`, `account/`, `ai_governance/`, `apple/`) the folder name is the Terraform name minus the family prefix, directly under the family folder, with no grouping tier. Platform Services packages sit under `blueprints/`, `cbengine/` or at the top level (`device/`, `device_group/`, `devices/`, `device_groups/`).

Filenames are resource-agnostic:

| File | Purpose |
|------|---------|
| `resource.go` | Schema definition and boilerplate |
| `crud.go` | Create, Read, Update, Delete and ImportState |
| `model_types.go` | Terraform model structs, for every construct in the package |
| `schema_types.go` | Attribute type maps for `ObjectValue`/`ListValue` state |
| `mappings.go` | Lookup tables and name mappings |
| `input_builders.go` | Build API request payloads from Terraform model data |
| `state_builders.go` | Map API responses to Terraform state |
| `helpers.go` | Resource-specific helper functions |
| `permissions.go` | The SDK methods each construct calls, which render the "Required Jamf permissions" table into its description |
| `state_upgrader.go` | Schema version upgraders (when the schema version is bumped) |
| `plan_modifiers.go` | Schema plan modifiers (if needed) |
| `validators.go` | Schema validators (if needed) |
| `collision.go` | Plan-time name collision check (see [§Plan-time name collision check](#plan-time-name-collision-check)) |
| `impact.go` | Impact alert adapter, for a resource whose scope or membership change reaches devices |
| `deprecations.go` | Deprecation messages and their removal markers (see [endpoint-lifecycle.md](endpoint-lifecycle.md#jamf-pro-terraform-schema-deprecation--90-day-window)) |
| `list_resource.go` | List resource implementation |
| `data_source.go` | Singular (by-id / by-name) data source implementation |
| `datasource_plural.go` | Plural (list) data source implementation, when the package also exposes one |

A large resource may split a concern into further files named for what they hold (`pro/policy/no_execute.go`, `pro/ebook/merge.go`, the blueprint `components/` sub-package).

### Data-source-only packages

A package that only contains a data source uses `model_types.go` for its model structs and `data_source.go` for the implementation.

### Plural (list) data sources

In the Jamf Pro, Security Cloud, Jamf Account and AI Governance families, a plural data source (`jamfplatform_pro_categories`) lives **in the same package as its singular counterpart** (`category/`), not in a separate `categories/` package. Add `datasource_plural.go` for the implementation; its model structs join the package's `model_types.go`. Reuse the singular's exported schema helpers, state builders and SDK wiring wherever the shapes overlap (`app_installer/` and `app_installer_title/` show full reuse).

- The plural read timeout const is `defaultPluralReadTimeout`, to avoid colliding with the singular's `defaultReadTimeout`. `minJamfProVersion` is declared once and shared.
- Where the list endpoint returns a strict subset of the singular's fields, the plural carries its own result model (for example `UserGroupsDataSourceResultModel`), so the sparse shape is explicit.

Two exceptions. A plural with no singular counterpart is a data-source-only package of its own (`security_cloud/ztna_shared_gateways/`, `account/tenants/`). The Platform Services plurals predate the rule and keep their separate packages (`device_groups/`, `devices/`, `blueprints/blueprints/`, `blueprints/components/`, `cbengine/benchmarks/`); do not add new ones in that shape.

## Provider Function File Conventions

Provider-defined functions live under `internal/functions/<function>/`. They are **offline**: Terraform calls them before provider configuration is evaluated, so they take no SDK client and no credentials. Each package uses function-agnostic filenames:

| File | Purpose |
|------|---------|
| `function.go` | The `function.Function` implementation: `Metadata`, `Definition` (parameters and return) and `Run`, plus the argument decode and input mapping. |
| `<core>.go` | The offline core that does the work (`assemble.go`, `render.go`): pure Go, no framework types, unit-testable on its own. |
| `function_test.go` | Unit tests for `Definition`/`Metadata` and the input mapping. |
| `<core>_test.go` | Unit tests for the core, including golden output where determinism matters. |
| `function_acceptance_test.go` | `//go:build acceptance`. Drives the function through real Terraform. |

Conventions:

- **Decode `types.Dynamic` via `helpers.TerraformDynamicToJSON`.** Declare each argument as a `function.DynamicParameter` and decode with `req.Arguments.Get` → `helpers.TerraformDynamicToJSON` → a `map[string]any`. `DynamicParameter`, not a typed `ObjectParameter`/`ListParameter`, is deliberate: a heterogeneous input (a `payloads` list whose objects have different key sets) then decodes as a cty tuple instead of being rejected for non-uniform element types. Guard the top-level type assertion and return `function.NewArgumentFuncError(argIndex, …)` on mismatch.
- **Keep the core framework-free.** `Run` decodes, delegates to the core and sets the result, nothing else. The core returns `([]byte, error)` or a plain value.
- **Share a core between related functions** instead of duplicating it. `mcx_forced_payload` imports the `mobileconfig` package for `Assemble`/`Profile`, because it is a specialisation of `mobileconfig`. If a third function needs the same core, lift the assembler into a neutral package that each function package imports, so no function package imports another's.
- **Register** the function in the `Functions()` method in `internal/provider/provider.go`.

### Testing a provider function

Cover the core, the `Run` seam and an offline acceptance test. The rules are in [TESTING.md](../TESTING.md#provider-functions); `internal/functions/mobileconfig/` is the reference.

## Test File Conventions

Test file names and test function names are in [TESTING.md](../TESTING.md#test-file-layout).

## Naming Patterns

Terraform construct names are `jamfplatform_<family>_<entity>`, snake_case, and the package folder mirrors the name. An action appends its verb or is named for it.

| Family | Pattern | Examples |
|---|---|---|
| Platform Services | `jamfplatform_<domain>_<entity>`, or no domain for devices | `jamfplatform_blueprints_blueprint`, `jamfplatform_cbengine_benchmark`, `jamfplatform_device_group`, `jamfplatform_device_erase` (action) |
| Jamf Pro | `jamfplatform_pro_<x>` | `jamfplatform_pro_category`, `jamfplatform_pro_send_blank_push` (action) |
| Jamf Security Cloud | `jamfplatform_security_cloud_<x>` | `jamfplatform_security_cloud_dns_zone`, `jamfplatform_security_cloud_uem_connect_synchronize` (action) |
| Jamf Account | `jamfplatform_account_<x>` | `jamfplatform_account_sso_domain`, `jamfplatform_account_sso_domain_verify` (action) |
| Jamf AI Governance | `jamfplatform_ai_governance_<x>` | `jamfplatform_ai_governance_policy` |
| Apple | `jamfplatform_apple_<x>` | `jamfplatform_apple_app_store_app` |
| Functions | bare name, called as `provider::jamfplatform::<name>` | `mobileconfig`, `mcx_forced_payload` |

A plural data source takes the plural of its singular's name (`jamfplatform_pro_categories`). The per-family rules are in [families.md](families.md): [Jamf Pro](families.md#jamf-pro-resource-naming), [Jamf Security Cloud](families.md#jamf-security-cloud-resource-naming), [Jamf Account](families.md#jamf-account-resource-naming).

## Shared abstractions — when to extract

Two triggers apply, one for schemas and one for code helpers, because they fail differently.

| Kind | Trigger | Destination |
|---|---|---|
| Schema (shape, field set) | 3+ shipped resources with verified-identical shape | A topic package under `internal/common/`, such as `internal/common/scope/` |
| Code helper (non-trivial, no per-resource branching) | 2 consumers | `internal/common/<topic>/` |
| Trivial code helper (one-line wrapper) | Stays in the resource; extract only on demonstrated need | `internal/common/helpers/` |

**Schemas: 3-consumer rule.** Many Jamf Pro resources expose similar shapes (scope, site, category, criteria, self-service payload). Do not extract them upfront. Superficially similar Jamf APIs differ in field names, ID types (int or UUID string) and null semantics, and a premature abstraction grows per-resource branching that reads worse than the duplication. "Verified-identical" means same fields, types and null semantics, checked against the SDK structs. Extract at the granularity that is actually identical: `internal/common/scope/` shares the scope sub-blocks, while the top-level scope shape stayed per-resource because it diverged.

**Code helpers: 2-consumer rule.** Parsers, normalisation and diff-suppression functions, identifier injectors and similar non-trivial code with no per-resource branching extract at the **second** consumer. Two copies of a plist mask or a `WriteOnly` rotation comparator are two places to chase the next server-side change. `internal/common/payloadhelpers/` is the example: the mobileconfig mask, compare and identifier-injection helpers moved there when the mobile device configuration profile became their second consumer.

**No resource package imports a sibling.** The family trees are flat, so code shared by two or more packages lives under `internal/common/<topic>/` (`payloadhelpers`, `availabletitles`, `invitationcommon`, …).

## Plan-time name collision check

A resource whose create or rename Jamf refuses, or silently accepts, for a duplicate key wires the shared check from `internal/common/collision` into `ModifyPlan`. The uniqueness rule comes from a wire probe, never from a guess: the probed types are tabled in `templates/guides/name-collisions.md`, and a type not listed there is probed before it is wired.

- **One `collision.go` per package**, holding the `collision.Namespace` builder and the `ModifyPlan` method that calls `collision.ReportPlan`. Fetch the checker in `Configure` with `providerdata.ConfigureCollision(req.ProviderData)`. A nil checker is the disabled state, so there is no flag check.
- **The namespace states the rule Jamf applies, not the convenient one.** `Enforced` is whether Jamf refuses the duplicate. `Collation` is one of the named collations (`CollationFolded`, `CollationAccentSensitive`, `CollationCaseSensitive`, `CollationExact`), taken from the probe. A key that is not a name (`domains`, `hostnames`, `serialNumber`, `fileName`) gets its own namespace and `KeyLabel`.
- **Types sharing one pool of names share one namespace.** Jamf Pro smart groups, static groups and Platform device groups of one device type draw on one pool, built once in `internal/common/groupcollision` so the constructs do not import each other. A `Source` names the Terraform resource that adopts it in `ResourceType`; the import snippet is offered only when that matches the resource being planned. An object two resource types manage under different identifiers lists the other identifiers in `Entry.AltIDs`, which the snippet and self-exclusion both read.
- **The collation is the write API's, not the object type's.** The same group pool compares differently by API: the Pro v3 and Platform computer-group writes are case-insensitive only, the Pro v2 and Platform mobile-group writes also fold trailing spaces and accents, and the Classic API trims every field's XML text first and compares accents exactly. Model the API the resource writes through, and re-probe when that changes.
- **A key in one top-level string attribute uses `collision.ReportAttributePlan`**, which reads the attribute from the plan and state directly. `collision.ReportPlan` decodes the whole model, which fails on an unknown nested value such as a smart group's criteria, and a failed decode skips the check.
- **List with the SDK list method the `Read` path already needs**, one call per namespace. Where a type's list endpoint drops rows, give the `Source` a `Find` function instead of `List`: it queries each planned key directly (v1 and Platform RSQL `name==`, or Classic `/name/{name}`, which refuses a name containing `/`), at one read per key. Add the method to the package's `resourceSDKMethods` so the permissions table lists it, and have the `permissions_test.go` assertion read `collision.go` as well as `crud.go`.
- **Record the rule in the `crud.go` endpoints annotation** on a `Uniqueness:` line, for example `Uniqueness: name, enforced (409), case- and trailing-space-insensitive, tenant-wide across sites`.
- **Acceptance test per package**: `collision_acceptance_test.go` creates the colliding object outside Terraform in `PreConfig` and asserts the plan-time error with `ExpectError`. Terraform wraps the message, so the regex uses `\s+` between words.
- **A listing that fails is a warning, never an error.** The check must not fail a plan that would otherwise succeed.
