// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// indirectCalls maps a call this package makes that the privilege registry does
// not name onto the SDK method it actually issues, so a declared list stays
// honest about a request made on this package's behalf.
//
// Three shapes. Two progroups helpers bridge the Jamf Pro and Jamf Platform
// identifier spaces. The shared directory-service group criterion helpers each
// search the configured directories, and there are four of them because the
// write, the read-back and the plan-time suppression all need it. And the SDK's
// own name resolver is built on the search endpoint and carries no registry
// entry of its own.
var indirectCalls = map[string]string{
	"JamfProIDForPlatformID":              "GetGroupV2",
	"PlatformIDsByJamfProID":              "ListGroupsV2",
	"ResolveDSGroupCriteria":              "SearchLdapGroupsV1",
	"ReadbackDSGroupCriteria":             "SearchLdapGroupsV1",
	"SuppressEquivalentDSGroupValues":     "SearchLdapGroupsV1",
	"ResolveSmartComputerGroupV3IDByName": "ListSmartComputerGroupsV3",
}

// calledMethods returns the SDK methods the named file reaches, directly or
// through one of the indirect helpers above.
//
// It parses the file rather than scanning its bytes, because a comment or a
// string literal naming a method would otherwise satisfy the "is called" half of
// the comparison on its own. parser.ParseComments is deliberately not passed, so
// a comment is absent from the tree and cannot be reached by accident.
//
// The registry it recognises calls against is the merged one, so the fallback
// write path's calls on the older interface count as calls. Scanning the Pro
// registry alone would have read crud.go as not making them and the declared
// list as over-stating what the resource needs.
func calledMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", filename, err)
	}

	called := map[string]bool{}
	ast.Inspect(parsed, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := selector.Sel.Name
		if mapped, ok := indirectCalls[name]; ok {
			called[mapped] = true
			return true
		}
		if _, known := privilegeRegistry[name]; known {
			called[name] = true
		}
		return true
	})
	return called
}

// assertMatch fails when the calls a file makes and the methods it declares
// disagree in either direction.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := calledMethods(t, filename)
	want := map[string]bool{}
	for _, method := range declared {
		want[method] = true
	}

	var undeclared, uncalled []string
	for method := range called {
		if !want[method] {
			undeclared = append(undeclared, method)
		}
	}
	for method := range want {
		if !called[method] {
			uncalled = append(uncalled, method)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("%s reaches SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not reach: %v", filename, uncalled)
	}
}

// TestIndirectCallsAreRealSDKMethods keeps the indirection table from mapping a
// call onto a method the SDK no longer has, which would silently excuse the call
// from the comparison.
func TestIndirectCallsAreRealSDKMethods(t *testing.T) {
	for helper, method := range indirectCalls {
		if _, ok := pro.Privileges[method]; !ok {
			t.Errorf("indirectCalls maps %s onto %q, which is not in pro.Privileges", helper, method)
		}
	}
}

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(privilegeRegistry, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in the merged privilege registry (SDK drift): %v", missing)
	}
}

// TestPrivilegeRegistryMergeIsDisjoint pins what makes the merge safe: no method
// the resource names is carried by both registries, so nothing is rendered from
// whichever one happened to be copied last.
func TestPrivilegeRegistryMergeIsDisjoint(t *testing.T) {
	for _, method := range resourceSDKMethods {
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

func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, scoped := range []string{"device-groups:create", "device-groups:update", "device-groups:delete", "ldap-servers:read"} {
		if !permissions.Renders(resourcePrivileges, scoped) {
			t.Errorf("resourcePrivileges did not render %q:\n%s", scoped, resourcePrivileges)
		}
	}
}

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
		t.Fatalf("dataSourcePrivileges did not render the device group privileges:\n%s", dataSourcePrivileges)
	}
}

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
		t.Fatalf("pluralDataSourcePrivileges did not render the device group privileges:\n%s", pluralDataSourcePrivileges)
	}
}

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
		t.Fatalf("listResourcePrivileges did not render the device group privileges:\n%s", listResourcePrivileges)
	}
}
