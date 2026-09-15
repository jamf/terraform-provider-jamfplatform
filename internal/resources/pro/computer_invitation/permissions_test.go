// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form `<recv>.client.<Method>(`
// across the resource (r.client), data source (d.client) and list resource
// (r.client) constructs — all share the `client.<Method>(` suffix.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names called in the
// given source file, restricted to names present in the SDK registry so that
// unrelated `client.` helpers (none here, but robust) cannot pollute the set.
func calledMethods(t *testing.T, filename string) map[string]bool {
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

// assertMatch fails if the file's SDK client calls do not equal declared.
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
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
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

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table in
// sync with the client.<Method> calls in crud.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps the data source privileges table in
// sync with the client.<Method> calls in data_source.go.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls keeps the list resource privileges
// table in sync with the client.<Method> calls in list_resource.go.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

// TestPrivileges_Rendered guards that the tables actually rendered into the
// construct descriptions (catches an empty/parse-skipped registry).
func TestPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "enrollment-invitations:create") {
		t.Fatalf("resourcePrivileges did not render the create privilege:\n%s", resourcePrivileges)
	}
	if !permissions.Renders(dataSourcePrivileges, "enrollment-invitations:read") {
		t.Fatalf("dataSourcePrivileges did not render the read privilege:\n%s", dataSourcePrivileges)
	}
	if !permissions.Renders(listResourcePrivileges, "enrollment-invitations:read") {
		t.Fatalf("listResourcePrivileges did not render the read privilege:\n%s", listResourcePrivileges)
	}
}
