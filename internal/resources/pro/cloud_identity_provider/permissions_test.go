// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK client method calls of the form `client.<Method>(`.
// Both the resource (r.client) and the data sources / list resource (d.client /
// r.client) reduce to a `client.<Method>(` substring, so a single pattern
// covers every construct in this package.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsIn returns the set of SDK client method calls made across the given
// source files, restricted to names present in the pro privilege registry.
// Restricting to known methods keeps the comparison robust against incidental
// `client.<something>(` hits that are not SDK endpoints.
func sdkCallsIn(t *testing.T, files ...string) map[string]bool {
	t.Helper()
	called := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			if _, ok := pro.Privileges[name]; ok {
				called[name] = true
			}
		}
	}
	return called
}

// assertCallsMatch fails if the SDK calls found in the source files differ from
// the declared method list — keeping the privileges table honest as the code
// path changes.
func assertCallsMatch(t *testing.T, declared []string, files ...string) {
	t.Helper()
	called := sdkCallsIn(t, files...)
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
		t.Errorf("%v call SDK methods missing from declared list: %v", files, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %v does not call: %v", files, uncalled)
	}
}

// --- resource -----------------------------------------------------------

// TestResourceSDKMethods_KnownToSDK fails if a declared resource method has
// been renamed or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if the resource's CRUD and
// plan-time paths call an SDK method not declared in resourceSDKMethods, or
// declare one they do not call. The CRUD path is split across crud.go,
// crud_azure.go, crud_google.go, plus the keystore verify in plan_modifiers.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertCallsMatch(t, resourceSDKMethods, "crud.go", "crud_azure.go", "crud_google.go", "plan_modifiers.go")
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description (catches an empty/parse-skipped registry).
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "ldap-servers:create") {
		t.Fatalf("resourcePrivileges did not render the ldap-servers privileges:\n%s", resourcePrivileges)
	}
}

// --- singular data source ----------------------------------------------

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, dataSourceSDKMethods, "data_source.go")
}

// TestDataSourcePrivileges_Rendered guards that the table rendered.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "ldap-servers:read") {
		t.Fatalf("dataSourcePrivileges did not render the ldap-servers privileges:\n%s", dataSourcePrivileges)
	}
}

// --- defaults data source ----------------------------------------------

// TestDefaultsDataSourceSDKMethods_KnownToSDK fails if a declared defaults data
// source method has been renamed or removed in the SDK privilege registry.
func TestDefaultsDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, defaultsDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("defaultsDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDefaultsDataSourceSDKMethods_MatchCalls fails if data_source_defaults.go
// calls an SDK method not declared in defaultsDataSourceSDKMethods, or declares
// one it does not call. It is also what would catch a return to the deprecated
// Entra ID mappings endpoint, which the data source deliberately does not call.
func TestDefaultsDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, defaultsDataSourceSDKMethods, "data_source_defaults.go")
}

// TestDefaultsDataSourcePrivileges_Rendered guards that the table rendered.
func TestDefaultsDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(defaultsDataSourcePrivileges, "ldap-servers:read") {
		t.Fatalf("defaultsDataSourcePrivileges did not render the ldap-servers privileges:\n%s", defaultsDataSourcePrivileges)
	}
}

// --- plural data source -------------------------------------------------

// TestPluralDataSourceSDKMethods_KnownToSDK fails if a declared plural data
// source method has been renamed or removed in the SDK privilege registry.
func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPluralDataSourceSDKMethods_MatchCalls fails if data_source_plural.go
// calls an SDK method not declared in pluralDataSourceSDKMethods, or declares
// one it does not call.
func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, pluralDataSourceSDKMethods, "data_source_plural.go")
}

// TestPluralDataSourcePrivileges_Rendered guards that the table rendered.
func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "ldap-servers:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the ldap-servers privileges:\n%s", pluralDataSourcePrivileges)
	}
}

// --- list resource ------------------------------------------------------

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go calls an SDK
// method not declared in listResourceSDKMethods, or declares one it does not call.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, listResourceSDKMethods, "list_resource.go")
}

// TestListResourcePrivileges_Rendered guards that the table rendered.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "ldap-servers:read") {
		t.Fatalf("listResourcePrivileges did not render the ldap-servers privileges:\n%s", listResourcePrivileges)
	}
}
