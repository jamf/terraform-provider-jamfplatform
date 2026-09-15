// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_inventory_collection_settings

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK client method calls of the form `<recv>.client.<Method>(`.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names invoked in the
// named source file, filtered to those present in the SDK privilege registry.
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

// assertMatch fails if the source file calls an SDK method not declared, or
// declares one it does not call — keeping the privileges table honest.
func assertMatch(t *testing.T, filename string, declaredMethods []string) {
	t.Helper()
	called := calledMethods(t, filename)
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
		t.Errorf("%s calls SDK methods missing from declared set: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared set lists methods %s does not call: %v", filename, uncalled)
	}
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
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description (catches an empty/parse-skipped registry). It names
// every capability-action pair the table renders: the custom file paths are a
// separate capability from the settings they hang off, and a row dropped from
// the table is invisible to a check that only looks for the row it kept.
func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{
		"computer-inventory-collection-settings:read",
		"computer-inventory-collection-settings:update",
		"custom-paths:create",
		"custom-paths:delete",
	} {
		if !permissions.Renders(resourcePrivileges, want) {
			t.Errorf("resourcePrivileges did not render %q:\n%s", want, resourcePrivileges)
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
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered
// into the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "computer-inventory-collection-settings:read") {
		t.Fatalf("dataSourcePrivileges did not render the expected privileges:\n%s", dataSourcePrivileges)
	}
}
