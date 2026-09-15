// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK client method calls of the form `*.client.<Method>(`
// (e.g. `d.client.GetUserV1(`). The leading `\b` falls on the boundary between
// the receiver's `.` and `client`, so this matches `d.client.X(` as well as a
// bare `client.X(`.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names invoked via the client
// value in the named source file, restricted to those known to the registry.
func calledSDKMethods(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if _, ok := pro.Privileges[name]; ok {
			called[name] = true
		}
	}
	out := make([]string, 0, len(called))
	for m := range called {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// assertMatch fails if the set of SDK client method calls in filename differs
// from declared.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	declaredSet := map[string]bool{}
	for _, m := range declared {
		declaredSet[m] = true
	}
	calledSet := map[string]bool{}
	for _, m := range calledSDKMethods(t, filename) {
		calledSet[m] = true
	}

	var undeclared, uncalled []string
	for m := range calledSet {
		if !declaredSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declaredSet {
		if !calledSet[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list contains methods %s does not call: %v", filename, uncalled)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps the privileges table honest as
// data_source.go changes.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestPluralDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPluralDataSourceSDKMethods_MatchCalls keeps the privileges table honest
// as datasource_plural.go changes.
func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

// TestPrivileges_Rendered guards that the tables actually rendered into the
// descriptions (catches an empty/parse-skipped registry).
func TestPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "users:read") {
		t.Fatalf("dataSourcePrivileges did not render the user privileges:\n%s", dataSourcePrivileges)
	}
	if !permissions.Renders(pluralDataSourcePrivileges, "users:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the user privileges:\n%s", pluralDataSourcePrivileges)
	}
}
