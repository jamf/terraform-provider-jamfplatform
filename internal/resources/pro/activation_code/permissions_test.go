// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches SDK client method calls of the form client.<Method>( on
// either the resource (r.client) or data source (d.client) receiver.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names invoked across the
// named source files, filtered to those present in the proclassic registry.
func calledMethods(t *testing.T, files ...string) map[string]bool {
	t.Helper()
	called := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
			if _, ok := proclassic.Privileges[m[1]]; ok {
				called[m[1]] = true
			}
		}
	}
	return called
}

// assertMatch fails if the source files call an SDK method not declared, or
// declare one they do not call.
func assertMatch(t *testing.T, declared []string, called map[string]bool) {
	t.Helper()
	decl := map[string]bool{}
	for _, m := range declared {
		decl[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !decl[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range decl {
		if !called[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("source calls SDK methods missing from declared list: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods source does not call: %v", uncalled)
	}
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if the resource CRUD path (crud.go
// + the applyActivationCode write path in helpers.go) calls an SDK method not
// declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	called := calledMethods(t, "crud.go", "helpers.go")
	assertMatch(t, resourceSDKMethods, called)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "activation-code:read") ||
		!permissions.Renders(resourcePrivileges, "activation-code:update") {
		t.Fatalf("resourcePrivileges did not render the activation-code privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	called := calledMethods(t, "data_source.go")
	assertMatch(t, dataSourceSDKMethods, called)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into
// the data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "activation-code:read") {
		t.Fatalf("dataSourcePrivileges did not render the activation-code privileges:\n%s", dataSourcePrivileges)
	}
}
