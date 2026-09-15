// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_volume_purchasing_content_search

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches calls on the SDK client value, e.g. client.GetFooV1( or
// r.client.GetFooV1( or d.client.ResolveFooV1ByName(.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// registryKnownCalls reads a construct source file and returns the set of SDK
// client method calls it makes that are present in the pro privilege registry.
// Filtering to registry-known names drops resolver wrappers (e.g.
// ResolveAdvancedUserContentSearchV1ByName) that are not themselves privileged
// endpoints.
func registryKnownCalls(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertCallsMatch fails if filename calls a registry-known SDK method not in
// declared, or declares one it does not call.
func assertCallsMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := registryKnownCalls(t, filename)
	declaredSet := map[string]bool{}
	for _, m := range declared {
		declaredSet[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !declaredSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declaredSet {
		if !called[m] {
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

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertCallsMatch(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source privileges
// table honest as its Read path changes. The by-name lookup also calls
// ResolveAdvancedUserContentSearchV1ByName, an SDK resolver wrapper absent from
// the registry; registryKnownCalls filters it out so the assertion compares
// only privileged endpoints.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertCallsMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps the list resource privileges
// table honest as its List path changes.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertCallsMatch(t, "list_resource.go", listResourceSDKMethods)
}

// TestPrivileges_Rendered guards that each table actually rendered into its
// description (catches an empty/parse-skipped registry).
func TestPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "advanced-user-searches:create") {
		t.Fatalf("resourcePrivileges did not render the expected privileges:\n%s", resourcePrivileges)
	}
	if !permissions.Renders(dataSourcePrivileges, "advanced-user-searches:read") {
		t.Fatalf("dataSourcePrivileges did not render the expected privileges:\n%s", dataSourcePrivileges)
	}
	if !permissions.Renders(listResourcePrivileges, "advanced-user-searches:read") {
		t.Fatalf("listResourcePrivileges did not render the expected privileges:\n%s", listResourcePrivileges)
	}
}
