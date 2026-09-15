// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches an SDK client method invocation in a construct source
// file. It matches both the resource/list-resource receiver form
// (r.client.Method(), client.Method()) and the data source form
// (d.client.Method()) because the \bclient\. anchor falls on a word boundary
// after the receiver's dot.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsKnownToRegistry reads a construct source file and returns the
// distinct SDK client method calls it makes that resolve to a privilege entry
// in the SDK registry. Calls to SDK helpers without their own privilege entry
// (e.g. the ResolvePackageV1ByName resolver, which layers over the
// GET /v1/packages list endpoint) are filtered out so the comparison stays
// honest against the declared method lists.
func sdkCallsKnownToRegistry(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if _, ok := pro.Privileges[name]; !ok {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// assertMethodSetMatches fails when the registry-known SDK calls in filename do
// not exactly equal the declared method list.
func assertMethodSetMatches(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := map[string]bool{}
	for _, m := range sdkCallsKnownToRegistry(t, filename) {
		called[m] = true
	}
	want := map[string]bool{}
	for _, m := range declared {
		want[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !want[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range want {
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

// --- resource ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps resourceSDKMethods in sync with
// the SDK calls in crud.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodSetMatches(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "packages:create") {
		t.Fatalf("resourcePrivileges did not render the packages privileges:\n%s", resourcePrivileges)
	}
}

// --- data source ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps dataSourceSDKMethods in sync
// with the registry-known SDK calls in data_source.go.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMethodSetMatches(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered
// into the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "packages:read") {
		t.Fatalf("dataSourcePrivileges did not render the packages privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource ---

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps listResourceSDKMethods in
// sync with the registry-known SDK calls in list_resource.go.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMethodSetMatches(t, "list_resource.go", listResourceSDKMethods)
}

// TestListResourcePrivileges_Rendered guards that the table actually rendered
// into the list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "packages:read") {
		t.Fatalf("listResourcePrivileges did not render the packages privileges:\n%s", listResourcePrivileges)
	}
}
