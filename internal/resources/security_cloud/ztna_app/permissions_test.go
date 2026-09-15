// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form
// <receiver>.client.<Method>( — covering r.client / d.client receivers used by
// the resource, data sources, and list resource in this package.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// syntheticMethodBacking maps a synthetic SDK helper onto the generated method
// whose privileges it actually consumes. The privilege registry omits
// Resolve<X>ByName / Apply<X> by design, so a call site using one would otherwise
// look like it needs no privileges at all.
//
// This package uses none of them. Application names are not unique on this
// endpoint and a predefined application has no name at all, so both non-ID lookups
// are done in-package over ListZtnaAppsV1 rather than through
// ResolveZtnaAppV1ByName — which means every call site here is already a generated
// method. The map stays so the helper reads the same as its siblings and so an
// added resolver call cannot slip through unattributed.
var syntheticMethodBacking = map[string]string{}

// calledMethods returns the distinct SDK client method names invoked in the given
// source file, restricted to methods known to the SDK privilege registry so
// unrelated identifiers (helpers, framework calls) are ignored. Synthetic helpers
// are resolved to the generated method they call.
func calledMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if backing, ok := syntheticMethodBacking[name]; ok {
			name = backing
		}
		if _, ok := securitycloud.Privileges[name]; ok {
			called[name] = true
		}
	}
	return called
}

// assertMatch fails if the called set and the declared set differ.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := calledMethods(t, filename)
	want := map[string]bool{}
	for _, m := range declared {
		want[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !want[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range want {
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

// --- resource ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "ztna:create") {
		t.Fatalf("resourcePrivileges did not render the ZTNA app privileges:\n%s", resourcePrivileges)
	}
}

// --- data source ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "ztna:read") {
		t.Fatalf("dataSourcePrivileges did not render the ZTNA app privileges:\n%s", dataSourcePrivileges)
	}
}

// --- plural data source ---

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "ztna:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the ZTNA app privileges:\n%s", pluralDataSourcePrivileges)
	}
}

// --- list resource ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "ztna:read") {
		t.Fatalf("listResourcePrivileges did not render the ZTNA app privileges:\n%s", listResourcePrivileges)
	}
}
