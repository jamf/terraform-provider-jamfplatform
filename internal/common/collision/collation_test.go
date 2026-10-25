// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import "testing"

func TestCollation_Equal(t *testing.T) {
	cases := []struct {
		name string
		c    Collation
		a, b string
		want bool
	}{
		{"folded case", CollationFolded, "zz-probe", "ZZ-PROBE", true},
		{"folded trailing space", CollationFolded, "zz-probe", "zz-probe ", true},
		{"folded accent", CollationFolded, "zz-probe", "zz-probé", true},
		{"folded leading space differs", CollationFolded, "zz-probe", " zz-probe", false},
		{"accent sensitive keeps accents apart", CollationAccentSensitive, "Cafe", "Café", false},
		{"accent sensitive folds case", CollationAccentSensitive, "Cafe", "CAFE", true},
		{"case sensitive keeps case apart", CollationCaseSensitive, "zz-probe", "ZZ-PROBE", false},
		{"case sensitive folds trailing space", CollationCaseSensitive, "zz-probe", "zz-probe  ", true},
		{"exact folds nothing", CollationExact, "a", "a ", false},
		{"decomposed accent", CollationFolded, "é", "e", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.Equal(tc.a, tc.b); got != tc.want {
				t.Fatalf("Equal(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCollation_Describe(t *testing.T) {
	cases := []struct {
		c    Collation
		want string
	}{
		{CollationFolded, "Names are compared without regard to case, trailing spaces or accents."},
		{CollationAccentSensitive, "Names are compared without regard to case or trailing spaces."},
		{CollationCaseSensitive, "Names are compared without regard to trailing spaces."},
		{CollationExact, "Names are compared exactly."},
	}
	for _, tc := range cases {
		if got := tc.c.Describe("name"); got != tc.want {
			t.Errorf("Describe = %q, want %q", got, tc.want)
		}
	}
}
