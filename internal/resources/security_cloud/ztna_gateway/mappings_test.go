// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// labelTable pairs a label map with the SDK value set it must cover, so one table
// drives every coverage assertion.
type labelTable struct {
	name   string
	labels map[string]string
	values []string
}

// labelTables enumerates every admin-UI label mapping on this resource.
func labelTables() []labelTable {
	return []labelTable{
		{"egress region", datacenterLabels, securitycloud.GatewayCreateRequestDatacenterValues()},
		{"key exchange", keyExchangeLabels, securitycloud.GatewayIpSecRequestKeyExchangeValues()},
		{"encryption", encryptionLabels, securitycloud.CipherSuiteConfigEncryptionValues()},
		{"integrity", integrityLabels, securitycloud.CipherSuiteConfigIntegrityValues()},
		{"Diffie-Hellman group", dhGroupLabels, securitycloud.CipherSuiteConfigDhGroupsValues()},
	}
}

// TestLabelCoverage is the drift guard the label tables rest on. A round-trip test
// passes by construction even when a label is mapped to the wrong value, so what
// it can usefully catch is a value the SDK gained that has no label — which would
// otherwise appear in the schema as a raw wire value — or a label left behind for
// a value the SDK dropped.
func TestLabelCoverage(t *testing.T) {
	for _, table := range labelTables() {
		t.Run(table.name, func(t *testing.T) {
			known := make(map[string]bool, len(table.values))
			for _, v := range table.values {
				known[v] = true
				if _, ok := table.labels[v]; !ok {
					t.Errorf("the SDK accepts %q but no admin-UI label is mapped for it; the schema would expose the raw value", v)
				}
			}
			for v := range table.labels {
				if !known[v] {
					t.Errorf("a label is mapped for %q, which the SDK no longer accepts", v)
				}
			}
		})
	}
}

// TestLabelsAreDistinct guards the reverse lookup: wireValueFor scans for a label,
// so two values sharing one label would make the translation ambiguous and silently
// pick whichever the map iteration reached first.
func TestLabelsAreDistinct(t *testing.T) {
	for _, table := range labelTables() {
		t.Run(table.name, func(t *testing.T) {
			seen := map[string]string{}
			for wire, label := range table.labels {
				if other, dup := seen[label]; dup {
					t.Errorf("label %q is mapped from both %q and %q; the reverse lookup would be ambiguous", label, other, wire)
				}
				seen[label] = wire
			}
		})
	}
}

// TestLabelRoundTrip covers the mechanics of the two translation helpers in both
// directions. It cannot verify the pairings themselves — see the provenance note
// in mappings.go for which of those were read off the admin UI and which were not.
func TestLabelRoundTrip(t *testing.T) {
	for _, table := range labelTables() {
		t.Run(table.name, func(t *testing.T) {
			for _, wire := range table.values {
				label := labelFor(wire, table.labels)
				if label == wire && len(table.labels) > 0 {
					if _, mapped := table.labels[wire]; mapped {
						t.Errorf("labelFor(%q) returned the wire value even though a label is mapped", wire)
					}
				}
				if got := wireValueFor(label, table.labels); got != wire {
					t.Errorf("wireValueFor(labelFor(%q)) = %q, want %q", wire, got, wire)
				}
			}
		})
	}
}

// TestLabelFallbacks pin the deliberate pass-through behaviour. An unmapped value
// reaching either helper means the table and the SDK disagree, and returning the
// input unchanged keeps the attribute readable and lets the server produce the
// error, rather than blanking state or sending an empty string.
func TestLabelFallbacks(t *testing.T) {
	if got := labelFor("aes999", encryptionLabels); got != "aes999" {
		t.Errorf("labelFor on an unmapped value = %q, want the value unchanged", got)
	}
	if got := wireValueFor("AES-999", encryptionLabels); got != "AES-999" {
		t.Errorf("wireValueFor on an unmapped label = %q, want the label unchanged", got)
	}
}

// TestAcceptedValueSetsAreNonEmpty guards the schema itself: every accepted-value
// slice feeds both a OneOf validator and a documented list, so a silently empty
// one would make the attribute accept anything and document nothing.
func TestAcceptedValueSetsAreNonEmpty(t *testing.T) {
	for name, values := range map[string][]string{
		"egress region":        egressRegionValues(),
		"key exchange":         keyExchangeValues(),
		"encryption":           encryptionValues(),
		"integrity":            integrityValues(),
		"Diffie-Hellman group": diffieHellmanGroupValues(),
		"vendor":               vendorValues(),
	} {
		if len(values) == 0 {
			t.Errorf("%s value set is empty; the OneOf validator would accept nothing and the docs would list nothing", name)
		}
	}
}

// TestEgressRegionValuesAreAlphabetical pins the ordering, which is what the admin
// UI's region dropdown uses — and which is not the order the spec declares the
// underlying values in.
func TestEgressRegionValuesAreAlphabetical(t *testing.T) {
	values := egressRegionValues()
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			t.Fatalf("egress regions are not sorted: %q precedes %q", values[i-1], values[i])
		}
	}
	if values[0] != "Africa - Cape Town" {
		t.Errorf("first egress region = %q, want the alphabetically first label", values[0])
	}
}

// TestVendorValuesNeedNoTranslation pins the one enum left on stored values: the
// admin UI shows them verbatim, casing included. `strongswan` is refused and
// `strongSwan` accepted (wire-probed 2026-08-27), and the server's reply to the
// wrong casing names neither the field nor the value.
func TestVendorValuesNeedNoTranslation(t *testing.T) {
	var found bool
	for _, v := range vendorValues() {
		if v == "strongSwan" {
			found = true
		}
		if v == "strongswan" {
			t.Error("vendor set contains the lowercase spelling, which Jamf Security Cloud refuses")
		}
	}
	if !found {
		t.Error("vendor set is missing strongSwan")
	}
}
