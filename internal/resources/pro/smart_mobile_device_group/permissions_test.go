// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// assertMatch fails if the called set and the declared set differ.
//
// Calls are read from the parsed file rather than matched against its bytes, so
// a method named only in a comment cannot satisfy the "is called" half of the
// comparison — which matters more here than it used to, because the endpoint
// annotation block at the top of crud.go now names seven of them.
//
// The merged registry is the filter for every file, including the three that
// render from the Pro registry alone. Detection and rendering are separate
// questions: a classic call arriving in a data source should be reported as
// undeclared, not filtered out of sight because that file's table does not carry
// the classic family.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called, err := permissions.SDKCallsInFile(filename, privilegeRegistry)
	if err != nil {
		t.Fatalf("reading SDK calls from %s: %v", filename, err)
	}
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

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(privilegeRegistry, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in the merged privilege registry (SDK drift): %v", missing)
	}
}

// TestPrivilegeRegistryMergeIsDisjoint pins the reason the two registries can be
// merged at all: no method this construct names is carried by both, so the merge
// cannot silently answer for one family with the other's entry.
func TestPrivilegeRegistryMergeIsDisjoint(t *testing.T) {
	declared := make([]string, 0, len(resourceSDKMethods)+len(dataSourceSDKMethods)+len(pluralDataSourceSDKMethods)+len(listResourceSDKMethods))
	declared = append(declared, resourceSDKMethods...)
	declared = append(declared, dataSourceSDKMethods...)
	declared = append(declared, pluralDataSourceSDKMethods...)
	declared = append(declared, listResourceSDKMethods...)

	for _, method := range declared {
		_, inPro := pro.Privileges[method]
		_, inClassic := proclassic.Privileges[method]
		if inPro && inClassic {
			t.Errorf("%s is carried by both privilege registries, so the merge picks one arbitrarily", method)
		}
	}
}

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, scoped := range []string{"device-groups:create", "device-groups:read", "device-groups:update", "device-groups:delete"} {
		if !permissions.Renders(resourcePrivileges, scoped) {
			t.Errorf("resourcePrivileges did not render %q:\n%s", scoped, resourcePrivileges)
		}
	}
}

// --- data source ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "device-groups:read") {
		t.Fatalf("dataSourcePrivileges did not render the device group read permission:\n%s", dataSourcePrivileges)
	}
}

// --- plural data source ---

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "device-groups:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the device group read permission:\n%s", pluralDataSourcePrivileges)
	}
}

// --- list resource ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "list_resource.go", listResourceSDKMethods)
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "device-groups:read") {
		t.Fatalf("listResourcePrivileges did not render the device group read permission:\n%s", listResourcePrivileges)
	}
}
