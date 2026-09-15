// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe captures SDK method names invoked on the SDK client value
// (r.client.<Method>( / d.client.<Method>(). Used by the per-construct
// match tests below.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsIn reads a construct source file and returns the distinct SDK client
// method names it calls, filtered to those known to the proclassic privilege
// registry (so unrelated client.* helpers can't leak in).
func sdkCallsIn(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	seen := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// assertMatch fails if the called set and declared set diverge.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := sdkCallsIn(t, filename)

	declaredSet := map[string]bool{}
	for _, m := range declared {
		declaredSet[m] = true
	}
	calledSet := map[string]bool{}
	for _, m := range called {
		calledSet[m] = true
	}

	var undeclared, uncalled []string
	for m := range calledSet {
		if !declaredSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declaredSet {
		if !calledSet[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("%s calls SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

// --- resource (crud.go) ---

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// --- data source (data_source.go) ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// --- list resource (list_resource.go) ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

// --- rendered-table guards ---

// TestPrivileges_Rendered guards that each construct's table rendered the applications
// row *and* the action that construct actually performs. Asserting the action —
// not just the capability — is what makes this a drift guard: a row that ticked
// the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "applications:create"},
		{"dataSourcePrivileges", dataSourcePrivileges, "applications:read"},
		{"listResourcePrivileges", listResourcePrivileges, "applications:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}
