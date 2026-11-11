// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestPickLAPSAccountGUID(t *testing.T) {
	accounts := []pro.LapsUserV2{
		{Username: "madmin", Guid: "G-JMF", UserSource: "JMF"},
		{Username: "mdmadmin", Guid: "G-MDM", UserSource: "MDM"},
		{Username: "dup", Guid: "G-1", UserSource: "JMF"},
		{Username: "dup", Guid: "G-2", UserSource: "MDM"},
		{Username: "noguid", UserSource: "MDM"},
	}
	cases := map[string]struct {
		username string
		want     string
		errPart  string
	}{
		"match":     {"mdmadmin", "G-MDM", ""},
		"jmf":       {"madmin", "G-JMF", ""},
		"case":      {"MADMIN", "", "no managed local administrator account"},
		"absent":    {"ghost", "", "madmin (JMF)"},
		"ambiguous": {"dup", "", "ambiguous"},
		"no guid":   {"noguid", "", "without a GUID"},
	}
	for name, c := range cases {
		got, problem := pickLAPSAccountGUID(accounts, c.username)
		if c.errPart == "" {
			if problem != "" || got != c.want {
				t.Errorf("%s: got %q, %q; want %q", name, got, problem, c.want)
			}
			continue
		}
		if !strings.Contains(problem, c.errPart) {
			t.Errorf("%s: problem %q does not contain %q", name, problem, c.errPart)
		}
	}
	if _, problem := pickLAPSAccountGUID(nil, "x"); !strings.Contains(problem, "none") {
		t.Errorf("empty list: %q", problem)
	}
}
