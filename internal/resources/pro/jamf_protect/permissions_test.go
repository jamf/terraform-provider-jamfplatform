// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_protect

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe extracts the SDK method name from every client.<Method>( call in
// a construct's source file (matches both r.client.X( and d.client.X().
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the set of SDK method names a construct source file
// invokes on its client value, restricted to names known to the SDK privilege
// registry so unrelated helper calls do not pollute the comparison.
func calledSDKMethods(t *testing.T, file string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertDeclaredMatchesCalls fails if the source file calls an SDK method not
// declared, or declares one it does not call.
func assertDeclaredMatchesCalls(t *testing.T, file string, declaredList []string) {
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
		t.Errorf("declared list has methods %s does not call: %v", file, uncalled)
	}
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the privileges table honest as
// the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertDeclaredMatchesCalls(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "jamf-protect-deployments:update") {
		t.Fatalf("resourcePrivileges did not render the Jamf Protect privileges:\n%s", resourcePrivileges)
	}
}

// TestPluralDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPluralDataSourceSDKMethods_MatchReadCalls keeps the data source
// privileges table in sync with the calls it actually makes.
func TestPluralDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertDeclaredMatchesCalls(t, "data_source.go", pluralDataSourceSDKMethods)
}

// TestPluralDataSourcePrivileges_Rendered guards that the table actually
// rendered into the data source description.
func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "jamf-protect-deployments:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the Jamf Protect plans privileges:\n%s", pluralDataSourcePrivileges)
	}
}
