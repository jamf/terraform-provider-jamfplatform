// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// enforcement_mode validates against explicit SDK constants rather than
// BenchmarkRequestV2EnforcementModeValues(), so validation cannot silently widen
// on an SDK bump while the attribute description still names two modes in prose.
// This is the tripwire for the opposite risk: a mode Jamf adds going unnoticed.
func TestEnforcementModeEnum_HasNotGrown(t *testing.T) {
	want := map[string]bool{
		compliancebenchmarks.BenchmarkRequestV2EnforcementModeMonitor:           true,
		compliancebenchmarks.BenchmarkRequestV2EnforcementModeMonitorAndEnforce: true,
	}
	got := compliancebenchmarks.BenchmarkRequestV2EnforcementModeValues()
	for _, v := range got {
		if !want[v] {
			t.Errorf("BenchmarkRequestV2EnforcementMode gained value %q: update the enforcement_mode validator and its description, or state why it is excluded", v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("BenchmarkRequestV2EnforcementMode has %d values, schema validates %d", len(got), len(want))
	}
}
