// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_connect

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkCallRe matches an SDK client method call (r.client.Foo(, d.client.Foo()
// on the construct's client field.
var sdkCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names a source file calls
// that are present in the pro privilege registry. Filtering by the registry
// keeps incidental client.* helper calls (none here) from leaking in.
func calledSDKMethods(t *testing.T, file string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	called := map[string]bool{}
	for _, m := range sdkCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the file's SDK calls diverge from the declared list.
func assertMatch(t *testing.T, file string, declaredList []string) {
	t.Helper()
	called := calledSDKMethods(t, file)
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
		t.Errorf("%s calls SDK methods missing from the declared list: %v", file, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list contains methods %s does not call: %v", file, uncalled)
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
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "jamf-connect-deployments:update") {
		t.Fatalf("resourcePrivileges did not render the Jamf Connect privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered
// into the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "jamf-connect-deployments:read") {
		t.Fatalf("dataSourcePrivileges did not render the Jamf Connect privileges:\n%s", dataSourcePrivileges)
	}
}
