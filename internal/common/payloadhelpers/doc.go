// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package payloadhelpers holds the .mobileconfig handling the two classic
// configuration profile resources share: the mask, the comparators built on it,
// the update-path identifier injection, the read-back diagnostic and the import
// gate. Every function is pure: it works on byte slices and parsed plist trees
// and carries no resource state.
//
// # Why a mask rather than a model
//
// The classic endpoints (osxconfigurationprofiles,
// mobiledeviceconfigurationprofiles) re-serialise the submitted plist. Jamf Pro
// strips whitespace, normalises the version ("1.0" becomes "1"), stamps
// defaults, rewrites the top-level UUID and identifier, and defaults each
// payload's display name from its PayloadType. Predicting those mutations
// needs a table of server defaults per PayloadType that goes stale whenever
// Jamf Pro changes one. MaskPayload instead drops or normalises the
// server-controlled parts from both sides, and the two masked trees are
// compared. The mask knows nothing about any payload's content, so a new
// server default needs no change here.
//
// The cost is that an authored change to a masked key is not detected as
// drift. Jamf Pro overwrites those keys on every write, so the alternative is
// a diff that never settles. Each resource's payloads description lists the
// masked keys for that reason.
//
// # One mask, several comparators
//
//   - LenientEqualPlist compares the keys both sides carry. It backs plan-time
//     diff suppression and the read-back check after a write.
//   - ThreeWayCompare and PayloadsStructurallyEqual compare strictly, keysets
//     included, to tell an edit made in Jamf Pro from a steady refresh.
//   - diffPayloadTrees names the leaves that differ. It backs
//     PayloadFidelityErrorDetail and PayloadImportRisk.
//
// The comparators go through MaskPayload and the differ skips the same masked
// keys, so a canonicalisation added to the mask reaches the comparators at
// once. That sharing is what stops the plan modifier, the read-back check and
// the import gate from disagreeing about the same payload, and it is why a
// resource must not compare payloads any other way.
//
// What the mask does and why is documented where it happens: the masked key
// lists and normalizeLineEndings in helpers.go, PayloadContent ordering on
// canonicalisePayloadContentOrder, the web clip icon tolerance in
// webclipicon.go and on structuralEqual, and the storage categories in
// importgate.go.
//
// # Read-back diagnostics
//
// A create whose stored payload fails the comparison is rolled back. An update
// cannot be: once a profile holds the mangled form, the Classic API refuses the
// original value back with "409 Unable to update the database", and the repair
// is made in the Jamf Pro admin UI. PayloadFidelityErrorDetail therefore has
// to tell the operator which value to change, and two rules keep it useful:
//
//   - A finding is reported only for a leaf the comparison rejected, through
//     the same intersection, numeric leniency and icon equivalence. The differ
//     must never return nothing for two trees that compared unequal.
//   - No explanation is printed unless a finding calls for it. A diagnostic
//     naming characters the payload does not contain sends the operator after
//     the wrong value.
//
// # Identifier injection
//
// InjectTopLevelIdentifierValues carries the stored top-level PayloadUUID and
// PayloadIdentifier into an update. The top level is enough: the identifiers
// of PayloadContent entries survive a PUT once stored.
package payloadhelpers
