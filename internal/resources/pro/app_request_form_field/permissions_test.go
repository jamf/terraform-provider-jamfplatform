// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches client.<Method>( calls regardless of receiver
// (r.client / d.client). Used to extract the SDK methods each construct calls.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// knownClientCalls reads a construct source file and returns the set of
// client.<Method> calls it makes that are present in the SDK privilege
// registry. Resolver/apply wrappers (e.g. ResolveAppRequestFormInputFieldV1ByName)
// are not registry entries and are intentionally filtered out — their required
// privilege is already covered by the underlying registered method.
func knownClientCalls(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if _, ok := pro.Privileges[name]; ok {
			called[name] = true
		}
	}
	return called
}

// assertMatch fails if the construct's registry-known client calls differ from
// its declared SDK method set.
func assertMatch(t *testing.T, filename string, declaredMethods []string) {
	t.Helper()
	called := knownClientCalls(t, filename)
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
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
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

// TestResourceSDKMethods_MatchCRUDCalls keeps resourceSDKMethods in sync with the
// client.<Method> calls in crud.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table rendered into the
// resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "app-request:update") {
		t.Fatalf("resourcePrivileges did not render the app-request privileges:\n%s", resourcePrivileges)
	}
}

// --- data source ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method is absent from
// the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps dataSourceSDKMethods in sync with the
// registry-known client.<Method> calls in data_source.go.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table rendered into the data
// source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "app-request:read") {
		t.Fatalf("dataSourcePrivileges did not render the app-request privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource ---

// TestListResourceSDKMethods_KnownToSDK fails if a declared method is absent from
// the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls keeps listResourceSDKMethods in sync with
// the registry-known client.<Method> calls in list_resource.go.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

// TestListResourcePrivileges_Rendered guards that the table rendered into the list
// resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "app-request:read") {
		t.Fatalf("listResourcePrivileges did not render the app-request privileges:\n%s", listResourcePrivileges)
	}
}
