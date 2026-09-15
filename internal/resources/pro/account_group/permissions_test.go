// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkCallRe matches an SDK client method call regardless of the receiver prefix
// (client. / r.client. / d.client.) — \bclient\. anchors on the client. token.
var sdkCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names invoked in a construct
// source file, restricted to those known to the SDK privilege registry so the
// regex does not pick up unrelated client.<X>() helpers.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range sdkCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the declared method list and the methods actually called
// in the construct source file diverge in either direction.
func assertMatch(t *testing.T, filename string, declaredList []string, called map[string]bool) {
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
		t.Errorf("declared list contains methods %s does not call: %v", filename, uncalled)
	}
}

// --- Resource ---------------------------------------------------------------

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as crud.go changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods, calledSDKMethods(t, "crud.go"))
}

// TestResourcePrivileges_Rendered guards that the table actually rendered.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "accounts:create") {
		t.Fatalf("resourcePrivileges did not render the accounts privileges:\n%s", resourcePrivileges)
	}
}

// --- Data source ------------------------------------------------------------

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source privileges
// table honest as data_source.go changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods, calledSDKMethods(t, "data_source.go"))
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "accounts:read") {
		t.Fatalf("dataSourcePrivileges did not render the accounts privileges:\n%s", dataSourcePrivileges)
	}
}

// --- List resource ----------------------------------------------------------

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls keeps the list resource privileges
// table honest as list_resource.go changes.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods, calledSDKMethods(t, "list_resource.go"))
}

// TestListResourcePrivileges_Rendered guards that the table actually rendered.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "accounts:read") {
		t.Fatalf("listResourcePrivileges did not render the accounts privileges:\n%s", listResourcePrivileges)
	}
}
