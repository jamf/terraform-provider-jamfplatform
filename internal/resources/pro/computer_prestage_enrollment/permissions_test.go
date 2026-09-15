// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form client.<Method>(.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// registryCalls reads the named source file and returns the set of
// client.<Method> calls it makes that are present in the SDK privilege
// registry. Synthetic Resolve<X>ByName helpers (used by the data source's
// name-lookup path) are not registry entries by design, so filtering against
// the registry keeps the assertion focused on the privilege-bearing methods a
// construct actually documents.
func registryCalls(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the called set and the declared set differ.
func assertMatch(t *testing.T, filename string, called map[string]bool, declared []string) {
	t.Helper()
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

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", registryCalls(t, "crud.go"), resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "prestage-enrollments:create") {
		t.Fatalf("resourcePrivileges did not render the prestage privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method is
// absent from the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go's registry-known
// SDK calls drift from dataSourceSDKMethods.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", registryCalls(t, "data_source.go"), dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table rendered into the
// data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "prestage-enrollments:read") {
		t.Fatalf("dataSourcePrivileges did not render the prestage privileges:\n%s", dataSourcePrivileges)
	}
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method is absent from the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go's
// registry-known SDK calls drift from listResourceSDKMethods.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", registryCalls(t, "list_resource.go"), listResourceSDKMethods)
}

// TestListResourcePrivileges_Rendered guards that the table rendered into the
// list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "prestage-enrollments:read") {
		t.Fatalf("listResourcePrivileges did not render the prestage privileges:\n%s", listResourcePrivileges)
	}
}
