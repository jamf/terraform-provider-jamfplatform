// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platformenvironmentaction

import (
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

func TestPlanAssignment(t *testing.T) {
	eu := &account.Environment{ID: "env-eu", Name: "Europe", MetaRegion: new("EU")}
	global := &account.Environment{ID: "env-global", Name: "Anywhere"}

	tenants := []account.Tenant{
		{ID: "free-eu", MetaRegion: new("EU")},
		{ID: "free-us", MetaRegion: new("US")},
		{ID: "free-noregion"},
		{ID: "inside", MetaRegion: new("EU"), EnvironmentID: new("env-eu")},
		{ID: "elsewhere", MetaRegion: new("EU"), EnvironmentID: new("env-other")},
		{ID: "emptyenv", MetaRegion: new("EU"), EnvironmentID: new("")},
	}

	cases := []struct {
		name          string
		environment   *account.Environment
		requested     []string
		wantAssign    []string
		wantAlreadyIn []string
		wantProblems  []string
	}{
		{"matching region", eu, []string{"free-eu"}, []string{"free-eu"}, nil, nil},
		{"region mismatch", eu, []string{"free-us"}, nil, nil, []string{"hosted in US"}},
		{"global accepts any region", global, []string{"free-eu", "free-us"}, []string{"free-eu", "free-us"}, nil, nil},
		{"tenant without a region is left to the server", eu, []string{"free-noregion"}, []string{"free-noregion"}, nil, nil},
		{"already inside is skipped", eu, []string{"inside"}, nil, []string{"inside"}, nil},
		{"another environment is refused", eu, []string{"elsewhere"}, nil, nil, []string{"already in another platform environment (env-other)"}},
		{"empty environment id counts as unassigned", eu, []string{"emptyenv"}, []string{"emptyenv"}, nil, nil},
		{"unknown id", eu, []string{"ghost"}, nil, nil, []string{"ghost: not a tenant of this organization"}},
		{"mixed", eu, []string{"free-eu", "inside", "free-us", "ghost"}, []string{"free-eu"}, []string{"inside"}, []string{"hosted in US", "ghost"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := planAssignment(tc.environment, tenants, tc.requested)

			ids := func(ts []account.Tenant) []string {
				var out []string
				for _, t := range ts {
					out = append(out, t.ID)
				}
				return out
			}
			if got := strings.Join(ids(plan.toAssign), ","); got != strings.Join(tc.wantAssign, ",") {
				t.Errorf("toAssign = %q, want %q", got, strings.Join(tc.wantAssign, ","))
			}
			if got := strings.Join(ids(plan.alreadyIn), ","); got != strings.Join(tc.wantAlreadyIn, ",") {
				t.Errorf("alreadyIn = %q, want %q", got, strings.Join(tc.wantAlreadyIn, ","))
			}
			if len(plan.problems) != len(tc.wantProblems) {
				t.Fatalf("problems = %v, want %d", plan.problems, len(tc.wantProblems))
			}
			for i, want := range tc.wantProblems {
				if !strings.Contains(plan.problems[i], want) {
					t.Errorf("problem %d = %q, want it to contain %q", i, plan.problems[i], want)
				}
			}
		})
	}
}

func TestNotInEnvironment(t *testing.T) {
	tenants := []account.Tenant{
		{ID: "a", EnvironmentID: new("env-1")},
		{ID: "b", EnvironmentID: new("env-2")},
		{ID: "c"},
	}
	got := notInEnvironment(tenants, "env-1", []string{"a", "b", "c", "d"})
	if strings.Join(got, ",") != "b,c,d" {
		t.Errorf("got %v, want b, c and d", got)
	}
	if got := notInEnvironment(tenants, "env-1", []string{"a"}); len(got) != 0 {
		t.Errorf("got %v, want nothing missing", got)
	}
}
