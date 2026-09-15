// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkCallRe matches an SDK client method call regardless of the receiver name
// (the resource CRUD path uses a `client` parameter, the data sources use
// `d.client`). Callers filter the captures down to the declared method set.
var sdkCallRe = regexp.MustCompile(`\b\w+\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names invoked in the named
// source file, restricted to the supplied declared set (so receiver/helper
// calls like `resp.Diagnostics.Append(` are ignored).
func calledSDKMethods(t *testing.T, filename string, declared map[string]bool) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range sdkCallRe.FindAllStringSubmatch(string(src), -1) {
		if declared[m[1]] {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatchesDeclared fails if the declared SDK method list and the set of
// SDK calls discovered in the source file diverge.
func assertMatchesDeclared(t *testing.T, filename string, methods []string) {
	t.Helper()
	declared := map[string]bool{}
	for _, m := range methods {
		declared[m] = true
	}
	called := calledSDKMethods(t, filename, declared)

	var uncalled []string
	for m := range declared {
		if !called[m] {
			uncalled = append(uncalled, m)
		}
	}
	var undeclared []string
	for m := range called {
		if !declared[m] {
			undeclared = append(undeclared, m)
		}
	}
	sort.Strings(uncalled)
	sort.Strings(undeclared)

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

// TestResourceSDKMethods_MatchCRUDCalls keeps resourceSDKMethods honest against
// the actual client.<Method> calls in crud.go.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatchesDeclared(t, "crud.go", resourceSDKMethods)
}

// TestDataSourceSDKMethods_KnownToSDK is the SDK drift guard for the SSO
// settings data source.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps dataSourceSDKMethods honest against
// data_source.go.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatchesDeclared(t, "data_source.go", dataSourceSDKMethods)
}

// TestDependenciesDataSourceSDKMethods_KnownToSDK is the SDK drift guard for the
// SSO dependencies data source.
func TestDependenciesDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dependenciesDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dependenciesDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDependenciesDataSourceSDKMethods_MatchCalls keeps
// dependenciesDataSourceSDKMethods honest against data_source_dependencies.go.
func TestDependenciesDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatchesDeclared(t, "data_source_dependencies.go", dependenciesDataSourceSDKMethods)
}

// TestMetadataDataSourceSDKMethods_KnownToSDK is the SDK drift guard for the
// SSO SP metadata data source.
func TestMetadataDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, metadataDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("metadataDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestMetadataDataSourceSDKMethods_MatchCalls keeps metadataDataSourceSDKMethods
// honest against data_source_metadata.go.
func TestMetadataDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatchesDeclared(t, "data_source_metadata.go", metadataDataSourceSDKMethods)
}

// TestPrivileges_Rendered guards that each construct's table rendered the sso-settings
// row *and* the action that construct actually performs. Asserting the action —
// not just the capability — is what makes this a drift guard: a row that ticked
// the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "sso-settings:update"},
		{"dataSourcePrivileges", dataSourcePrivileges, "sso-settings:read"},
		{"dependenciesDataSourcePrivileges", dependenciesDataSourcePrivileges, "sso-settings:read"},
		{"metadataDataSourcePrivileges", metadataDataSourcePrivileges, "sso-settings:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}
