// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls regardless of the receiver
// variable (r.client.X, d.client.X). The matched set is filtered down to
// registry-known methods before comparison so resolver convenience wrappers
// (e.g. ResolveInventoryPreloadRecordV2BySerialNumber, which is not a registry
// entry) and unrelated helper calls do not register as undeclared methods.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods reads the named source file and returns the set of SDK
// client method calls it makes that are present in the pro privilege registry.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if len(permissions.Missing(pro.Privileges, name)) == 0 {
			called[name] = true
		}
	}
	return called
}

// assertMethodSet compares the registry-known SDK calls in filename against the
// declared method list, failing on either direction of drift.
func assertMethodSet(t *testing.T, filename string, declaredList []string) {
	t.Helper()
	called := calledSDKMethods(t, filename)
	declared := map[string]bool{}
	for _, m := range declaredList {
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

// TestResourceSDKMethods_KnownToSDK fails if a declared resource method has
// been renamed or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps resourceSDKMethods in sync with
// the actual client.<Method> calls in crud.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodSet(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps dataSourceSDKMethods in sync
// with the registry-known client.<Method> calls in data_source.go.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMethodSet(t, "data_source.go", dataSourceSDKMethods)
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps listResourceSDKMethods in
// sync with the registry-known client.<Method> calls in list_resource.go.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMethodSet(t, "list_resource.go", listResourceSDKMethods)
}

// TestPrivileges_Rendered guards that each construct's table rendered the inventory-preload-records
// row *and* the action that construct actually performs. Asserting the action —
// not just the capability — is what makes this a drift guard: a row that ticked
// the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "inventory-preload-records:create"},
		{"dataSourcePrivileges", dataSourcePrivileges, "inventory-preload-records:read"},
		{"listResourcePrivileges", listResourcePrivileges, "inventory-preload-records:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}
