// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"slices"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestPlatformLabels_CoverEverySDKValue fails when the SDK gains a platform this
// package has no label for.
//
// A round-trip test would pass by construction even with a wrong pairing, so the
// guard is coverage against the SDK's own value set rather than self-consistency
// (see .claude/skills/new-construct/references/naming.md).
func TestPlatformLabels_CoverEverySDKValue(t *testing.T) {
	values := securitycloud.PublicApiCreateActivationProfileRequestPlatformsValues()
	for _, v := range values {
		if _, ok := platformLabelByWire[v]; !ok {
			t.Errorf("SDK platform %q has no provider label", v)
		}
	}
	if got, want := len(platformLabels()), len(values); got != want {
		t.Errorf("platformLabels() returned %d labels, SDK declares %d values", got, want)
	}
}

// TestPlatformLabels_NoLabelOutlivesItsValue fails when a label no longer
// corresponds to any SDK value.
func TestPlatformLabels_NoLabelOutlivesItsValue(t *testing.T) {
	values := securitycloud.PublicApiCreateActivationProfileRequestPlatformsValues()
	for wire := range platformLabelByWire {
		if !slices.Contains(values, wire) {
			t.Errorf("label mapped from %q, which the SDK no longer declares", wire)
		}
	}
}

// TestPlatformLabels_DeclarationOrder pins that labels come back in the order the
// spec declares the values, which is what the schema's documented list shows.
func TestPlatformLabels_DeclarationOrder(t *testing.T) {
	if got, want := platformLabels(), []string{"ios", "mac"}; !slices.Equal(got, want) {
		t.Errorf("platformLabels() = %v, want %v", got, want)
	}
}

// TestPlatformWire_RoundTrip checks both directions agree for every known label.
func TestPlatformWire_RoundTrip(t *testing.T) {
	for wire, label := range platformLabelByWire {
		gotWire, ok := platformWire(label)
		if !ok {
			t.Errorf("platformWire(%q) not found", label)
			continue
		}
		if gotWire != wire {
			t.Errorf("platformWire(%q) = %q, want %q", label, gotWire, wire)
		}
	}
}

// TestPlatformWire_UnknownLabel reports not-found rather than an empty value, so
// the caller raises a diagnostic instead of sending "".
func TestPlatformWire_UnknownLabel(t *testing.T) {
	if _, ok := platformWire("windows"); ok {
		t.Error("platformWire accepted a platform Jamf Security Cloud does not take here")
	}
}

// TestSortedPlatformLabels_DoesNotMutateInput matters because the caller holds
// the slice it decoded from the plan.
func TestSortedPlatformLabels_DoesNotMutateInput(t *testing.T) {
	in := []string{"mac", "ios"}
	out := sortedPlatformLabels(in)
	if !slices.Equal(in, []string{"mac", "ios"}) {
		t.Errorf("input mutated to %v", in)
	}
	if !slices.Equal(out, []string{"ios", "mac"}) {
		t.Errorf("sortedPlatformLabels = %v, want [ios mac]", out)
	}
}
