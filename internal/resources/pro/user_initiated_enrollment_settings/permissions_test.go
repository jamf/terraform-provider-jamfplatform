// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches r.client.<Method>( and d.client.<Method>( SDK calls.
var clientCallRE = regexp.MustCompile(`\b[a-z]\.client\.([A-Za-z0-9]+)\(`)

// collectClientCalls returns the distinct SDK method names called across the
// named source files, restricted to those present in the SDK registry. Doc
// comments that merely name a method (e.g. the "SDK endpoints used" header) do
// not match because they lack the receiver.<Method>( call shape.
func collectClientCalls(t *testing.T, files ...string) map[string]bool {
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

// diffSets compares the called set against the declared list and reports
// methods called-but-undeclared and declared-but-uncalled.
func diffSets(called map[string]bool, declared []string) (undeclared, uncalled []string) {
	decl := map[string]bool{}
	for _, m := range declared {
		decl[m] = true
	}
	for m := range called {
		if !decl[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range decl {
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

// TestResourceSDKMethods_MatchCRUDCalls fails if the resource's CRUD path calls
// an SDK method not declared in resourceSDKMethods, or declares one it does not
// call. The resource's calls span crud.go, access_groups.go, and
// messaging_languages.go, so all three are inspected.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	called := collectClientCalls(t, "crud.go", "access_groups.go", "messaging_languages.go")
	undeclared, uncalled := diffSets(called, resourceSDKMethods)
	if len(undeclared) > 0 {
		t.Errorf("resource CRUD path calls SDK methods missing from resourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("resourceSDKMethods declares methods the resource CRUD path does not call: %v", uncalled)
	}
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description (catches an empty/parse-skipped registry).
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "user-initiated-enrollment:update") {
		t.Fatalf("resourcePrivileges did not render the enrollment privileges:\n%s", resourcePrivileges)
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
	called := collectClientCalls(t, "data_source.go")
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
	if !permissions.Renders(dataSourcePrivileges, "user-initiated-enrollment:read") {
		t.Fatalf("dataSourcePrivileges did not render the enrollment privileges:\n%s", dataSourcePrivileges)
	}
}
