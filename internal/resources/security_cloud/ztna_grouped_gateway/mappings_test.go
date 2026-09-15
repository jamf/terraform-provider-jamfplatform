// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestRoutingStrategyLabelCoverage is the drift guard on the strategy table. A
// round-trip test passes by construction even with a wrong pairing, so what it can
// usefully catch is a value the SDK gained with no label — which would surface in
// the schema as a raw `ACTIVE_STANDBY`-style constant — or a label left behind for
// a value the SDK dropped.
func TestRoutingStrategyLabelCoverage(t *testing.T) {
	values := securitycloud.RoutingStrategyValues()
	if len(values) == 0 {
		t.Fatal("the SDK reports no routing strategies; the OneOf validator would accept nothing")
	}

	known := make(map[string]bool, len(values))
	for _, v := range values {
		known[v] = true
		if _, ok := routingStrategyLabels[v]; !ok {
			t.Errorf("the SDK accepts %q but no admin-UI label is mapped for it", v)
		}
	}
	for v := range routingStrategyLabels {
		if !known[v] {
			t.Errorf("a label is mapped for %q, which the SDK no longer accepts", v)
		}
	}
}

// TestRoutingStrategyLabelsAreDistinct guards the reverse lookup, which scans for a
// label: two strategies sharing one would make the translation ambiguous.
func TestRoutingStrategyLabelsAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for wire, label := range routingStrategyLabels {
		if other, dup := seen[label]; dup {
			t.Errorf("label %q is mapped from both %q and %q; the reverse lookup would be ambiguous", label, other, wire)
		}
		seen[label] = wire
	}
}

func TestRoutingStrategyRoundTrip(t *testing.T) {
	for _, wire := range securitycloud.RoutingStrategyValues() {
		label := labelForStrategy(wire)
		if label == wire {
			t.Errorf("labelForStrategy(%q) returned the stored value; every strategy should have a label", wire)
		}
		if got := wireStrategyFor(label); got != wire {
			t.Errorf("wireStrategyFor(%q) = %q, want %q", label, got, wire)
		}
	}
}

// TestRoutingStrategyFirstAvailablePairing pins the one inferred pairing in this
// package, so a future edit that reassigns it has to do so deliberately. The UI
// calls ACTIVE_STANDBY "First available" — nothing else in the set could.
func TestRoutingStrategyFirstAvailablePairing(t *testing.T) {
	if got := labelForStrategy(securitycloud.RoutingStrategyActiveStandby); got != "First available" {
		t.Errorf("ACTIVE_STANDBY label = %q, want \"First available\"", got)
	}
	if got := wireStrategyFor("First available"); got != securitycloud.RoutingStrategyActiveStandby {
		t.Errorf("\"First available\" maps to %q, want ACTIVE_STANDBY", got)
	}
}

// TestGatewayStabilityDurations pins the numbers against the server's own rejection
// message: "must be one of the supported durations in seconds: 300 (5 min), 1800
// (30 min), 3600 (1 h), 10800 (3 h), 28800 (8 h)". This table has no SDK helper
// behind it, so this test is the only thing holding the accepted set in place.
func TestGatewayStabilityDurations(t *testing.T) {
	want := []int64{300, 1800, 3600, 10800, 28800}

	if len(gatewayStabilityOrder) != len(want) {
		t.Fatalf("stability durations = %v, want %v", gatewayStabilityOrder, want)
	}
	for i := range want {
		if gatewayStabilityOrder[i] != want[i] {
			t.Fatalf("stability durations = %v, want %v (shortest first)", gatewayStabilityOrder, want)
		}
	}
	for _, seconds := range want {
		if _, ok := gatewayStabilityLabels[seconds]; !ok {
			t.Errorf("no label mapped for %d seconds", seconds)
		}
	}
	if len(gatewayStabilityLabels) != len(want) {
		t.Errorf("stability label table has %d entries, want %d", len(gatewayStabilityLabels), len(want))
	}
	if _, zeroMapped := gatewayStabilityLabels[0]; zeroMapped {
		t.Error("zero must not be an accepted duration; Jamf Security Cloud refuses it")
	}
}

func TestGatewayStabilityRoundTrip(t *testing.T) {
	for _, seconds := range gatewayStabilityOrder {
		label := labelForStability(seconds)
		if got := wireStabilityFor(label); got != seconds {
			t.Errorf("wireStabilityFor(%q) = %d, want %d", label, got, seconds)
		}
	}
}

// TestGatewayStabilityLegacyValue covers the case the SDK warns about: a grouped
// gateway created before the current constraint can hold a duration outside the
// accepted set, and a read of it must stay legible rather than blanking.
func TestGatewayStabilityLegacyValue(t *testing.T) {
	if got := labelForStability(7200); got != "7200 seconds" {
		t.Errorf("labelForStability on a legacy duration = %q, want it rendered rather than dropped", got)
	}
	if got := wireStabilityFor("nonsense"); got != 0 {
		t.Errorf("wireStabilityFor on an unmapped label = %d, want 0", got)
	}
}

// TestAcceptedValueSetsAreNonEmpty guards the schema: both slices feed a OneOf
// validator and a documented list, so a silently empty one would make the
// attribute accept anything and document nothing.
func TestAcceptedValueSetsAreNonEmpty(t *testing.T) {
	if len(routingStrategyValues()) == 0 {
		t.Error("routing strategy value set is empty")
	}
	if len(gatewayStabilityValues()) == 0 {
		t.Error("gateway stability value set is empty")
	}
}

// TestGatewayStabilityValuesAreShortestFirst pins the documented order, which is
// the order the admin UI's dropdown presents them in.
func TestGatewayStabilityValuesAreShortestFirst(t *testing.T) {
	values := gatewayStabilityValues()
	if values[0] != "5 minutes" || values[len(values)-1] != "8 hours" {
		t.Errorf("stability labels = %v, want shortest first", values)
	}
}
