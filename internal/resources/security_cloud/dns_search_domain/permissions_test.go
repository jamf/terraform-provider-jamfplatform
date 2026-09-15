// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form
// <receiver>.client.<Method>( — covering the r.client and d.client receivers used
// by the resource and data source in this package.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names invoked in the given
// source file, restricted to methods known to the SDK privilege registry so
// unrelated identifiers are ignored.
func calledMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := securitycloud.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the called set and the declared set differ.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := calledMethods(t, filename)
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

// --- resource ---

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table honest
// as the CRUD path changes. It is what catches the read privilege that Create's
// clobber preflight quietly requires.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{"search-domains:read", "search-domains:update", "search-domains:delete"} {
		if !permissions.Renders(resourcePrivileges, want) {
			t.Errorf("resourcePrivileges did not render %q:\n%s", want, resourcePrivileges)
		}
	}
}

// --- data source ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "search-domains:read") {
		t.Fatalf("dataSourcePrivileges did not render the search domain privileges:\n%s", dataSourcePrivileges)
	}
}
