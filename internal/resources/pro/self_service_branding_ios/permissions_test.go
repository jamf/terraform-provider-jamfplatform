// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_ios

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form
// "<receiver>.client.<Method>(" — covering both the resource's r.client and
// the data source's d.client receivers.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names invoked on the client
// in the named source file, filtered to those known to the pro registry so
// helper/non-SDK calls don't leak in.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMethodsMatch fails if the called set and the declared set differ.
func assertMethodsMatch(t *testing.T, filename string, declared []string, called map[string]bool) {
	t.Helper()
	declaredSet := map[string]bool{}
	for _, m := range declared {
		declaredSet[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !declaredSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declaredSet {
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

// TestResourceSDKMethods_KnownToSDK fails if a declared resource method has been
// renamed or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	called := calledSDKMethods(t, "crud.go")
	assertMethodsMatch(t, "crud.go", resourceSDKMethods, called)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "self-service:create") {
		t.Fatalf("resourcePrivileges did not render the branding privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method has
// been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	called := calledSDKMethods(t, "data_source.go")
	assertMethodsMatch(t, "data_source.go", dataSourceSDKMethods, called)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "self-service:read") {
		t.Fatalf("dataSourcePrivileges did not render the branding privileges:\n%s", dataSourcePrivileges)
	}
}
