// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls (e.g. r.client.GetEbookByID(,
// d.client.GetEbookByName(). The receiver varies by construct but the
// `client.<Method>(` tail is stable.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsIn returns the set of SDK client method calls in the named source
// file, filtered to those present in the proclassic privilege registry so
// helper/local calls do not leak in.
func sdkCallsIn(t *testing.T, filename string) map[string]bool {
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

// assertMatch fails if the declared method list and the SDK calls discovered in
// the source file diverge in either direction.
func assertMatch(t *testing.T, filename string, declared []string, called map[string]bool) {
	t.Helper()
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

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods, sdkCallsIn(t, "crud.go"))
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method has
// been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go and the declared
// dataSourceSDKMethods diverge.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods, sdkCallsIn(t, "data_source.go"))
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource method
// has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go and the
// declared listResourceSDKMethods diverge.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods, sdkCallsIn(t, "list_resource.go"))
}

// TestPrivileges_Rendered guards that the tables actually rendered into each
// construct description (catches an empty/parse-skipped registry).
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		want     string
	}{
		{"resource", resourcePrivileges, "ebooks:create"},
		{"dataSource", dataSourcePrivileges, "ebooks:read"},
		{"listResource", listResourcePrivileges, "ebooks:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.want) {
			t.Errorf("%s privileges did not render %q:\n%s", tc.name, tc.want, tc.rendered)
		}
	}
}
