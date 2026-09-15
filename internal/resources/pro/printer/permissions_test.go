// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls (r.client.<Method>( or
// client.<Method>(). The receiver in this package is always "client" on the
// construct struct.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the set of SDK method names invoked in the named
// source file, restricted to those present in the proclassic registry so that
// unrelated helper calls do not pollute the comparison.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails when the declared method list and the file's actual SDK
// client calls diverge in either direction.
func assertMatch(t *testing.T, declared []string, called map[string]bool, filename string, varName string) {
	t.Helper()
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
		t.Errorf("%s calls SDK methods missing from %s: %v", filename, varName, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("%s declares methods %s does not call: %v", varName, filename, uncalled)
	}
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	called := calledSDKMethods(t, "crud.go")
	assertMatch(t, resourceSDKMethods, called, "crud.go", "resourceSDKMethods")
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	called := calledSDKMethods(t, "data_source.go")
	assertMatch(t, dataSourceSDKMethods, called, "data_source.go", "dataSourceSDKMethods")
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls fails if list_resource.go calls an
// SDK method not declared in listResourceSDKMethods, or declares one it does
// not call.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	called := calledSDKMethods(t, "list_resource.go")
	assertMatch(t, listResourceSDKMethods, called, "list_resource.go", "listResourceSDKMethods")
}

// TestPrivileges_Rendered guards that each construct's table actually rendered
// into its description (catches an empty/parse-skipped registry).
func TestPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "printers:create") {
		t.Fatalf("resourcePrivileges did not render the printers privileges:\n%s", resourcePrivileges)
	}
	if !permissions.Renders(dataSourcePrivileges, "printers:read") {
		t.Fatalf("dataSourcePrivileges did not render the printers privileges:\n%s", dataSourcePrivileges)
	}
	if !permissions.Renders(listResourcePrivileges, "printers:read") {
		t.Fatalf("listResourcePrivileges did not render the printers privileges:\n%s", listResourcePrivileges)
	}
}
