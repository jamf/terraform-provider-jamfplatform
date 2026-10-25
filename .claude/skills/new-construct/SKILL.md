---
name: new-construct
description: Build a new Terraform construct in this provider — resource, data source, list resource, action or provider-defined function — end to end, from family classification and wire probes through acceptance tests, docs and the pre-PR gates. Use when adding, planning, or writing a kickoff brief for any jamfplatform_* construct.
---

# New construct build

This skill encodes the *order of work* and the *checked gates*. It does not restate
`STYLE_GUIDE.md`, `CONTRIBUTING.md` or `TESTING.md` — those are authoritative and move often.
Where this file names a section, go read it. A rule copied here would be a second source of
truth, and the previous generation of kickoff prompts died of exactly that.

**`STYLE_GUIDE.md` wins on conflict.** §5 below makes adherence a checked gate at three
points, not a vibe — sessions drift from it and need maintainer course-correction otherwise.

---

## 0. Classify first — family, then shape

Everything downstream (Configure helper, version gate, folder, error translation, test
pre-check) follows from these two answers. Getting the family wrong means mirroring the wrong
reference implementation, which is the most expensive early mistake available.

### Family

| Family | SDK package | Terraform name | Go path | Configure | Version gate |
|---|---|---|---|---|---|
| Platform Services | `blueprints`, `cbengine`, `devices`… | `jamfplatform_<x>` | `internal/resources/<domain>/<x>/` | hand-rolled type-assert + `pd.RequireScope(...)` | none |
| Jamf Pro | `pro/` | `jamfplatform_pro_<x>` | `internal/resources/pro/<x>/` — **flat, no domain tier** | `providerdata.ConfigurePro` | `minJamfProVersion` **required** |
| Jamf Pro classic | `proclassic/` | `jamfplatform_pro_<x>` | same | `providerdata.ConfigureProClassic` | required |
| Security Cloud | `securitycloud` | `jamfplatform_security_cloud_<x>` | `internal/resources/security_cloud/<x>/` — flat | `providerdata.ConfigureSecurityCloud` | **none — must not use ConfigurePro** |
| Actions | `pro`/`proclassic`/`deviceactions` | `jamfplatform_pro_<verb>` / `jamfplatform_device_<verb>` | `internal/actions/pro/<family>/` | `ConfigurePro`/`ConfigureProClassic` | per-action const |
| Functions | none — offline | `jamfplatform::<fn>` | `internal/functions/<fn>/` | none, no client, no provider config | none |

Then read the family file: `references/pro.md`, `references/security_cloud.md`,
`references/platform.md`, `references/actions.md`, `references/functions.md`.

### Shape

