// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package scope provides shared schema attributes, builders, and validators
// for the <scope> block of Jamf Classic-API resources (policies, ebooks,
// mac applications, mobile device applications, OS X configuration profiles,
// mobile device configuration profiles, patch policies, restricted software).
//
// Each scope block mirrors the Jamf Pro admin UI's three Scope tabs:
// the all-flags and every per-category target set live inside a `targets`
// sub-block, with `limitations` and `exclusions` as its siblings. Every
// attribute is Optional-only and follows per-category granular ownership:
// a declared category (including `[]`) is owned by Terraform, an omitted
// (null) category is left to the admin UI. Because the classic wire replaces
// the whole scope subtree once any category element is present in a PUT
// (wire-probed — see STYLE_GUIDE.md §Scope helper omission semantics), the
// "leave it alone" half is synthesised at apply time: the consuming
// resource's Update GETs the live scope, overlays the declared categories
// via the Merge* helpers in this package, and emits every merged category
// explicitly (empty wrappers included). Read-side, the RefreshManaged*
// helpers keep unmanaged categories out of state.
//
// Sub-block items are modelled as flat Set<String> of IDs (or names for the
// directory-service categories), not nested objects. Server-augmented <name>
// and <udid> wire fields are discarded on read; only IDs round-trip through
// Terraform state. Full rule set in STYLE_GUIDE.md §Scope helper.
//
// Cross-field invariants (all_computers ⇒ no per-computer or per-group
// targets, etc.) are enforced by AllFlagConflictsWith, a value-discriminated
// bool validator, and mirrored at apply time by the merge helpers: a true flag
// empties exactly its paired categories, and a declared paired category turns
// an unmanaged flag off (PickAllFlag). Per-resource exceptions (e.g.
// RestrictedSoftware rejects limitations entirely, and honours the admin UI by
// pairing buildings and departments with all_computers too) live in the
// resource package, not here.
package scope
