// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_title

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE extracts the SDK method name from a `client.<Method>(` call,
// which also matches the `d.client.<Method>(` receiver used by these data
// sources.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names invoked in a construct
// source file, filtered to those present in the pro privilege registry so
// helper/local calls don't leak into the comparison.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
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

// compareMethodSets fails if the called set and the declared set differ.
func compareMethodSets(t *testing.T, filename string, declaredList []string, called map[string]bool) {
	t.Helper()
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
	called := calledSDKMethods(t, "data_source.go")
	compareMethodSets(t, "data_source.go", dataSourceSDKMethods, called)
}

// TestPluralDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPluralDataSourceSDKMethods_MatchReadCalls fails if datasource_plural.go
// calls an SDK method not declared in pluralDataSourceSDKMethods, or declares
// one it does not call.
func TestPluralDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	called := calledSDKMethods(t, "datasource_plural.go")
	compareMethodSets(t, "datasource_plural.go", pluralDataSourceSDKMethods, called)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source descriptions (catches an empty/parse-skipped registry).
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "applications:read") {
		t.Fatalf("dataSourcePrivileges did not render the mac-applications privileges:\n%s", dataSourcePrivileges)
	}
	if !permissions.Renders(pluralDataSourcePrivileges, "applications:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the mac-applications privileges:\n%s", pluralDataSourcePrivileges)
	}
}
