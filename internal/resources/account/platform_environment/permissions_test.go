// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

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
// <receiver>.client.<Method>( — covering the r.client and d.client receivers the
// resource, data sources and list resource use.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledMethods returns the distinct SDK client method names invoked in the given
// source file, restricted to methods known to the SDK privilege registry so
// unrelated identifiers are ignored.
func calledMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := account.Privileges[m[1]]; ok {
			called[m[1]] = true
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

func TestSDKMethods_KnownToSDK(t *testing.T) {
	for name, methods := range map[string][]string{
		"resource":           resourceSDKMethods,
		"data source":        dataSourceSDKMethods,
		"plural data source": pluralDataSourceSDKMethods,
		"list resource":      listResourceSDKMethods,
	} {
		if missing := permissions.Missing(account.Privileges, methods...); len(missing) > 0 {
			t.Errorf("%s methods not present in account.Privileges (SDK drift): %v", name, missing)
		}
	}
}

func TestSDKMethods_MatchTheCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, scoped := range []string{
		"platform-environments:create",
		"platform-environments:read",
		"platform-environments:update",
		"platform-environments:delete",
	} {
		if !permissions.Renders(resourcePrivileges, scoped) {
			t.Errorf("resourcePrivileges did not render %s:\n%s", scoped, resourcePrivileges)
		}
	}
}

func TestReadOnlyConstructsNeedOnlyTheReadPrivilege(t *testing.T) {
	for name, section := range map[string]string{
		"data source":        dataSourcePrivileges,
		"plural data source": pluralDataSourcePrivileges,
		"list resource":      listResourcePrivileges,
	} {
		if !permissions.Renders(section, "platform-environments:read") {
			t.Errorf("the %s section did not render platform-environments:read:\n%s", name, section)
		}
		for _, other := range []string{"create", "update", "delete"} {
			if permissions.Renders(section, "platform-environments:"+other) {
				t.Errorf("the %s section asks for the %s action:\n%s", name, other, section)
			}
		}
	}
}

// TestPrivileges_NeverClaimNoPermissionNeeded guards against the upstream defect
// this namespace shipped with: the published spec declares no privileges, and an
// empty declaration renders as "None — any authenticated … integration may call
// the underlying endpoints", which is false here.
func TestPrivileges_NeverClaimNoPermissionNeeded(t *testing.T) {
	for name, section := range map[string]string{
		"resource":           resourcePrivileges,
		"data source":        dataSourcePrivileges,
		"plural data source": pluralDataSourcePrivileges,
		"list resource":      listResourcePrivileges,
	} {
		if section == "" {
			t.Errorf("the %s permissions section is empty", name)
		}
		if strings.Contains(section, "None — any authenticated") {
			t.Errorf("the %s permissions section claims no permission is needed:\n%s", name, section)
		}
	}
}

func TestResourceSDKMethods_ExcludeAssignTenants(t *testing.T) {
	for _, m := range resourceSDKMethods {
		if m == "AssignTenants" {
			t.Error("AssignTenants is irreversible and belongs to the assign_tenants action, not this resource's lifecycle")
		}
	}
}
