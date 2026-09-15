// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE extracts SDK client method calls of the form `client.<Method>(`
// regardless of the receiver prefix (`r.client.`, `d.client.`).
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the set of SDK methods a construct source file calls,
// restricted to names known to the SDK privilege registry so unrelated
// `client.<X>` helper calls don't pollute the comparison.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMethodSetsEqual fails if the declared method list and the set of SDK
// calls discovered in the source file diverge.
func assertMethodSetsEqual(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := calledSDKMethods(t, filename)
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
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

// --- resource (crud.go) ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps resourceSDKMethods in sync with
// the actual client.<Method> calls in crud.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodSetsEqual(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "advanced-user-searches:create") {
		t.Fatalf("resourcePrivileges did not render the advanced-user-searches privileges:\n%s", resourcePrivileges)
	}
}

// --- data source (data_source.go) ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps dataSourceSDKMethods in sync
// with the actual client.<Method> calls in data_source.go.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMethodSetsEqual(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "advanced-user-searches:read") {
		t.Fatalf("dataSourcePrivileges did not render the advanced-user-searches privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource (list_resource.go) ---

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps listResourceSDKMethods in sync
// with the actual client.<Method> calls in list_resource.go.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMethodSetsEqual(t, "list_resource.go", listResourceSDKMethods)
}

// TestListResourcePrivileges_Rendered guards that the table actually rendered
// into the list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "advanced-user-searches:read") {
		t.Fatalf("listResourcePrivileges did not render the advanced-user-searches privileges:\n%s", listResourcePrivileges)
	}
}
