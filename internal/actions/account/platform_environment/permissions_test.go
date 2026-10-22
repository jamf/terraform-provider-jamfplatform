// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platformenvironmentaction

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

func TestSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(account.Privileges, assignTenantsSDKMethods...); len(missing) > 0 {
		t.Fatalf("assignTenantsSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
}

// TestSDKMethods_MatchTheCalls keeps the privileges table honest as Invoke
// changes.
func TestSDKMethods_MatchTheCalls(t *testing.T) {
	src, err := os.ReadFile("assign_tenants.go")
	if err != nil {
		t.Fatal(err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := account.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	var got []string
	for m := range called {
		got = append(got, m)
	}
	sort.Strings(got)
	want := append([]string(nil), assignTenantsSDKMethods...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("assign_tenants.go calls %v, declared %v", got, want)
	}
}

func TestPrivileges_RenderBothActionsAndNeverClaimNoPermissionNeeded(t *testing.T) {
	for _, scoped := range []string{"platform-environments:update", "platform-environments:read"} {
		if !permissions.Renders(assignTenantsPrivileges, scoped) {
			t.Errorf("the privileges section did not render %s:\n%s", scoped, assignTenantsPrivileges)
		}
	}
	if strings.Contains(assignTenantsPrivileges, "None — any authenticated") {
		t.Errorf("the privileges section claims no permission is needed:\n%s", assignTenantsPrivileges)
	}
}
