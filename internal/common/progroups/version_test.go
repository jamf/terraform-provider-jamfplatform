// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"strings"
	"testing"
)

// TestRefusalDiagnostics pins the version table the family refuses on. The two
// cases that matter most are the boundaries and the unparseable one: 11.30.1 is
// the fix and must be allowed, and a version string the provider cannot read
// must NOT be refused, because a refusal on "cannot tell" would turn one odd
// response into an outage across all sixteen constructs.
func TestRefusalDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		version string
		refuse  bool
	}{
		{name: "well below the window", version: "11.20.0"},
		{name: "immediately below the window", version: "11.28.9"},
		{name: "the window opens", version: "11.29.0", refuse: true},
		{name: "inside the window", version: "11.29.4", refuse: true},
		{name: "last version in the window", version: "11.30.0", refuse: true},
		{name: "the fix", version: "11.30.1"},
		{name: "above the fix", version: "11.32.0"},
		{name: "build suffix inside the window", version: "11.29.0-t1787580540993", refuse: true},
		{name: "build suffix on the fix", version: "11.30.1-t1787580540993"},
		{name: "unparseable is not refused", version: "not-a-version"},
		{name: "empty is not refused"},
		{name: "two segments is not refused", version: "11.29"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RefusalDiagnostics(tc.version, "jamfplatform_pro_smart_computer_group")
			if got.HasError() != tc.refuse {
				t.Fatalf("version %q: refused = %v, want %v (%v)", tc.version, got.HasError(), tc.refuse, got)
			}
			if !tc.refuse {
				return
			}
			detail := got.Errors()[0].Detail()
			for _, want := range []string{"jamfplatform_pro_smart_computer_group", tc.version, "11.29.0", "11.30.1", "jamfplatform_device_group"} {
				if !strings.Contains(detail, want) {
					t.Errorf("detail does not mention %q:\n%s", want, detail)
				}
			}
		})
	}
}

// TestRequireSupportedJamfProVersionNilData pins that a construct configured
// before the framework has handed over provider data reports nothing, rather
// than dereferencing nil or refusing on an unknown version.
func TestRequireSupportedJamfProVersionNilData(t *testing.T) {
	if got := RequireSupportedJamfProVersion(t.Context(), nil, "jamfplatform_pro_static_computer_group"); got.HasError() {
		t.Fatalf("nil provider data produced errors: %v", got)
	}
}
