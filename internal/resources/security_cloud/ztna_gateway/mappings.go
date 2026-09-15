// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"sort"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Every enumerated attribute on this resource is a fixed dropdown in the admin UI
// whose labels differ from the values the API stores, so each one is exposed as
// the UI label and translated at the boundary — the convention in
// STYLE_GUIDE §Translating UI labels/presets to wire values.
//
// The label tables below are keyed by the SDK's generated value constants — not
// string literals — so a value the SDK renames breaks the build, and the
// accepted-label slices are derived from its `*Values()` helpers, so a value it
// gains or loses fails TestLabelCoverage rather than silently disappearing from
// the schema. That is the only drift guard available here: as the style guide
// warns, a round-trip test passes by construction even when a label is mapped to
// the wrong value, so the table's correctness rests on the provenance below.
//
// Label provenance — observed means read off the admin UI in the screenshots this
// was built from:
//
//   - encryption, integrity: observed (`3DES`, `AES-128`, `AES-256`; `MD5`,
//     `SHA-1`, `SHA-256`, `SHA-512`).
//   - key exchange: `IKEv2` observed; `IKEv1` follows the same spelling.
//   - Diffie-Hellman: groups 2, 5, 14, 19, 20 and 21 observed. **`modp3072` and
//     `modp4096` are NOT offered by the admin UI** — both were wire-probed and
//     accepted on 2026-08-27, so the values are real, but their group numbers
//     (15 and 16) come from RFC 3526 rather than from a Jamf screen. Labels
//     unverified.
//
// Every stored value these tables map to was wire-probed and accepted on
// 2026-08-27, `ikev1` and the two extra Diffie-Hellman groups included. What is
// unverified is the label side of the two entries called out above.
//
// Nothing here changes what goes on the wire, so a wrong label is a documentation
// and ergonomics bug rather than a data-integrity one — but it would still be a
// bug, and these two entries are the ones to check first.
var (
	// encryptionLabels maps each stored encryption value to its admin-UI label.
	encryptionLabels = map[string]string{
		securitycloud.CipherSuiteConfigEncryption3des:   "3DES",
		securitycloud.CipherSuiteConfigEncryptionAes128: "AES-128",
		securitycloud.CipherSuiteConfigEncryptionAes256: "AES-256",
	}

	// integrityLabels maps each stored integrity value to its admin-UI label.
	integrityLabels = map[string]string{
		securitycloud.CipherSuiteConfigIntegrityMd5:    "MD5",
		securitycloud.CipherSuiteConfigIntegritySha1:   "SHA-1",
		securitycloud.CipherSuiteConfigIntegritySha256: "SHA-256",
		securitycloud.CipherSuiteConfigIntegritySha512: "SHA-512",
	}

	// dhGroupLabels maps each stored Diffie-Hellman value to its admin-UI label.
	// See the provenance note above for the two entries the UI does not offer.
	dhGroupLabels = map[string]string{
		securitycloud.CipherSuiteConfigDhGroupsModp1024: "Group 2 (modp1024)",
		securitycloud.CipherSuiteConfigDhGroupsModp1536: "Group 5 (modp1536)",
		securitycloud.CipherSuiteConfigDhGroupsModp2048: "Group 14 (modp2048)",
		securitycloud.CipherSuiteConfigDhGroupsModp3072: "Group 15 (modp3072)",
		securitycloud.CipherSuiteConfigDhGroupsModp4096: "Group 16 (modp4096)",
		securitycloud.CipherSuiteConfigDhGroupsEcp256:   "Group 19 (ecp256)",
		securitycloud.CipherSuiteConfigDhGroupsEcp384:   "Group 20 (ecp384)",
		securitycloud.CipherSuiteConfigDhGroupsEcp521:   "Group 21 (ecp521)",
	}

	// keyExchangeLabels maps each stored key-exchange value to its admin-UI label.
	keyExchangeLabels = map[string]string{
		securitycloud.GatewayIpSecRequestKeyExchangeIkev1: "IKEv1",
		securitycloud.GatewayIpSecRequestKeyExchangeIkev2: "IKEv2",
	}
)

