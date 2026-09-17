// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches an SDK method call on a client value — r.client.Method(
// or d.client.Method( — which is how every construct in this package reaches
// Jamf Pro.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names the named file invokes
// on a client, restricted to those the privilege registry knows so a resolver or
// other unprivileged helper does not leak into the assertion.
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

// assertMethodsMatch fails when a file's calls and its declared method list
// disagree in either direction, which is what keeps the rendered permission
// table honest as the code path changes.
func assertMethodsMatch(t *testing.T, filename string, declaredMethods []string) {
	t.Helper()
	called := calledSDKMethods(t, filename)
	declared := map[string]bool{}
	for _, m := range declaredMethods {
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
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMethodsMatch(t, "crud.go", resourceSDKMethods)
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMethodsMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMethodsMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMethodsMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{"device-groups:create", "device-groups:delete", "devices:read"} {
		if !permissions.Renders(resourcePrivileges, want) {
			t.Errorf("resourcePrivileges did not render %q:\n%s", want, resourcePrivileges)
		}
	}
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{"device-groups:read", "devices:read"} {
		if !permissions.Renders(dataSourcePrivileges, want) {
			t.Errorf("dataSourcePrivileges did not render %q:\n%s", want, dataSourcePrivileges)
		}
	}
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "device-groups:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the device group read permission:\n%s", pluralDataSourcePrivileges)
	}
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "device-groups:read") {
		t.Fatalf("listResourcePrivileges did not render the device group read permission:\n%s", listResourcePrivileges)
	}
}
