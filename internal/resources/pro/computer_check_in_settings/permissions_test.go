// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_check_in_settings

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK client method calls of the form `r.client.Method(`
// or `d.client.Method(` (the `\bclient\.` anchor catches both receivers).
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names invoked in the
// named source file, filtered to those present in the pro.Privileges registry
// so unrelated helper calls do not pollute the comparison.
func calledMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the source file's SDK calls and the declared method list
// disagree in either direction.
func assertMatch(t *testing.T, filename string, declaredList []string) {
	t.Helper()
	called := calledMethods(t, filename)
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
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source privileges
// table honest as its Read path changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description (catches an empty/parse-skipped registry).
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "computer-check-in:update") {
		t.Fatalf("resourcePrivileges did not render the check-in privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "computer-check-in:read") {
		t.Fatalf("dataSourcePrivileges did not render the check-in privileges:\n%s", dataSourcePrivileges)
	}
}
