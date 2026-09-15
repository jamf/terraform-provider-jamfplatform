// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_macos

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK method calls on a client value (e.g.
// r.client.GetX( or d.client.ListX().
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the set of SDK client method names called in the named
// source file, filtered to those known to the registry so unrelated helper
// calls do not leak in.
func calledMethods(t *testing.T, filename string, known map[string]bool) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if known[m[1]] {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the called set and the declared set differ.
func assertMatch(t *testing.T, called map[string]bool, declared []string, filename string, declName string) {
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
		t.Errorf("%s calls SDK methods missing from %s: %v", filename, declName, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("%s declares methods %s does not call: %v", declName, filename, uncalled)
	}
}

// knownRegistry builds the set of method names present in pro.Privileges so the
// call-extraction can ignore non-SDK helper calls that happen to match the
// client.<Method>( shape.
func knownRegistry() map[string]bool {
	known := map[string]bool{}
	for name := range pro.Privileges {
		known[name] = true
	}
	return known
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
	known := knownRegistry()
	called := calledMethods(t, "crud.go", known)
	assertMatch(t, called, resourceSDKMethods, "crud.go", "resourceSDKMethods")
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "self-service:create") {
		t.Fatalf("resourcePrivileges did not render the branding privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	known := knownRegistry()
	called := calledMethods(t, "data_source.go", known)
	assertMatch(t, called, dataSourceSDKMethods, "data_source.go", "dataSourceSDKMethods")
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "self-service:read") {
		t.Fatalf("dataSourcePrivileges did not render the branding privileges:\n%s", dataSourcePrivileges)
	}
}
