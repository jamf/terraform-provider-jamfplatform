// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form <receiver>.client.<Method>( — covering
// the r.client and d.client receivers used by the resource and data sources in this package.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// cacheCallRe matches calls into the shared schema cache, which reaches the SDK on this package's
// behalf. Without it the plan-time validation path would look as though it needed no privileges,
// because the calls it makes are one indirection away.
var cacheCallRe = regexp.MustCompile(`\bschemas\.([A-Za-z0-9]+)\(`)

// cacheMethodBacking maps each schema cache method onto the SDK method it calls.
var cacheMethodBacking = map[string]string{
	"Tool":     "ListTools",
	"Tools":    "ListTools",
	"Document": "GetToolSchema",
}

// cacheBookkeepingMethods are schema cache methods that reach no API and so need no privilege.
// NoticeOnce is the once-per-plan latch on the "validation unavailable" warning; declaring it here
// rather than ignoring unmapped names keeps the mapping's guarantee, so a cache method that does
// reach the SDK still fails calledMethods until it is mapped.
var cacheBookkeepingMethods = map[string]bool{
	"NoticeOnce": true,
}

// calledMethods returns the distinct SDK method names the given source files reach, directly or
// through the schema cache, restricted to methods the SDK privilege registry knows so unrelated
// identifiers are ignored.
func calledMethods(t *testing.T, filenames ...string) map[string]bool {
	t.Helper()
	called := map[string]bool{}
	for _, filename := range filenames {
		src, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("reading %s: %v", filename, err)
		}
		for _, match := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
			if _, ok := aigovernance.Privileges[match[1]]; ok {
				called[match[1]] = true
			}
		}
		for _, match := range cacheCallRe.FindAllStringSubmatch(string(src), -1) {
			if cacheBookkeepingMethods[match[1]] {
				continue
			}
			backing, ok := cacheMethodBacking[match[1]]
			if !ok {
				t.Errorf("%s calls schemas.%s, which permissions_test.go does not map to an SDK method", filename, match[1])
				continue
			}
			called[backing] = true
		}
	}
	return called
}

// assertMatch fails if the called set and the declared set differ.
func assertMatch(t *testing.T, declared []string, filenames ...string) {
	t.Helper()
	called := calledMethods(t, filenames...)
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
		t.Errorf("%v call SDK methods missing from the declared list: %v", filenames, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %v do not call: %v", filenames, uncalled)
	}
}

// --- resource ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed or removed in the
// SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(aigovernance.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in aigovernance.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCalls keeps the resource privileges table honest as the CRUD and
// plan-time validation paths change.
func TestResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, resourceSDKMethods, "crud.go", "validators.go")
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into the description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, privilege := range []string{"ai-policies:create", "ai-policies:read", "ai-policies:update", "ai-policies:delete"} {
		if !permissions.Renders(resourcePrivileges, privilege) {
			t.Errorf("resourcePrivileges did not render %s:\n%s", privilege, resourcePrivileges)
		}
	}
}

// --- data source ---

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(aigovernance.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in aigovernance.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, dataSourceSDKMethods, "data_source.go")
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "ai-policies:read") {
		t.Fatalf("dataSourcePrivileges did not render the read privilege:\n%s", dataSourcePrivileges)
	}
}

// --- plural data source ---

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(aigovernance.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in aigovernance.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, pluralDataSourceSDKMethods, "datasource_plural.go")
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "ai-policies:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the read privilege:\n%s", pluralDataSourcePrivileges)
	}
}

// --- list resource ---

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(aigovernance.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in aigovernance.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, listResourceSDKMethods, "list_resource.go")
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "ai-policies:read") {
		t.Fatalf("listResourcePrivileges did not render the read privilege:\n%s", listResourcePrivileges)
	}
}
