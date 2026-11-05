// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/deprecations"
)

// TestDeprecationMarkersAreWellFormed fails when a schema deprecation marker cannot be read by
// scripts/deprecationsweep. It checks the format only and never the date: the sweep workflow is
// what reports a marker whose window has passed, and a test that failed on a date would break
// unrelated work on a day nobody chose.
func TestDeprecationMarkersAreWellFormed(t *testing.T) {
	markers, problems, err := deprecations.Scan("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s", p)
	}
	if len(markers) == 0 && len(problems) == 0 {
		t.Log("no deprecation markers in the tree")
	}
}
