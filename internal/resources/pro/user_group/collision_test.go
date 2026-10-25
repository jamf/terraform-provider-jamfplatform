// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import "testing"

func TestUserGroupNamespace_IsCaseSensitive(t *testing.T) {
	ns := userGroupNamespace(nil)
	if !ns.Enforced {
		t.Fatal("Jamf Pro refuses a duplicate user group name, so the namespace is enforced")
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"Lab", "Lab", true},
		{"Lab", "Lab ", true},
		{"Lab", " Lab", true},
		{"Lab", "LAB", false},
		{"Cafe", "Café", false},
	}
	for _, tc := range cases {
		if got := ns.Collation.Equal(tc.a, tc.b); got != tc.want {
			t.Errorf("Equal(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
