// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"slices"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestLabelCoverage is the drift guard the label tables rest on: every value the SDK
// generates must have a label, and every label must map back to a value the SDK
// generates. A round-trip test alone passes by construction even when a label points
// at the wrong value, so this checks the two sets rather than the journey.
func TestLabelCoverage(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		labels map[string]string
	}{
		{"routing mode", securitycloud.RoutingTypeValues(), routingModeLabels},
		{"dns resolution", securitycloud.RoutingDnsIpResolutionTypeValues(), dnsResolutionLabels},
		{"risk level", securitycloud.RiskControlsLevelThresholdValues(), riskLevelLabels},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.values) != len(tc.labels) {
				t.Errorf("SDK generates %d values but the table carries %d labels", len(tc.values), len(tc.labels))
			}
			for _, v := range tc.values {
				label, ok := tc.labels[v]
				if !ok {
					t.Errorf("SDK value %q has no label", v)
					continue
				}
				if label == "" {
					t.Errorf("SDK value %q maps to an empty label", v)
				}
			}
			for wire := range tc.labels {
				found := slices.Contains(tc.values, wire)
				if !found {
					t.Errorf("table has a label for %q, which the SDK no longer generates", wire)
				}
			}
		})
	}
}

// TestLabelsAreDistinct pins that no two values in a set share a label, which
// wireValueFor's reverse scan would otherwise resolve arbitrarily.
func TestLabelsAreDistinct(t *testing.T) {
	for name, labels := range map[string]map[string]string{
		"routing mode":   routingModeLabels,
		"dns resolution": dnsResolutionLabels,
		"risk level":     riskLevelLabels,
	} {
		t.Run(name, func(t *testing.T) {
			seen := map[string]string{}
			for wire, label := range labels {
				if other, dup := seen[label]; dup {
					t.Errorf("label %q maps to both %q and %q", label, other, wire)
				}
				seen[label] = wire
			}
		})
	}
}

// TestLabelRoundTrip pins that a label survives the journey to the wire and back,
// which is what the read and write paths each do once per value.
func TestLabelRoundTrip(t *testing.T) {
	for name, labels := range map[string]map[string]string{
		"routing mode":   routingModeLabels,
		"dns resolution": dnsResolutionLabels,
		"risk level":     riskLevelLabels,
	} {
		t.Run(name, func(t *testing.T) {
			for wire, label := range labels {
				if got := wireValueFor(label, labels); got != wire {
					t.Errorf("wireValueFor(%q) = %q, want %q", label, got, wire)
				}
				if got := labelFor(wire, labels); got != label {
					t.Errorf("labelFor(%q) = %q, want %q", wire, got, label)
				}
			}
		})
	}
}

// TestLabelFallbacks pin the behaviour on a value or label the table does not know.
// Both fall through unchanged rather than blanking, so an SDK addition surfaces as an
// odd-looking but usable value instead of an empty string — TestLabelCoverage is what
// turns that into a failure.
func TestLabelFallbacks(t *testing.T) {
	if got := labelFor("SOMETHING_NEW", routingModeLabels); got != "SOMETHING_NEW" {
		t.Errorf("labelFor on an unknown value = %q, want it unchanged", got)
	}
	if got := wireValueFor("Something New", routingModeLabels); got != "Something New" {
		t.Errorf("wireValueFor on an unknown label = %q, want it unchanged", got)
	}
}

// TestBlockIsNotExposed pins that the API-internal third routing type stays out of
// the schema. The SDK deliberately does not generate it, so a label table keyed on
// the SDK cannot carry it — this is the assertion that says that is intentional.
func TestBlockIsNotExposed(t *testing.T) {
	for _, label := range routingModeValues() {
		if label == "BLOCK" || label == "Block" {
			t.Fatalf("routing mode values expose the API-internal BLOCK type: %v", routingModeValues())
		}
	}
	if _, ok := routingModeLabels["BLOCK"]; ok {
		t.Fatal("routingModeLabels carries a label for BLOCK")
	}
}

// TestDNSResolutionValuesOrder pins that the documented list leads with the
// recommended setting, which is the order the admin UI's dropdown uses and the
// opposite of the order the SDK declares the values in.
func TestDNSResolutionValuesOrder(t *testing.T) {
	got := dnsResolutionValues()
	if len(got) == 0 || got[0] != "Standard" {
		t.Fatalf("dnsResolutionValues() = %v, want it to lead with Standard", got)
	}
}

// TestRiskLevelValuesOrder pins lowest-first, matching both the SDK's declaration
// order and the dropdown.
func TestRiskLevelValuesOrder(t *testing.T) {
	want := []string{"Low", "Medium", "High"}
	got := riskLevelValues()
	if len(got) != len(want) {
		t.Fatalf("riskLevelValues() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("riskLevelValues() = %v, want %v", got, want)
		}
	}
}

// TestRoutingModeLabelsAreLiteral is the only assertion in this package that can
// catch a transposed routing-mode label. Every other routing test derives its
// expectation from the table under test: TestLabelCoverage compares the two sets
// against the SDK and never looks at label content, TestLabelRoundTrip is true by
// construction for any bijection, and the builder tests use
// routingModeLabels[...] as their own input. Swapping the two entries therefore
// survives all of them, and the regression that admits is a security-relevant
// misroute — an app written `traffic_routing = "Direct device routing"` created as
// CUSTOM, or one meant for ZTNA created DIRECT and bypassing the access gateway. It
// would ship green, because the Security Cloud acceptance tests skip in CI by
// design. Both labels were read off the admin UI on 2026-08-30 and the direction was
// confirmed against the live API: CUSTOM is the type that requires a gatewayId.
// This is the routing-mode equivalent of TestDNSResolutionValuesOrder and
// TestRiskLevelValuesOrder, which escape the same trap only because they happen to
// assert literal strings.
func TestRoutingModeLabelsAreLiteral(t *testing.T) {
	if got := routingModeLabels[securitycloud.RoutingTypeCustom]; got != "Encrypt and route via ZTNA" {
		t.Errorf("routingModeLabels[%s] = %q, want %q — it is the type that sends traffic through the access gateway",
			securitycloud.RoutingTypeCustom, got, "Encrypt and route via ZTNA")
	}
	if got := routingModeLabels[securitycloud.RoutingTypeDirect]; got != "Direct device routing" {
		t.Errorf("routingModeLabels[%s] = %q, want %q — it is the type that bypasses the access gateway",
			securitycloud.RoutingTypeDirect, got, "Direct device routing")
	}
}

// TestAppTypeFor pins the derivation of the app-type label from the presence of a
// predefined app ID — the only thing on the wire that distinguishes the two forms.
func TestAppTypeFor(t *testing.T) {
	predefined := "2aaa401c-232e-4db1-8384-6a94d9fc264e"
	empty := ""

	cases := []struct {
		name  string
		input *string
		want  string
	}{
		{"nil", nil, appTypeCustom},
		{"empty string", &empty, appTypeCustom},
		{"uuid", &predefined, appTypePredefined},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := appTypeFor(tc.input); got != tc.want {
				t.Errorf("appTypeFor(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestMarkdownList pins the rendering shared by the descriptions and the validators,
// so a documented value set cannot drift from the validated one.
func TestMarkdownList(t *testing.T) {
	if got := markdownList([]string{"Low", "High"}); got != "`Low`, `High`" {
		t.Errorf("markdownList = %q", got)
	}
	if got := markdownList(nil); got != "" {
		t.Errorf("markdownList(nil) = %q, want empty", got)
	}
}
