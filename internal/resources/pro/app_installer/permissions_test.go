// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches an SDK client method call on the resource/data-source
// receiver (r.client.Foo( / d.client.Foo( / client.Foo(); the trailing "(" and
// the \b anchor on "client" keep it to genuine call sites.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// helperCallRE matches a bare call to a package-local function, whose name is
// then looked up in helperSDKMethods.
var helperCallRE = regexp.MustCompile(`\b([a-z][A-Za-z0-9]*)\(`)

// helperSDKMethods maps a package-local helper the construct entry files call to
// the SDK methods it reaches. The name resolvers take a narrow interface
// receiver (titleCatalog / deploymentLister) rather than the client, so
// clientCallRE cannot see those requests at the point they are issued — and the
// title resolvers now go one step further, answering from a provider-instance
// snapshot whose read is registered at Configure. Declaring the reach here keeps
// each privilege list honest without sweeping name_lookup.go whole, which would
// over-report: that one file serves both the resource (the title catalog) and the
// data source (the deployment list).
var helperSDKMethods = map[string][]string{
	"resolveAppTitleID":         {"ListAppInstallerTitlesV1"},
	"validateAppTitleName":      {"ListAppInstallerTitlesV1"},
	"titleNameForID":            {"ListAppInstallerTitlesV1"},
	"resolveDeploymentIDByName": {"ListAppInstallerDeploymentsV1"},
}

// callsIn returns the distinct SDK method names the named source file reaches —
// direct client.<Method> calls plus the methods helperSDKMethods records for
// each package-local helper it calls — filtered to those present in
// pro.Privileges. Filtering to the registry drops anything the SDK does not
// price, so a helper name that happens to match a method name cannot inflate
// the result.
func callsIn(t *testing.T, filename string) map[string]bool {
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
	for _, m := range helperCallRE.FindAllStringSubmatch(string(src), -1) {
		for _, method := range helperSDKMethods[m[1]] {
			if _, ok := pro.Privileges[method]; ok {
				called[method] = true
			}
		}
	}
	return called
}

// assertMatch fails when the registry-known SDK calls reachable from filename do
// not exactly equal declared.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := callsIn(t, filename)
	want := map[string]bool{}
	for _, m := range declared {
		want[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !want[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range want {
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

// --- Helper indirection map ---

// TestHelperSDKMethods_HelpersExist fails when helperSDKMethods names a helper
// the package no longer declares, which would silently stop contributing its SDK
// reach and let a privilege list drift out of sync unnoticed.
func TestHelperSDKMethods_HelpersExist(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}
	var pkg strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		pkg.WriteString(string(src))
	}
	for helper := range helperSDKMethods {
		if !strings.Contains(pkg.String(), "func "+helper+"(") {
			t.Errorf("helperSDKMethods names %q, which the package does not declare", helper)
		}
	}
}

// TestHelperSDKMethods_KnownToSDK fails when a helper's declared reach names a
// method the SDK privilege registry does not price.
func TestHelperSDKMethods_KnownToSDK(t *testing.T) {
	for helper, methods := range helperSDKMethods {
		if missing := permissions.Missing(pro.Privileges, methods...); len(missing) > 0 {
			t.Errorf("helperSDKMethods[%q] names methods absent from pro.Privileges (SDK drift): %v", helper, missing)
		}
	}
}

// --- Resource (crud.go) ---

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "applications:create") {
		t.Fatalf("resourcePrivileges did not render the App Installer privileges:\n%s", resourcePrivileges)
	}
}

// --- Singular data source (data_source.go) ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "applications:read") {
		t.Fatalf("dataSourcePrivileges did not render the App Installer privileges:\n%s", dataSourcePrivileges)
	}
}

// --- Plural data source (datasource_plural.go) ---

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "applications:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the App Installer privileges:\n%s", pluralDataSourcePrivileges)
	}
}

// --- List resource (list_resource.go) ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "applications:read") {
		t.Fatalf("listResourcePrivileges did not render the App Installer privileges:\n%s", listResourcePrivileges)
	}
}
