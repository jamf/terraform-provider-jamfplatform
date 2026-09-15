// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form
// <receiver>.client.<Method>( — covering r.client / d.client receivers used by
// the resource, data sources, and list resource in this package.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names invoked in the given
// source file, restricted to methods known to the SDK privilege registry so
// unrelated identifiers (helpers, framework calls) are ignored.
func calledMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if _, ok := account.Privileges[name]; ok {
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
	if missing := permissions.Missing(account.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description, for all three actions the lifecycle needs.
func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, scoped := range []string{"sso-domains:create", "sso-domains:read", "sso-domains:delete"} {
		if !permissions.Renders(resourcePrivileges, scoped) {
			t.Errorf("resourcePrivileges did not render %s:\n%s", scoped, resourcePrivileges)
		}
	}
}

// TestResourcePrivileges_NeverClaimsNoPermissionNeeded is the guard against the
// upstream defect this namespace shipped with. The published spec declares no
// privileges for any Jamf Account operation, and an empty declaration renders as
// "None — any authenticated … integration may call the underlying endpoints",
// which for these operations is false. The SDK now supplies the gateway's own
// policy; this asserts the provider never publishes the false claim if that
// regresses.
func TestResourcePrivileges_NeverClaimsNoPermissionNeeded(t *testing.T) {
	for name, section := range map[string]string{
		"resource":           resourcePrivileges,
		"data source":        dataSourcePrivileges,
		"plural data source": pluralDataSourcePrivileges,
		"list resource":      listResourcePrivileges,
	} {
		if strings.Contains(section, "None — any authenticated") {
			t.Errorf("the %s permissions section claims no permission is needed:\n%s", name, section)
		}
		if section == "" {
			t.Errorf("the %s permissions section is empty", name)
		}
	}
}

// TestResourceSDKMethods_ExcludeVerify pins the division of labour with the
// verification action. Verification is the only Jamf Account domain operation
// needing the update action, it is rate-limited, and it mutates two timestamps on
// every call — so it belongs to an action, not to this resource's lifecycle.
func TestResourceSDKMethods_ExcludeVerify(t *testing.T) {
	for _, m := range resourceSDKMethods {
		if m == "VerifyDomain" {
			t.Error("VerifyDomain must not be part of the resource lifecycle")
		}
	}
	if permissions.Renders(resourcePrivileges, "sso-domains:update") {
		t.Errorf("resourcePrivileges must not ask for the update action:\n%s", resourcePrivileges)
	}
}

// --- data source ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(account.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "sso-domains:read") {
		t.Fatalf("dataSourcePrivileges did not render the SSO domain privileges:\n%s", dataSourcePrivileges)
	}
}

// --- plural data source ---

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(account.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "sso-domains:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the SSO domain privileges:\n%s", pluralDataSourcePrivileges)
	}
}

// --- list resource ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(account.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "sso-domains:read") {
		t.Fatalf("listResourcePrivileges did not render the SSO domain privileges:\n%s", listResourcePrivileges)
	}
}
