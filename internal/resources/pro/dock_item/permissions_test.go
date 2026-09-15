// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form
// client.Method( / r.client.Method( / d.client.Method(. The leading "client"
// receiver field is shared across the resource, data source, and list resource.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK methods a construct's source file
// invokes that are present in the proclassic privilege registry. Filtering to
// the registry keeps the assertion robust against unrelated helper calls.
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

// assertMatch fails if declared != called, naming the drift in either direction.
func assertMatch(t *testing.T, filename string, declared []string, called map[string]bool) {
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
		t.Errorf("%s calls SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
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
	assertMatch(t, "crud.go", resourceSDKMethods, calledSDKMethods(t, "crud.go"))
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods, calledSDKMethods(t, "data_source.go"))
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls fails if list_resource.go calls an
// SDK method not declared in listResourceSDKMethods, or declares one it does
// not call.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods, calledSDKMethods(t, "list_resource.go"))
}

// TestPrivileges_Rendered guards that each construct's table rendered the dock-items
// row *and* the action that construct actually performs. Asserting the action —
// not just the capability — is what makes this a drift guard: a row that ticked
// the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "dock-items:create"},
		{"dataSourcePrivileges", dataSourcePrivileges, "dock-items:read"},
		{"listResourcePrivileges", listResourcePrivileges, "dock-items:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}
