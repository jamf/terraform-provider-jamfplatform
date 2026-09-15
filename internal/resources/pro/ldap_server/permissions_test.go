// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkCallRe matches a method call on any receiver (client.X(, c.X(, r.client.X(,
// d.client.X() and is filtered against the proclassic privilege registry so only
// genuine SDK methods survive. Matching any receiver and then intersecting with
// the registry keeps the guard robust across the resource (r.client), data
// source (d.client), and list resource (r.client) call sites.
var sdkCallRe = regexp.MustCompile(`\b\w+\.([A-Za-z0-9]+)\(`)

// sdkCallsInFile returns the set of SDK method names (present in
// proclassic.Privileges) called in the named source file.
func sdkCallsInFile(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range sdkCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the source file's set of SDK client method calls differs
// from the declared method list, keeping the privileges table honest as a
// construct's call path changes.
func assertMatch(t *testing.T, declaredMethods []string, called map[string]bool) {
	t.Helper()
	declared := map[string]bool{}
	for _, m := range declaredMethods {
		declared[m] = true
	}
	var undeclared, uncalled []string
	for m := range called {
		if !declared[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declared {
		if !called[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)
	if len(undeclared) > 0 {
		t.Errorf("source calls SDK methods missing from declared list: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list contains methods the source does not call: %v", uncalled)
	}
}

// --- Resource ---------------------------------------------------------------

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
	assertMatch(t, resourceSDKMethods, sdkCallsInFile(t, "crud.go"))
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "ldap-servers:create") {
		t.Fatalf("resourcePrivileges did not render the ldap-servers privileges:\n%s", resourcePrivileges)
	}
}

// --- Data source ------------------------------------------------------------

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, dataSourceSDKMethods, sdkCallsInFile(t, "data_source.go"))
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "ldap-servers:read") {
		t.Fatalf("dataSourcePrivileges did not render the ldap-servers privileges:\n%s", dataSourcePrivileges)
	}
}

// --- List resource ----------------------------------------------------------

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go calls an SDK
// method not declared in listResourceSDKMethods, or declares one it does not
// call.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, listResourceSDKMethods, sdkCallsInFile(t, "list_resource.go"))
}

// TestListResourcePrivileges_Rendered guards that the table actually rendered
// into the list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "ldap-servers:read") {
		t.Fatalf("listResourcePrivileges did not render the ldap-servers privileges:\n%s", listResourcePrivileges)
	}
}
