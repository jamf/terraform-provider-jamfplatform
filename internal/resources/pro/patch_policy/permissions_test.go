// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// mergedRegistry is the union of the SDK families this package spans: ProClassic
// CRUD plus the Pro v2 enumeration the list resource runs on.
var mergedRegistry = permissions.Merge(proclassic.Privileges, pro.Privileges)

// callsInFile returns the distinct SDK client method names called in the named
// source file that are present in the merged privilege registry. The walk lives
// in permissions.SDKCallsInFile so that comments and string literals cannot
// forge a call — see its doc comment for the byte-scanning regex it replaces and
// why the other permissions_test.go copies still carry one. Only the fatal-on-
// error behaviour stays here, where the *testing.T is.
func callsInFile(t *testing.T, filename string) map[string]bool {
	t.Helper()
	called, err := permissions.SDKCallsInFile(filename, mergedRegistry)
	if err != nil {
		t.Fatalf("scanning SDK calls: %v", err)
	}
	return called
}

// assertMethodsMatch fails if the file calls an SDK method not declared in
// methods, or declares one it does not call.
func assertMethodsMatch(t *testing.T, filename string, methods []string) {
	t.Helper()
	called := callsInFile(t, filename)
	declared := map[string]bool{}
	for _, m := range methods {
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
		t.Errorf("%s calls SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

// --- resource (crud.go) ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(mergedRegistry, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in the merged privilege registry (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodsMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "patch-policies:create") {
		t.Fatalf("resourcePrivileges did not render the patch-policies privileges:\n%s", resourcePrivileges)
	}
}

// --- data source (data_source.go) ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(mergedRegistry, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in the merged privilege registry (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMethodsMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "patch-policies:read") {
		t.Fatalf("dataSourcePrivileges did not render the patch-policies privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource (list_resource.go) ---

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(mergedRegistry, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in the merged privilege registry (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go calls an SDK
// method not declared in listResourceSDKMethods, or declares one it does not
// call.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMethodsMatch(t, "list_resource.go", listResourceSDKMethods)
}

// TestListResourcePrivileges_Rendered guards that the table actually rendered
// into the list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "patch-policies:read") {
		t.Fatalf("listResourcePrivileges did not render the patch-policies privileges:\n%s", listResourcePrivileges)
	}
}
