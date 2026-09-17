// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package progroups holds the machinery the four Jamf Pro device-group
// constructs share: jamfplatform_pro_smart_computer_group,
// jamfplatform_pro_static_computer_group,
// jamfplatform_pro_smart_mobile_device_group and
// jamfplatform_pro_static_mobile_device_group, each with a singular data source,
// a plural data source and a list resource.
//
// # Why these exist at all
//
// jamfplatform_device_group already manages computer and mobile-device groups,
// smart and static, through Platform Services. It is the one an operator should
// reach for. What it cannot do is put a group in a Jamf Pro site: the
// /device-groups/v1 surface has no site field and never sends one. These
// constructs exist for that single gap, which is why every schema description
// says so and points back to jamfplatform_device_group — see
// DeviceGroupAlternative.
//
// # The wire laws behind the shared helpers
//
// All wire facts below were probed on 2026-09-17 against Jamf Pro 11.32.0 on the
// EU gateway under an environment-scoped integration, except where marked.
//
// Every write is a full replace of the object's scalars. A body that omits
// `description`/`groupDescription` clears it to the empty string, and one that
// omits `siteId` clears the site to the "-1" sentinel. So every write emits every
// scalar; nothing relies on omission to retain.
//
// `siteId` is mandatory on both mobile endpoints and optional on both computer
// endpoints. The mobile refusal is 403 with code INVALID_PRIVILEGE naming
// `field: siteId`, which reads as a permission problem and is not one — a site id
// that does not exist draws the same code with a null field. Because the
// always-emit rule above already sends `siteId` on every write, the asymmetry
// costs callers nothing; SiteRefusal translates the code when it does surface.
//
// `assignments` is mandatory on every static write — create and update, computer
// and mobile. Omitting it answers 500 with an empty `errors` array and applies
// nothing. The SDK records the create half of this under "server bugs worth
// reporting" in its docs/WIRE-FACTS.md; the update half and the mobile endpoint
// behave identically. Static membership therefore always emits the key, and
// "leave membership unmanaged" is synthesised provider-side rather than by
// omitting the field.
//
// The two static endpoints then disagree about what a populated `assignments`
// means, which is the single most important difference in this family. The
// computer endpoint (PUT /pro/v3/computer-groups/static-groups/{id}, a []string
// of Jamf Pro computer ids) REPLACES the membership, and [] clears it. The mobile
// endpoint (PATCH /pro/v2/mobile-device-groups/static-groups/{id}, a []Assignment
// carrying `selected`) DELTA-MERGES: an entry with selected true adds, selected
// false removes, other members are untouched, and [] is a no-op that leaves
// membership exactly as it was. Same field name, same concept, opposite
// semantics, one API generation apart.
//
// A static group that has a site accepts only devices assigned to that same site.
// Anything else is 400 INVALID_DEVICE on `field: assignments` with nothing
// applied, and the same code covers a device id that does not exist — probed both
// ways round, including moving a computer into the site and watching the identical
// request start succeeding. InvalidDevice names both possibilities because the
// server does not distinguish them.
//
// Deleting a group that anything is scoped to is 422 HAS_DEPENDENCIES, and the
// description names the dependents. It is permanent rather than
// propagation-blocked, so it is translated and returned, never re-issued or
// polled — the opposite of device_group's delete. See Dependencies.
//
// Every /pro/v2 and /pro/v3 group endpoint accepts a group's platform UUID in
// place of {id} and always answers with the Jamf Pro numeric id, so the two id
// spaces are interchangeable on input. These constructs key on the Jamf Pro id
// (matching every other pro/ construct) and surface the platform UUID as a
// Computed attribute through ResolvePlatformID.
package progroups
