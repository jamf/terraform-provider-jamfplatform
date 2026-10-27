// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import "testing"

func TestPolicyNamespace_FollowsTheClassicCollation(t *testing.T) {
	ns := policyNamespace(nil)
	if !ns.Enforced {
		t.Fatal("Jamf Pro refuses a duplicate policy name, so the namespace is enforced")
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"Install Chrome", "INSTALL CHROME", true},
		{"Install Chrome", "Install Chrome ", true},
		{"Install Chrome", " Install Chrome", true},
		{"Cafe", "Café", false},
	}
	for _, tc := range cases {
		if got := ns.Collation.Equal(tc.a, tc.b); got != tc.want {
			t.Errorf("Equal(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
