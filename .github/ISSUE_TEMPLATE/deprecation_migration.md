---
name: Deprecation migration
about: Track migrating off a Jamf-deprecated API surface (maintainers)
title: "deprecation(<pro|proclassic|platform>): <surface> <from>→<to> — migrate by YYYY-MM-DD"
labels: deprecation-migration
assignees: neilmartin83

---

<!--
Conventions are mandatory — see STYLE_GUIDE.md §Deprecation migration timeline →
"Tracking — the migration issue". In short:

- Title carries the binding date; edit it if the date moves.
- Add `blocked-upstream` while the SDK exposes no successor; remove it when it ships.
- Add the issue to GitHub Project #2 and set both `Deprecated On` and `Migrate By`.
- Close only when no call site reaches the deprecated method AND every SA1019
  suppression for the surface is gone.
-->

## Status

<!-- One paragraph: blocked upstream / ready to migrate / in flight in PR #NNN. Name the
     SDK version and issue if the successor arrived through one. -->

## What Jamf deprecated

<!-- SDK version that introduced the Deprecated markers, the spec's deprecation-date,
     how many methods, and which surfaces + versions. -->

## Call sites in this provider

| Call site | Deprecated method | Successor | Migration shape |
|---|---|---|---|
| `path/to/file.go:NN` |  |  | <!-- drop-in / signature change / type change — say which fields differ and whether this call site reads them --> |

<!-- Include acceptance-tagged call sites; mark them as such. -->

## Successor verification

<!-- Wire-probe table (endpoint, status, Deprecation header) and/or the SDK-source
     comparison that establishes the successor is usable: signatures, request/response
     types, query-parameter contract, privilege names. State what was probed on which
     Jamf Pro version and on what date. -->

## Permissions

<!-- Do the successors' Scoped capability permissions in the SDK privilege registry
     match the predecessors'? If not, the rendered "Required Jamf permissions" table
     diffs and `make generate` is required — list the old→new {capability}:{action}
     values. MethodPrivileges.Legacy is no longer rendered, so a change there is not
     user-facing. -->

## Work required

- [ ] <!-- one item per package, each ending in "drop the suppression" -->
- [ ] Refresh the `crud.go` SDK-endpoints annotation (`Status:` line + `Last reviewed`).
- [ ] `make fix fmt lint test`, then `make generate` if any description changed.
- [ ] Re-run acceptance for the affected resources.

## Deadlines

Per [STYLE_GUIDE §Deprecation migration timeline](../blob/main/contributing/endpoint-lifecycle.md#deprecation-migration-timeline), with the deprecation announced YYYY-MM-DD:

- **6-month soft target: YYYY-MM-DD** — migration should be merged.
- **Hard floor: 3 months before Jamf's announced removal date.** <!-- State whether a removal
  date has been published; if it has, give the computed hard floor and use it as the binding date. -->

Being blocked upstream does not pause the clock.
