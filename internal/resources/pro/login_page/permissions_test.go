// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package login_page

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCalls extracts the distinct SDK method names invoked on the SDK client
// value (r.client / d.client) in the given source file.
func clientCalls(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		called[m[1]] = true
	}
	return called
}

// assertMatch fails if the source file calls an SDK method not declared in
// methods, or declares one it does not call — keeping the privileges table
// honest as the construct's SDK usage changes.
func assertMatch(t *testing.T, filename string, methods []string) {
	t.Helper()
	called := clientCalls(t, filename)
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
		t.Errorf("declared list contains methods %s does not call: %v", filename, uncalled)
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

// TestResourcePrivileges_Rendered is a guard that the table actually rendered
// into the resource description (catches an empty/parse-skipped registry).
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "login-disclaimer:update") {
		t.Fatalf("resourcePrivileges did not render the login-disclaimer privilege:\n%s", resourcePrivileges)
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

// TestDataSourcePrivileges_Rendered is a guard that the section actually
// rendered into the data source description. GetLoginCustomizationV1 requires
// no special privilege, so the section is the "None" sentinel block.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !strings.Contains(dataSourcePrivileges, "Required Jamf permissions") {
		t.Fatalf("dataSourcePrivileges did not render a privileges section:\n%s", dataSourcePrivileges)
	}
}