// datacenterLabels maps each stored egress-region value to its admin-UI label.
// Both come from the SDK, which documents the pairing in a table on the
// availability-zone field, and every label was observed in the region dropdown.
var datacenterLabels = map[string]string{
	securitycloud.GatewayCreateRequestDatacenterAfSouth1:     "Africa - Cape Town",
	securitycloud.GatewayCreateRequestDatacenterApEast1:      "Asia - Hong Kong",
	securitycloud.GatewayCreateRequestDatacenterApNortheast1: "Asia - Japan",
	securitycloud.GatewayCreateRequestDatacenterApSouth1:     "Asia - Mumbai",
	securitycloud.GatewayCreateRequestDatacenterApSoutheast1: "Asia - Singapore",
	securitycloud.GatewayCreateRequestDatacenterApSoutheast2: "Australia",
	securitycloud.GatewayCreateRequestDatacenterCaCentral1:   "North America - Canada",
	securitycloud.GatewayCreateRequestDatacenterEuCentral1:   "Europe - Germany",
	securitycloud.GatewayCreateRequestDatacenterEuWest1:      "Europe - Ireland",
	securitycloud.GatewayCreateRequestDatacenterEuWest2:      "Europe - UK",
	securitycloud.GatewayCreateRequestDatacenterSaEast1:      "South America - Brazil",
	securitycloud.GatewayCreateRequestDatacenterUsEast1:      "North America - USA East",
	securitycloud.GatewayCreateRequestDatacenterUsWest2:      "North America - USA West",
}

// egressRegionValues returns the accepted egress-region labels, in the order the
// admin UI lists them — alphabetically, which is not the order the spec declares
// the underlying values in.
func egressRegionValues() []string {
	labels := labelsFor(securitycloud.GatewayCreateRequestDatacenterValues(), datacenterLabels)
	sort.Strings(labels)
	return labels
}

// keyExchangeValues returns the accepted key-exchange labels.
func keyExchangeValues() []string {
	return labelsFor(securitycloud.GatewayIpSecRequestKeyExchangeValues(), keyExchangeLabels)
}

// encryptionValues returns the accepted encryption labels.
func encryptionValues() []string {
	return labelsFor(securitycloud.CipherSuiteConfigEncryptionValues(), encryptionLabels)
}

// integrityValues returns the accepted integrity labels.
func integrityValues() []string {
	return labelsFor(securitycloud.CipherSuiteConfigIntegrityValues(), integrityLabels)
}

// diffieHellmanGroupValues returns the accepted Diffie-Hellman group labels.
func diffieHellmanGroupValues() []string {
	return labelsFor(securitycloud.CipherSuiteConfigDhGroupsValues(), dhGroupLabels)
}

// vendorValues returns the accepted remote-peer VPN vendors.
//
// This one needs no label table: the admin UI's dropdown shows the stored values
// verbatim, casing included — `strongSwan`, `Checkpoint`, `Palo Alto` and the
// rest. Inventing a translation here would add a table to keep in step for no
// change in what the user types.
func vendorValues() []string {
	return securitycloud.ConnectionConfigRightRequestVendorValues()
}

// labelsFor renders a set of stored values as their admin-UI labels, preserving
// the order of the input. A value with no label falls back to itself so an SDK
// addition surfaces as an odd-looking but usable schema value rather than an
// empty string; labelCoverage is what turns that into a test failure.
func labelsFor(values []string, labels map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if label, ok := labels[v]; ok {
			out = append(out, label)
			continue
		}
		out = append(out, v)
	}
	return out
}

// wireValueFor translates an admin-UI label back to the value the API stores.
// An unrecognised label passes through unchanged: the schema's OneOf validator has
// already rejected anything not in the table, so reaching here with an unknown
// label means the two disagree, and sending it lets the server produce the error
// rather than the provider sending an empty string.
func wireValueFor(label string, labels map[string]string) string {
	for wire, candidate := range labels {
		if candidate == label {
			return wire
		}
	}
	return label
}

// labelFor translates a stored value into its admin-UI label, falling back to the
// value itself. Read paths use this, so a value the table does not know still
// round-trips through state instead of blanking the attribute.
func labelFor(wire string, labels map[string]string) string {
	if label, ok := labels[wire]; ok {
		return label
	}
	return wire
}

// markdownList renders a value set as a comma-separated list of backticked
// literals, so a description and its validator are generated from one slice.
func markdownList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, "`"+v+"`")
	}
	return strings.Join(quoted, ", ")
}
