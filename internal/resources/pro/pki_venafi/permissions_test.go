// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_venafi

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls on the resource/data-source
// client field (r.client.X(, d.client.X() — the receiver varies but all land
// on a value named "client").
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK method names invoked on the client in
// the named source file, restricted to those present in the SDK registry so
// helper-call noise (e.g. helpers.X) never leaks in.
func calledMethods(t *testing.T, filename string) map[string]bool {
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

// diffSets compares the methods a source file calls against a declared list,
// reporting calls missing from the declaration and declarations not called.
func diffSets(called map[string]bool, declared []string) (undeclared, uncalled []string) {
	declSet := map[string]bool{}
	for _, m := range declared {
		declSet[m] = true
	}
	for m := range called {
		if !declSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declSet {
		if !called[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)
	return undeclared, uncalled
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call —
// keeping the privileges table honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	called := calledMethods(t, "crud.go")
	undeclared, uncalled := diffSets(called, resourceSDKMethods)
	if len(undeclared) > 0 {
		t.Errorf("crud.go calls SDK methods missing from resourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("resourceSDKMethods declares methods crud.go does not call: %v", uncalled)
	}
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description (catches an empty/parse-skipped registry).
func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{"pki:read", "pki:update"} {
		if !permissions.Renders(resourcePrivileges, want) {
			t.Fatalf("resourcePrivileges did not render %q:\n%s", want, resourcePrivileges)
		}
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
	called := calledMethods(t, "data_source.go")
	undeclared, uncalled := diffSets(called, dataSourceSDKMethods)
	if len(undeclared) > 0 {
		t.Errorf("data_source.go calls SDK methods missing from dataSourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("dataSourceSDKMethods declares methods data_source.go does not call: %v", uncalled)
	}
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "pki:read") {
		t.Fatalf("dataSourcePrivileges did not render the PKI read privilege:\n%s", dataSourcePrivileges)
	}
}
