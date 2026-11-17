# Style Guide

Conventions for this provider, split by concern under [`contributing/`](contributing/). This file is the index: code comments cite sections as `STYLE_GUIDE.md §<section name>`, and the tables below say which file holds each one.

| File | Covers |
|---|---|
| [Package layout and Go conventions](contributing/package-layout.md) | Go style, dependencies, the file split inside a resource or function package, naming patterns, shared abstractions and the plan-time name collision check. |
| [Schema design](contributing/schema-design.md) | Attribute naming, enums, Sets vs Lists, secrets, `Optional+Computed`, nested blocks and cross-field validation. |
| [Descriptions and example prose](contributing/descriptions.md) | What a schema description and an example say, and how they say it. |
| [Read, write and lifecycle semantics](contributing/write-semantics.md) | Import hydration, server quirks, error handling, IDs and import, singletons, full-replace and merge-patch updates, classic collections, create and delete semantics, retry helpers. |
| [Configuration profile payloads](contributing/configuration-profiles.md) | Payload diff suppression (mask-and-compare), storage categories and the import fidelity gate. |
| [Scope](contributing/scope.md) | The API integration scope gate and the shared Jamf Pro scope helper. |
| [Product family rules](contributing/families.md) | Naming, Configure helpers and recurring shapes for Jamf Account, Jamf Security Cloud and Jamf Pro, including the Jamf Pro version gate. |
| [Endpoint adoption, migration and deprecation](contributing/endpoint-lifecycle.md) | Adopting new endpoints, migrating off deprecated ones, the 90-day schema deprecation window, tracking and audit. |

Testing conventions are in [TESTING.md](TESTING.md); the contribution workflow is in [CONTRIBUTING.md](CONTRIBUTING.md).

## Section index

### Package layout and Go conventions

