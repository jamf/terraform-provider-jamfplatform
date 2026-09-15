// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form `<recv>.client.Method(`,
// which covers both the resource (`r.client.X(`) and the data sources (`d.client.X(`).
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods extracts the distinct SDK client method names invoked in the
// named source file, filtered to those present in the SDK privilege registry so
// unrelated helper-method calls do not leak into the comparison.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
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

// assertMethodsMatch fails if the source file calls an SDK method not declared in
// the construct's SDK method list, or declares one it does not call.
func assertMethodsMatch(t *testing.T, filename string, declaredList []string) {
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

// --- resource (crud.go) ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodsMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "onboarding:update") {
		t.Fatalf("resourcePrivileges did not render the onboarding privileges:\n%s", resourcePrivileges)
	}
}

// --- data source (data_source.go) ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source privileges table
// honest as its Read path changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMethodsMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "onboarding:read") {
		t.Fatalf("dataSourcePrivileges did not render the onboarding privileges:\n%s", dataSourcePrivileges)
	}
}

// --- plural / eligible-items data source (data_source_eligible_items.go) ---

// TestPluralDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPluralDataSourceSDKMethods_MatchReadCalls keeps the eligible-items data
// source privileges table honest as its Read path changes.
func TestPluralDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMethodsMatch(t, "data_source_eligible_items.go", pluralDataSourceSDKMethods)
}

// TestPluralDataSourcePrivileges_Rendered guards that the table actually rendered
// into the eligible-items data source description.
func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "onboarding:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the onboarding privileges:\n%s", pluralDataSourcePrivileges)
	}
}
