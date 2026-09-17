// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// assertMethodsMatch fails when a file's SDK calls differ from the method list
// that renders its permissions table, keeping the table honest as the code path
// changes.
//
// It parses the file rather than scanning its bytes, so a method named only in
// a comment or a string cannot satisfy the "is called" half of the comparison.
func assertMethodsMatch(t *testing.T, filename string, declaredMethods []string) {
	t.Helper()

	called, err := permissions.SDKCallsInFile(filename, privilegeRegistry)
	if err != nil {
		t.Fatalf("reading SDK calls from %s: %v", filename, err)
	}

	declared := make(map[string]bool, len(declaredMethods))
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
		t.Errorf("the declared list names methods %s does not call: %v", filename, uncalled)
	}
}

// --- SDK drift guards: every declared method must exist in the registry. ---

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(privilegeRegistry, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods absent from the merged privilege registry (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(privilegeRegistry, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods absent from the merged privilege registry (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(privilegeRegistry, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods absent from the merged privilege registry (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(privilegeRegistry, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods absent from the merged privilege registry (SDK drift): %v", missing)
	}
}

// --- Call-site guards: declared methods must match the construct's calls. ---

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

// --- Render guards: the table each description actually carries. ---

func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, scoped := range []string{"device-groups:create", "device-groups:read", "device-groups:update", "device-groups:delete"} {
		if !permissions.Renders(resourcePrivileges, scoped) {
			t.Errorf("resourcePrivileges did not render %s:\n%s", scoped, resourcePrivileges)
		}
	}
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "device-groups:read") {
		t.Fatalf("dataSourcePrivileges did not render device-groups:read:\n%s", dataSourcePrivileges)
	}
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "device-groups:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render device-groups:read:\n%s", pluralDataSourcePrivileges)
	}
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "device-groups:read") {
		t.Fatalf("listResourcePrivileges did not render device-groups:read:\n%s", listResourcePrivileges)
	}
}

// TestPrivilegeRegistryMergeIsDisjoint pins the reason the two registries can be
// merged: no method this package names is carried by both, so the merge cannot
// silently prefer one family's entry over the other's.
func TestPrivilegeRegistryMergeIsDisjoint(t *testing.T) {
	all := make([]string, 0, len(resourceSDKMethods)+len(dataSourceSDKMethods)+len(pluralDataSourceSDKMethods)+len(listResourceSDKMethods))
	all = append(all, resourceSDKMethods...)
	all = append(all, dataSourceSDKMethods...)
	all = append(all, pluralDataSourceSDKMethods...)
	all = append(all, listResourceSDKMethods...)

	for _, method := range all {
		_, inPro := pro.Privileges[method]
		_, inClassic := proclassic.Privileges[method]
		if inPro && inClassic {
			t.Errorf("%s is carried by both privilege registries, so the merge picks one arbitrarily — render it from the registry that owns the call", method)
		}
		if !inPro && !inClassic {
			t.Errorf("%s is carried by neither privilege registry", method)
		}
	}
}
