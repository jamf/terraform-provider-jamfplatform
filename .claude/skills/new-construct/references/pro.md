# Jamf Pro / ProClassic

Authoritative: `CONTRIBUTING.md` §Adding a Jamf Pro Resource (the 11-step workflow),
`STYLE_GUIDE.md` §Jamf Pro Resource Naming, §Minimum Jamf Pro version check,
§Endpoint adoption & migration policy. Read those. This file carries only what a build
needs held in mind at the same time.

## Choosing the layer

Default to `pro/`. Switch to `proclassic/` only when `pro/` is missing the namespace or is
materially less feature-complete. The user never learns which one you picked — the Terraform
name is `jamfplatform_pro_<x>` either way.

## ProClassic SDK payload audit — a gate, not a review

If the layer is `proclassic/`, run `CONTRIBUTING.md` §ProClassic SDK payload audit **before
writing any Terraform code**. Any gap — `*[]any`, a missing field, a divergent write shape, a
mis-tagged bare slice — **must be fixed upstream and a new SDK tag cut** before the provider
code is written. Do not hand-decode around an SDK defect.

Stop-to-modify-the-SDK is the norm here, not an exception; the `feedback_proclassic_sdk_fix_toolbox`
memory carries the defect taxonomy. A mis-tagged bare slice is a data-loss bug, not a cosmetic
one — the `class` build stopped on seven of them.

## Version floor

Every Pro construct declares an unexported `const minJamfProVersion` and funnels Configure
through `providerdata.ConfigurePro` / `ConfigureProClassic` — never a hand-rolled type-assert,
version fetch, gate and floor-warning. Source the floor from Jamf release notes, the SDK
function's `// Available since` comment, or hand-research, and record it in the inventory.

## Endpoint annotation block

`crud.go` opens with the SDK-endpoints annotation block carrying
`Status: current. Last reviewed YYYY-MM-DD.` — Pro and ProClassic only; Platform Services
constructs are exempt. Endpoint paths and SDK function names live **here and in Go comments**,
never in user-facing schema text.

Add a `Uniqueness:` line below the endpoints, for example
`Uniqueness: name, enforced (409), case- and trailing-space-insensitive, tenant-wide across sites`,
or `not enforced; duplicates stored; checked only with include_unenforced`. The probe result stays
with the code that depends on it.

## Classic write semantics to probe, never assume

Each of these bit a shipped build. They are per-endpoint, not per-namespace:

- **Merge PUT where empty clears** — omit to retain, empty to clear. Always-emit scalars.
- **Sub-collection omit / `[]` / present** semantics — and whether clearing is gated on a
  discriminator transition.
- **Field-order sensitivity** in the XML body — a 500 on Create that looks transient.
- **Form-decoded input fields** needing encode-on-write.
- **Subset reads that silently drop structure** — the classic policy subset drops
  `PackageConfiguration` entirely; use the full get.

## Inventory and planner

`spike/JAMF_PRO_INVENTORY.md` is gitignored by design — edit locally, never commit. Flip
`in-design` → `in-progress` → `shipped`. The online planner is GitHub Project #2, not Jira.
