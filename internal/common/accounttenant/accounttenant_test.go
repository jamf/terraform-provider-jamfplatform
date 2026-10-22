// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package accounttenant

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

func TestInEnvironment(t *testing.T) {
	one, two := "env-1", "env-2"
	tenants := []account.Tenant{
		{ID: "a", EnvironmentID: &one},
		{ID: "b", EnvironmentID: &two},
		{ID: "c"},
		{ID: "d", EnvironmentID: &one},
	}
	got := InEnvironment(tenants, "env-1")
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "d" {
		t.Errorf("got %v, want a and d in order", got)
	}
	if got := InEnvironment(tenants, "env-9"); len(got) != 0 {
		t.Errorf("got %v for an environment holding nothing", got)
	}
	if got := InEnvironment(nil, "env-1"); len(got) != 0 {
		t.Errorf("got %v for no tenants", got)
	}
}

func TestDescribe(t *testing.T) {
	name, product, empty := "Corp", "pro", ""
	for _, tc := range []struct {
		tenant account.Tenant
		want   string
	}{
		{account.Tenant{ID: "t-1", Name: &name, Product: &product}, "Corp (t-1, pro)"},
		{account.Tenant{ID: "t-2"}, "(unnamed) (t-2)"},
		{account.Tenant{ID: "t-3", Name: &empty, Product: &empty}, "(unnamed) (t-3)"},
	} {
		if got := Describe(tc.tenant); got != tc.want {
			t.Errorf("Describe = %q, want %q", got, tc.want)
		}
	}
}
