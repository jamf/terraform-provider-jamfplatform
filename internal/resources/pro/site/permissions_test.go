// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK method calls made on a `*.client` value, e.g.
// `r.client.CreateSiteByID(` or `d.client.GetSiteByName(`.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names called in the
// named source file, filtered to those present in the registry so unrelated
// helper calls (e.g. context.WithTimeout) never leak in.
func calledMethods(t *testing.T, filename string, known []string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	knownSet := map[string]bool{}
	for _, m := range known {
		knownSet[m] = true
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if knownSet[m[1]] {
			called[m[1]] = true
		}
	}
	out := make([]string, 0, len(called))
	for m := range called {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// assertMatch fails when the declared method list and the set of registry
// methods actually called in the source file diverge in either direction.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := calledMethods(t, filename, declared)
	want := append([]string(nil), declared...)
	sort.Strings(want)

	declaredSet := map[string]bool{}
	for _, m := range want {
		declaredSet[m] = true
	}
	calledSet := map[string]bool{}
	for _, m := range called {
		calledSet[m] = true
	}

	var uncalled []string
	for m := range declaredSet {
		if !calledSet[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(uncalled)
	if len(uncalled) > 0 {
		t.Errorf("%s: declared methods not called: %v", filename, uncalled)
	}
}

// --- resource (crud.go) ---

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	src, err := os.ReadFile("crud.go")
	if err != nil {
		t.Fatalf("reading crud.go: %v", err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		called[m[1]] = true
	}
	declared := map[string]bool{}
	for _, m := range resourceSDKMethods {
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
		t.Errorf("crud.go calls SDK methods missing from resourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("resourceSDKMethods declares methods crud.go does not call: %v", uncalled)
	}
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "sites:create") {
		t.Fatalf("resourcePrivileges did not render the sites privileges:\n%s", resourcePrivileges)
	}
}

// --- singular data source (data_source.go) ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "sites:read") {
		t.Fatalf("dataSourcePrivileges did not render the sites privileges:\n%s", dataSourcePrivileges)
	}
}

// --- plural data source (datasource_plural.go) ---

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "sites:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the sites privileges:\n%s", pluralDataSourcePrivileges)
	}
}

// --- list resource (list_resource.go) ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "sites:read") {
		t.Fatalf("listResourcePrivileges did not render the sites privileges:\n%s", listResourcePrivileges)
	}
}
