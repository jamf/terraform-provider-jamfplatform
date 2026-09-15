// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkClientCallRe matches an SDK client method call on any of this package's
// receiver fields: client (data source / list resource), proClient and
// classicClient (the hybrid resource). The capture group is the method name.
var sdkClientCallRe = regexp.MustCompile(`\b\w*[Cc]lient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods reads a source file and returns the set of SDK method names
// it calls on a client receiver, filtered to methods present in the given
// registry. Filtering to the registry drops synthetic resolver/apply wrappers
// (e.g. ResolveAccountV1ByName) that the SDK privilege registry does not carry,
// so the comparison reflects exactly the privilege-bearing calls a construct
// makes.
func calledSDKMethods(t *testing.T, filename string, reg permissions.Registry) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range sdkClientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := reg[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertDeclaredMatchesCalled fails if the source file calls a registry-known
// SDK method not declared, or declares one it does not call.
func assertDeclaredMatchesCalled(t *testing.T, filename string, reg permissions.Registry, declaredMethods []string) {
	t.Helper()
	called := calledSDKMethods(t, filename, reg)
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
		t.Errorf("%s calls SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list contains methods %s does not call: %v", filename, uncalled)
	}
}

// TestResourceSDKMethods_KnownToSDK fails if a declared resource method has
// been renamed or removed in the merged Pro+ProClassic privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(resourcePrivilegeRegistry, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in Pro+ProClassic Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privilege table
// honest as crud.go changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertDeclaredMatchesCalled(t, "crud.go", resourcePrivilegeRegistry, resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "accounts:create") {
		t.Fatalf("resourcePrivileges did not render the accounts privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the Pro privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source privilege table
// honest as data_source.go changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertDeclaredMatchesCalled(t, "data_source.go", pro.Privileges, dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered
// into the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "accounts:read") {
		t.Fatalf("dataSourcePrivileges did not render the accounts privileges:\n%s", dataSourcePrivileges)
	}
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource method
// has been renamed or removed in the merged Pro + ProClassic privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(resourcePrivilegeRegistry, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in the merged Pro + ProClassic registry (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps the list resource privilege
// table honest as list_resource.go changes.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertDeclaredMatchesCalled(t, "list_resource.go", resourcePrivilegeRegistry, listResourceSDKMethods)
}

// TestListResourcePrivileges_Rendered guards that the table actually rendered
// into the list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "accounts:read") {
		t.Fatalf("listResourcePrivileges did not render the accounts privileges:\n%s", listResourcePrivileges)
	}
}
