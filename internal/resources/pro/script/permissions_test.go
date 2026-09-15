// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches calls on the SDK client value, e.g. r.client.GetScriptV1(
// or d.client.ListScriptsV1( — capturing the SDK method name.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// assertCallsMatch reads a construct source file and asserts the set of SDK
// client method calls it makes equals the declared method list, keeping the
// privileges table honest as the construct's calls change.
func assertCallsMatch(t *testing.T, filename string, declaredMethods []string) {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		called[m[1]] = true
	}
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
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
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

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertCallsMatch(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK fails on SDK registry drift for the
// singular data source.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go's SDK calls drift
// from dataSourceSDKMethods.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestPluralDataSourceSDKMethods_KnownToSDK fails on SDK registry drift for the
// plural data source.
func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPluralDataSourceSDKMethods_MatchCalls fails if datasource_plural.go's SDK
// calls drift from pluralDataSourceSDKMethods.
func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

// TestListResourceSDKMethods_KnownToSDK fails on SDK registry drift for the
// list resource.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go's SDK calls
// drift from listResourceSDKMethods.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertCallsMatch(t, "list_resource.go", listResourceSDKMethods)
}

// TestPrivileges_Rendered guards that each construct's table rendered the scripts
// row *and* the action that construct actually performs. Asserting the action —
// not just the capability — is what makes this a drift guard: a row that ticked
// the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "scripts:create"},
		{"dataSourcePrivileges", dataSourcePrivileges, "scripts:read"},
		{"pluralDataSourcePrivileges", pluralDataSourcePrivileges, "scripts:read"},
		{"listResourcePrivileges", listResourcePrivileges, "scripts:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}
