// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE captures SDK client method calls of the form
// `client.<Method>(` regardless of the receiver expression preceding it
// (r.client / d.client / r.client). The trailing word-boundary on `client`
// matches both `r.client.Foo(` and a bare `client.Foo(`.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsIn returns the distinct SDK method names called in the named source
// file, filtered to those present in the proclassic privilege registry so that
// unrelated `client.` helper calls cannot pollute the comparison.
func sdkCallsIn(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// diffSets reports method calls present in the source but undeclared, and
// declared methods the source does not call.
func diffSets(declared []string, called map[string]bool) (undeclared, uncalled []string) {
	decl := map[string]bool{}
	for _, m := range declared {
		decl[m] = true
	}
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
	return undeclared, uncalled
}

// --- resource ---

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
	called := sdkCallsIn(t, "crud.go")
	undeclared, uncalled := diffSets(resourceSDKMethods, called)
	if len(undeclared) > 0 {
		t.Errorf("crud.go calls SDK methods missing from resourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("resourceSDKMethods declares methods crud.go does not call: %v", uncalled)
	}
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "json-web-token-configuration:create") {
		t.Fatalf("resourcePrivileges did not render the JSON Web Token configuration privileges:\n%s", resourcePrivileges)
	}
}

// --- data source ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared data source method
// has been renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	called := sdkCallsIn(t, "data_source.go")
	undeclared, uncalled := diffSets(dataSourceSDKMethods, called)
	if len(undeclared) > 0 {
		t.Errorf("data_source.go calls SDK methods missing from dataSourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("dataSourceSDKMethods declares methods data_source.go does not call: %v", uncalled)
	}
}

// TestDataSourcePrivileges_Rendered guards that the table rendered into the
// data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "json-web-token-configuration:read") {
		t.Fatalf("dataSourcePrivileges did not render the JSON Web Token configuration privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource ---

// TestListResourceSDKMethods_KnownToSDK fails if a declared list resource
// method has been renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchListCalls fails if list_resource.go calls an
// SDK method not declared in listResourceSDKMethods, or declares one it does
// not call.
func TestListResourceSDKMethods_MatchListCalls(t *testing.T) {
	called := sdkCallsIn(t, "list_resource.go")
	undeclared, uncalled := diffSets(listResourceSDKMethods, called)
	if len(undeclared) > 0 {
		t.Errorf("list_resource.go calls SDK methods missing from listResourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("listResourceSDKMethods declares methods list_resource.go does not call: %v", uncalled)
	}
}

// TestListResourcePrivileges_Rendered guards that the table rendered into the
// list resource description.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "json-web-token-configuration:read") {
		t.Fatalf("listResourcePrivileges did not render the JSON Web Token configuration privileges:\n%s", listResourcePrivileges)
	}
}
