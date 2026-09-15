// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls regardless of the receiver name
// (r.client. for resources/list resources, d.client. for data sources).
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the set of SDK method names called against the
// client value in the named source file, filtered to those known to the
// proclassic privilege registry. Filtering keeps unrelated client.<X>( calls
// (e.g. helper methods absent from the registry) from polluting the match set.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the source file calls an SDK method not declared in
// methods, or declares one it does not call — keeping the privileges table
// honest as the construct changes.
func assertMatch(t *testing.T, filename string, methods []string) {
	t.Helper()
	called := calledSDKMethods(t, filename)
	declared := map[string]bool{}
	for _, m := range methods {
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

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "network-segments:create") {
		t.Fatalf("resourcePrivileges did not render the network-segments privileges:\n%s", resourcePrivileges)
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
	if !permissions.Renders(dataSourcePrivileges, "network-segments:read") {
		t.Fatalf("dataSourcePrivileges did not render the network-segments privileges:\n%s", dataSourcePrivileges)
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
	if !permissions.Renders(pluralDataSourcePrivileges, "network-segments:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the network-segments privileges:\n%s", pluralDataSourcePrivileges)
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
	if !permissions.Renders(listResourcePrivileges, "network-segments:read") {
		t.Fatalf("listResourcePrivileges did not render the network-segments privileges:\n%s", listResourcePrivileges)
	}
}