- [Go Conventions](contributing/package-layout.md#go-conventions)
- [Dependencies](contributing/package-layout.md#dependencies)
- [Resource Package File Conventions](contributing/package-layout.md#resource-package-file-conventions)
  - [Data-source-only packages](contributing/package-layout.md#data-source-only-packages)
  - [Plural (list) data sources](contributing/package-layout.md#plural-list-data-sources)
- [Provider Function File Conventions](contributing/package-layout.md#provider-function-file-conventions)
  - [Testing a provider function](contributing/package-layout.md#testing-a-provider-function)
- [Test File Conventions](contributing/package-layout.md#test-file-conventions)
- [Naming Patterns](contributing/package-layout.md#naming-patterns)
- [Shared abstractions — when to extract](contributing/package-layout.md#shared-abstractions--when-to-extract)
- [Plan-time name collision check](contributing/package-layout.md#plan-time-name-collision-check)

### Schema design

- [Schema Guidelines](contributing/schema-design.md#schema-guidelines)
  - [Attribute names mirror the Jamf Pro admin UI when the wire name is cryptic](contributing/schema-design.md#attribute-names-mirror-the-jamf-pro-admin-ui-when-the-wire-name-is-cryptic)
  - [Enum values and error codes come from the SDK, not from literals](contributing/schema-design.md#enum-values-and-error-codes-come-from-the-sdk-not-from-literals)
  - [Sets vs Lists](contributing/schema-design.md#sets-vs-lists)
  - [Plaintext secrets — `WriteOnly` with `_wo_version` rotation companion](contributing/schema-design.md#plaintext-secrets--writeonly-with-_wo_version-rotation-companion)
  - [Server-minted secrets — `Computed + Sensitive` with a rotation trigger](contributing/schema-design.md#server-minted-secrets--computed--sensitive-with-a-rotation-trigger)
  - [Server-derived computed fields & `Optional+Computed` attributes](contributing/schema-design.md#server-derived-computed-fields--optionalcomputed-attributes)
  - [`SingleNestedAttribute` blocks: Optional-only when the model uses typed-pointer](contributing/schema-design.md#singlenestedattribute-blocks-optional-only-when-the-model-uses-typed-pointer)
  - [Asymmetric server normalisation on `type`-style discriminator fields](contributing/schema-design.md#asymmetric-server-normalisation-on-type-style-discriminator-fields)
  - [Cross-field validation](contributing/schema-design.md#cross-field-validation)
  - [Attribute defaults do not work inside `SetNestedAttribute`](contributing/schema-design.md#attribute-defaults-do-not-work-inside-setnestedattribute)
  - [Referencing a server-managed catalog by name](contributing/schema-design.md#referencing-a-server-managed-catalog-by-name)

### Descriptions and example prose

- [User-facing descriptions are UI-aligned, not wire-aligned](contributing/descriptions.md#user-facing-descriptions-are-ui-aligned-not-wire-aligned)
  - [Translating UI labels/presets to wire values](contributing/descriptions.md#translating-ui-labelspresets-to-wire-values)
- [Description and example prose: the terse-reference voice](contributing/descriptions.md#description-and-example-prose-the-terse-reference-voice)

### Read, write and lifecycle semantics

- [First-time hydration detection in Read](contributing/write-semantics.md#first-time-hydration-detection-in-read)
  - [Blocks holding a `WriteOnly` secret](contributing/write-semantics.md#blocks-holding-a-writeonly-secret)
  - [Never hydrate state that drives a destructive write](contributing/write-semantics.md#never-hydrate-state-that-drives-a-destructive-write)
- [ID type handling](contributing/write-semantics.md#id-type-handling)
- [Import format](contributing/write-semantics.md#import-format)
- [Endpoint shape classification](contributing/write-semantics.md#endpoint-shape-classification)
- [Singleton resources](contributing/write-semantics.md#singleton-resources)
- [Tenant singletons with a real clear](contributing/write-semantics.md#tenant-singletons-with-a-real-clear)
- [Create semantics: a failed create is not proof nothing was created](contributing/write-semantics.md#create-semantics-a-failed-create-is-not-proof-nothing-was-created)
- [Update semantics at a glance](contributing/write-semantics.md#update-semantics-at-a-glance)
- [Full-replace endpoints & shared backing stores](contributing/write-semantics.md#full-replace-endpoints--shared-backing-stores)
  - [1. Every write sends a complete body](contributing/write-semantics.md#1-every-write-sends-a-complete-body)
  - [2. `Optional+Computed` with a state-preserving plan modifier for every user-settable optional scalar](contributing/write-semantics.md#2-optionalcomputed-with-a-state-preserving-plan-modifier-for-every-user-settable-optional-scalar)
  - [3. Shared backing store: read-merge-write and a process-shared mutex](contributing/write-semantics.md#3-shared-backing-store-read-merge-write-and-a-process-shared-mutex)
- [Merge-patch (`PATCH` → `204`) settings updates](contributing/write-semantics.md#merge-patch-patch--204-settings-updates)
  - [Merge-patch merges *inside* a nested object, so a discriminated block must send explicit nulls](contributing/write-semantics.md#merge-patch-merges-inside-a-nested-object-so-a-discriminated-block-must-send-explicit-nulls)
- [Classic XML `PUT` merge where *empty clears* (omit = retain)](contributing/write-semantics.md#classic-xml-put-merge-where-empty-clears-omit--retain)
  - [Bearer-auth-only gaps](contributing/write-semantics.md#bearer-auth-only-gaps)
- [Classic sub-collections — omit / empty / present clear semantics](contributing/write-semantics.md#classic-sub-collections--omit--empty--present-clear-semantics)
  - [The shared `<criterion>` element](contributing/write-semantics.md#the-shared-criterion-element)
- [Server-managed child collections via create/delete-only endpoints](contributing/write-semantics.md#server-managed-child-collections-via-createdelete-only-endpoints)
- [Server-*expanding* collections — intersect-on-read, not omit/empty/present](contributing/write-semantics.md#server-expanding-collections--intersect-on-read-not-omitemptypresent)
- [Classic membership: author by username, mirror the resolved ID set as Computed](contributing/write-semantics.md#classic-membership-author-by-username-mirror-the-resolved-id-set-as-computed)
- [Delete semantics: not-found, async, and propagation-blocked](contributing/write-semantics.md#delete-semantics-not-found-async-and-propagation-blocked)
- [Referenced-by-name dependencies & `HAS_DEPENDENCIES` on delete](contributing/write-semantics.md#referenced-by-name-dependencies--has_dependencies-on-delete)
- [Working around server-broken classic endpoints](contributing/write-semantics.md#working-around-server-broken-classic-endpoints)
- [SDK type-decode bugs — fix by generator override, not provider hand-decode](contributing/write-semantics.md#sdk-type-decode-bugs--fix-by-generator-override-not-provider-hand-decode)
- [Field-order-sensitive classic write payloads (rare)](contributing/write-semantics.md#field-order-sensitive-classic-write-payloads-rare)
- [Form-decoded classic input fields (encode on write)](contributing/write-semantics.md#form-decoded-classic-input-fields-encode-on-write)
- [Error Handling](contributing/write-semantics.md#error-handling)
  - [Whitespace Jamf Pro strips on save](contributing/write-semantics.md#whitespace-jamf-pro-strips-on-save)
- [Pro error/retry helpers](contributing/write-semantics.md#pro-errorretry-helpers)

### Configuration profile payloads

- [Configuration profile payload diff suppression (mask-and-compare)](contributing/configuration-profiles.md#configuration-profile-payload-diff-suppression-mask-and-compare)
- [Payload storage categories and the import fidelity gate](contributing/configuration-profiles.md#payload-storage-categories-and-the-import-fidelity-gate)

### Scope

- [API integration scope: every construct declares which scopes reach it](contributing/scope.md#api-integration-scope-every-construct-declares-which-scopes-reach-it)
  - [Widening and narrowing a family](contributing/scope.md#widening-and-narrowing-a-family)
  - [Where the code lives](contributing/scope.md#where-the-code-lives)
- [Scope helper](contributing/scope.md#scope-helper)
  - [Block layout](contributing/scope.md#block-layout)
  - [Item shape and naming](contributing/scope.md#item-shape-and-naming)
  - [Composition](contributing/scope.md#composition)
  - [All-flag validators](contributing/scope.md#all-flag-validators)
  - [Omission semantics](contributing/scope.md#omission-semantics)
  - [Directory-service user-group preflight](contributing/scope.md#directory-service-user-group-preflight)

### Product family rules

- [Jamf Account Resource Naming](contributing/families.md#jamf-account-resource-naming)
  - [Rules](contributing/families.md#rules)
  - [Jamf Account shapes that recur](contributing/families.md#jamf-account-shapes-that-recur)
- [Jamf Security Cloud Resource Naming](contributing/families.md#jamf-security-cloud-resource-naming)
  - [Rules](contributing/families.md#rules-1)
  - [Security Cloud shapes that recur](contributing/families.md#security-cloud-shapes-that-recur)
  - [Security Cloud Configure: use the `providerdata.ConfigureSecurityCloud` helper](contributing/families.md#security-cloud-configure-use-the-providerdataconfiguresecuritycloud-helper)
- [Jamf Pro Resource Naming](contributing/families.md#jamf-pro-resource-naming)
  - [Rules](contributing/families.md#rules-2)
  - [Minimum Jamf Pro version check](contributing/families.md#minimum-jamf-pro-version-check)
  - [Provider overall minimum Jamf Pro version (advisory warning)](contributing/families.md#provider-overall-minimum-jamf-pro-version-advisory-warning)

### Endpoint adoption, migration and deprecation

- [Endpoint adoption & migration policy](contributing/endpoint-lifecycle.md#endpoint-adoption--migration-policy)
  - [Jamf Pro Terraform schema deprecation — 90-day window](contributing/endpoint-lifecycle.md#jamf-pro-terraform-schema-deprecation--90-day-window)
  - [Platform Services *Terraform schema* deprecation — 90-day window](contributing/endpoint-lifecycle.md#platform-services-terraform-schema-deprecation--90-day-window)
  - [SDK side-by-side dependency](contributing/endpoint-lifecycle.md#sdk-side-by-side-dependency)
  - [Deprecated with no generated successor](contributing/endpoint-lifecycle.md#deprecated-with-no-generated-successor)
  - [Deprecated with no successor at all](contributing/endpoint-lifecycle.md#deprecated-with-no-successor-at-all)
  - [Adoption (new endpoints)](contributing/endpoint-lifecycle.md#adoption-new-endpoints)
  - [Deprecation migration timeline](contributing/endpoint-lifecycle.md#deprecation-migration-timeline)
  - [Fast-track exception](contributing/endpoint-lifecycle.md#fast-track-exception)
  - [Slow-track exception](contributing/endpoint-lifecycle.md#slow-track-exception)
  - [Tracking — the migration issue](contributing/endpoint-lifecycle.md#tracking--the-migration-issue)
  - [Tracking — in-code annotation (Pro resources only)](contributing/endpoint-lifecycle.md#tracking--in-code-annotation-pro-resources-only)
  - [How a deprecation is detected](contributing/endpoint-lifecycle.md#how-a-deprecation-is-detected)
