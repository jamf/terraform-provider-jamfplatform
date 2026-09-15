// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK method calls on the client value, e.g.
// r.client.CreateLicensedSoftwareByID( or d.client.GetLicensedSoftwareByName(.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct set of SDK method names a construct's
// source file invokes on its client value, restricted to methods the SDK
// privilege registry knows about.
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

// assertMethodSets fails if the construct's source file calls an SDK method not
// declared in the var, or declares one it does not call.
func assertMethodSets(t *testing.T, filename string, declaredMethods []string) {
	t.Helper()
	called := calledSDKMethods(t, filename)
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
		t.Errorf("%s calls SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list includes methods %s does not call: %v", filename, uncalled)
	}
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodSets(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source privileges table
// honest as its Read path changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMethodSets(t, "data_source.go", dataSourceSDKMethods)
}

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps the list resource privileges
// table honest as its List path changes.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMethodSets(t, "list_resource.go", listResourceSDKMethods)
}

// TestPrivileges_Rendered guards that each construct's table rendered the licensed-software
// row *and* the action that construct actually performs. Asserting the action —
// not just the capability — is what makes this a drift guard: a row that ticked
// the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "licensed-software:create"},
		{"dataSourcePrivileges", dataSourcePrivileges, "licensed-software:read"},
		{"listResourcePrivileges", listResourcePrivileges, "licensed-software:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}