Classify against [STYLE_GUIDE §Endpoint shape classification](../../../STYLE_GUIDE.md#endpoint-shape-classification):
resource / data source / list resource / singleton / action / read-only catalogue (plural DS
only, no per-id endpoint). Then pick the closest row from the **Reference implementations**
table in `CLAUDE.md` and diff against it throughout the build. Do not pick the reference from
memory — the table is maintained and your recollection of the tree layout is probably stale.

File split: [STYLE_GUIDE §Resource Package File Conventions](../../../STYLE_GUIDE.md#resource-package-file-conventions).
Two conventions that get invented instead of looked up: name-to-value lookup tables go in
**`mappings.go`** (there is no `enums.go` in the table), and a plural data source lives **in
the singular's package**, not a sibling `<x>s/` directory.

---

## 1. Collect the inputs

A build cannot start from the SDK alone. Gather, and stop and ask for whatever is missing:

- **Construct name + shape + family** per §0. For Pro, also `minJamfProVersion` and its
  source (release notes URL / SDK `// Available since` / hand-research).
- **SDK function set** — Create / Read-by-id / Read-by-name / Update / Delete / List, with the
  list-item shape (full object vs `[]IDName`) and whether Update is full-replace or
  partial-merge. **Do not invent SDK functions or fields.** A missing function or an
  under-specified type (`*[]any`, absent field, divergent write shape) **stops the build** —
  fix the SDK upstream and cut a tag first. See `references/pro.md` §payload audit.
- **UI evidence** — admin-UI screenshots of every tab the construct exposes, saved under
  `spike/screenshots/` (gitignored). Two jobs: attribute/value naming, and surface coverage.
  You cannot screenshot for yourself; ask.
- **Wire evidence** — raw request/response bodies under `local-testing/<endpoint>/`
  (gitignored, PII-sanitised). See `references/wire-probes.md` for how to get them and how to
  write them down honestly.
- **Anything weird** — suspected server invariants, computed echoes that drift, lossy PUTs,
  write-only secrets, multipart upload, paged children. One line each, as *probes to run*.

---

## 2. Workflow

1. **Planner** — flip the tracking row/card to `in-design` (`spike/JAMF_PRO_INVENTORY.md` for
   Pro; GitHub Project #2 otherwise — see the `reference_online_planner` memory).
2. **Branch.** Cut a dedicated branch; never work on `main`.
3. **SDK audit** — every function in §1 exists with concrete field types. ProClassic: run the
   payload audit gate before any Terraform code.
4. **STYLE_GUIDE read** — §5(a) below, *before* the spike, not after.
5. **Wire-probe** (including the uniqueness probe in `references/wire-probes.md`) and write a one-page spike doc at `spike/<RESOURCE>_SPIKE.md` (gitignored).
   Every non-trivial decision **cites the governing STYLE_GUIDE section**; an uncited decision
   is the flag to re-check. Open questions numbered, each with the default you will adopt if
   unanswered. Unrun probes marked NOT-PROBED with the reason — never a plausible-looking
   invented result.
6. **UI naming review** — before the schema exists, not after. `references/naming.md`. This is
   the single most commonly deferred gate and the most expensive to defer.
7. **PAUSE for maintainer approval.** Surface the spike doc and the numbered questions in
   chat. Locked in at this point: SDK function set, construct name, package name, version
   floor, schema sketch.
8. **Scaffold** per the file conventions, mirroring the §0 reference implementation.
9. **Shared abstractions** — extract only at the trigger ([STYLE_GUIDE §Shared abstractions](../../../STYLE_GUIDE.md#shared-abstractions--when-to-extract):
   schemas at 3 consumers, non-trivial code helpers at 2). If a trigger fires mid-build, ship
   the extraction as a **separate prior commit**. Extracting tends to expose a bug the
   duplication was hiding — expect that, and check for it.
10. **Register** in `internal/provider/provider.go`. A construct with a create path also gets its
    plan-time name collision check (`collision.go`, [STYLE_GUIDE §Plan-time name collision check](../../../STYLE_GUIDE.md#plan-time-name-collision-check))
    and a row in `templates/guides/name-collisions.md`; a conformance test fails the build without the row.
11. **Tests** — `references/acceptance.md`. Unit tests always; acceptance coverage per
    [TESTING.md §Writing Acceptance Tests](../../../TESTING.md#writing-acceptance-tests).
12. **Examples** under `examples/{resources,data-sources,list-resources,actions,functions}/<name>/`.
13. **`make fix fmt lint test`** clean, then **`make generate`** — *after* the last HCL edit,
    because the generated doc inlines the example. Commit the regenerated `docs/`.
14. **§5(c) conformance re-check, then the quality gate** (`advisor()` / `/code-review`).
15. **Planner** — flip to `shipped` after merge. **Delete the spike doc**, having first lifted
    every load-bearing finding into STYLE_GUIDE / CONTRIBUTING / the package's own doc
    comments. A spike doc left behind rots into a false authority.

---

## 3. Gates — cheap before, expensive after

Each of these has been paid for late at least once. The right-hand column is what late costs.

| Gate | Do it | Late cost |
|---|---|---|
| SDK payload audit (ProClassic) | before any Terraform code | provider-side decoders working around a fixable SDK bug |
| UI naming + value review | before the schema | schema + tests + examples + docs rewritten; two refactor commits on the Security Cloud branch |
| Wire-probe collection write semantics (omit / `[]` / present) | before modelling any collection | silent data loss, discovered by a user |
| Enum vocabulary from the SDK's `*Values()` helper | at schema time | a restated list that drifts from the API |
| `make generate` | after the **last** HCL edit | ships a stale, sometimes invalid, inlined example |
| Uniqueness probe (create twice; same name, upper case, trailing space, `é`) | with the wire probes | a 409 at apply that `terraform plan` could have named, or two same-named objects nobody was warned about |
| Description jargon grep | before the PR | wire vocabulary published to the Terraform Registry |
| Description prose gate (em dash, bold run-in label, stock clause) | before the PR | generated-sounding docs on the Terraform Registry, and a de-slop sweep across every package later |
| Plumbing (`GNUmakefile`, `.github/workflows/**`, `go.mod`, `scripts/acctargets/**`) in its own PR | from the start | `acctargets` widens CI to `./...` — the full serial suite on the shared tenant |

---

## 4. Guard-rails (DON'T)

- **Don't** invent SDK functions, fields, or enum values. Stop and ask; fix upstream.
- **Don't** write a string literal where the SDK generates a constant — **check each value
  individually, not the set**. Covers schema value enums (build the `OneOf` validator *and* the
  documented list from `*Values()`) *and* the error codes in `mappings.go` (alias
  `securitycloud.ApiErrorItemCodeNotEntitled`, don't retype `"NOT_ENTITLED"`). A generated set
  can carry the generic codes while carrying none of your construct's, so "the SDK has none of
  these" is a claim to verify per code — it has been wrong twice, and a comment asserting it
  hid the defect both times. A deliberate subset keeps its curated list but uses SDK constants
  as the elements. Pin it with an `enum_literals_test.go` calling
  `internal/common/enumguard` — it parses the package's own `const`/`var`/`:=` declarations and
  inlined `OneOf` sets, so a `var validFoos = []string{…}` cannot slip past. Copy
  `internal/resources/pro/ebook/enum_literals_test.go`, or
  `pro/macos_configuration_profile/enum_literals_test.go` if you need exemptions. `Absent` means
  the SDK carries no constant (checked against `Covered`, so a later SDK release that adds it
  fails); `Ignore` means a different vocabulary shares the spelling (not checked). Getting that
  pair the wrong way round reports a promotion that never happened.
  ([STYLE_GUIDE §Enum values and error codes come from the SDK](../../../STYLE_GUIDE.md#enum-values-and-error-codes-come-from-the-sdk-not-from-literals))
- **Don't** ship `*[]any` or other under-specified SDK types.
- **Don't** copy a wire-shape assumption from another construct — even in the same namespace.
  Probe afresh.
- **Don't** write state upgraders. The provider is unshipped; breaking changes are free
  (`feedback_no_state_upgraders`). Renames are free now and breaking after merge.
- **Don't** put wire vocabulary in `MarkdownDescription` — no API/endpoint/SDK/HTTP-verb/status-code
  language ([STYLE_GUIDE §User-facing descriptions are UI-aligned](../../../STYLE_GUIDE.md#user-facing-descriptions-are-ui-aligned-not-wire-aligned)).
  Wire facts belong in Go comments and the `crud.go` annotation block.
- **Don't** glue a description together with em dashes, open a paragraph with a bolded run-in
  label (`**Omit = preserve** — …`), or repeat another package's stock clause word for word.
  One em dash per description at most, and only for a real interruption
  ([STYLE_GUIDE §Description and example prose](../../../STYLE_GUIDE.md#description-and-example-prose-the-terse-reference-voice)).
  The same voice governs the `#` comments in `examples/`.
- **Don't** put comments inside function bodies or type definitions (STYLE_GUIDE:11). A schema
  is built inside a function body — so the wire-name mapping table goes in the **package doc
  comment**, not beside each attribute.
- **Don't** use `Optional+Computed` with `UseStateForUnknown` inside a `ListNestedAttribute` /
  `SetNestedAttribute` — use `UseNonNullStateForUnknown`.
- **Don't** use HCL block syntax for nested attributes. `scope = { ... }`, never `scope { ... }`
  — block syntax fails in examples *and* acceptance configs.
- **Don't** put real tenant / LDAP / directory-group / account / device names in any public
  file. Placeholders in examples and committed tests; acceptance reads real values from env and
  `t.Skip`s when unset. (`spike/` and `local-testing/` are gitignored — captures there are fine.)
- **Don't** call a surface "deprecated" or "legacy" without maintainer confirmation. A wrong
  premise misdirects the whole build.
- **Don't** mask a 4xx/5xx with longer timeouts or added polling. Diagnose the payload — a
  prior Create-500 was XML field order, and "timing/contention" wasted the cycles.
- **Don't** silently skip an acceptance test around a server bug. Raise it, record the skip and
  the escalation link in a memory.
- **Don't** read a green CI run as coverage. Security Cloud's 29 acceptance tests skip in CI by
  design; green and zero-coverage look identical from the outside.

---

## 5. STYLE_GUIDE compliance gate (read → apply → verify)

**(a) Before the spike** — read the sections that apply to this shape and design against them.
High-frequency: §Resource Package File Conventions; §Schema Guidelines → §Attribute names
mirror the admin UI, §User-facing descriptions are UI-aligned; §Sets vs Lists (**Computed
nested collections are `types.List`**); §Server-derived computed fields & `Optional+Computed`;
§`SingleNestedAttribute` blocks: Optional-only with typed-pointer models; §Cross-field
validation; §Plaintext secrets `WriteOnly` / §Server-minted secrets `Computed+Sensitive`;
§ID type handling; §Import format; §Delete semantics; §Plan-time name collision check; §Shared abstractions. ProClassic adds
§Classic XML PUT merge where empty clears, §Classic sub-collections, §Field-order-sensitive
classic write payloads, §Form-decoded classic input fields.

**(b) In the spike doc** — each non-trivial decision names the section it follows, or justifies
the divergence so the maintainer can rule on it at the §2.7 pause.

**(c) Pre-PR re-check** — walk the (a) list against the actual code. Structural checks miss
prose, so this pass is **mechanical too**:

```sh
grep -n "MarkdownDescription" internal/<path>/*.go \
  | grep -iE "api|wire|server|endpoint|PUT|GET|POST|DELETE|HTTP|sdk|/v1/|/JSSResource"
```

Judge the hits: "Jamf Pro rejects…" as product framing is fine, protocol framing is not.

---

## 6. Done definition

- ✅ §2 complete; spike decisions cite their sections; spike doc deleted, findings lifted.
- ✅ Acceptance coverage complete per `references/acceptance.md` — no "deferred to follow-up".
- ✅ `make fix fmt lint test` clean; `make generate` run after the last HCL edit; `docs/` committed.
- ✅ §5(c) re-check passed, including the description grep; quality gate passed.
- ✅ Name collision check wired (or the reason for none recorded), with its guide row and acceptance test.
- ✅ Planner row/card flipped; acceptance suite handed to the maintainer to run
  (`feedback_user_runs_acc_tests` — the maintainer runs acceptance, not the agent).

---

## References

| File | Contents |
|---|---|
| `references/pro.md` | Pro / ProClassic: the CONTRIBUTING workflow, payload audit gate, version floor, annotation block |
| `references/security_cloud.md` | Security Cloud: Configure, entitlement, the recurring shapes, acc scope declaration |
| `references/platform.md` | Platform Services: scope gate, no version gate, GA cutover |
| `references/actions.md` | Actions: no state, framework limits on secrets, partial-failure diagnostics |
| `references/functions.md` | Provider-defined functions: offline, `types.Dynamic`, both sides of the seam |
| `references/wire-probes.md` | How to probe the gateway, and how to record results honestly |
| `references/naming.md` | The UI-naming review: names, enum values, what to keep wire-spelled |
| `references/acceptance.md` | Coverage requirements + the harness traps found live |
