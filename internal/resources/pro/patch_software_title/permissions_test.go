// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// mergedRegistry is the union of the SDK families every construct in this
// package spans: Pro v3 configuration CRUD plus the ProClassic create and patch
// source catalogues.
var mergedRegistry = permissions.Merge(proclassic.Privileges, pro.Privileges)

// callRe matches a "<receiver>.<Method>(" call. The test filters the captures to
// names present in the SDK privilege registry, so non-SDK calls (e.g.
// resp.Diagnostics.Append) are discarded.
var callRe = regexp.MustCompile(`\b\w+\.([A-Za-z0-9]+)\(`)

// sdkCallsIn returns the distinct SDK method names a source file calls, filtered
// to those known to reg.
func sdkCallsIn(t *testing.T, reg permissions.Registry, files ...string) map[string]bool {
	t.Helper()
	called := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		for _, m := range callRe.FindAllStringSubmatch(string(src), -1) {
			if _, ok := reg[m[1]]; ok {
				called[m[1]] = true
			}
		}
	}
	return called
}

// assertMatch fails if declared and called diverge, naming the gaps both ways.
func assertMatch(t *testing.T, declared []string, called map[string]bool, where string) {
	t.Helper()
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
		t.Errorf("%s calls SDK methods missing from the declared list: %v", where, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", where, uncalled)
	}
}

// --- resource (CRUD across Pro v3 + the ProClassic create) ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the merged SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(mergedRegistry, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in merged SDK registry (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCalls fails if the resource's CRUD path calls an
// SDK method not declared in resourceSDKMethods, or declares one it does not
// call. The v3 CRUD lives in crud.go, the extension-attribute side-channel in
// extension_attributes.go (invoked from Create/Read/Update), and the patch
// source catalogue reads behind source_id resolution in patch_sources.go.
func TestResourceSDKMethods_MatchCalls(t *testing.T) {
	called := sdkCallsIn(t, mergedRegistry, "crud.go", "extension_attributes.go", "patch_sources.go")
	assertMatch(t, resourceSDKMethods, called, "crud.go+extension_attributes.go+patch_sources.go")
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "patch-management-software-titles:create") {
		t.Fatalf("resourcePrivileges did not render the patch software title privileges:\n%s", resourcePrivileges)
	}
}

// --- data source ---

// TestDataSourceSDKMethods_KnownToSDK fails on SDK drift for the data source.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(mergedRegistry, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in merged SDK registry (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps the data source's privilege list in
// sync with data_source.go plus the patch source catalogue reads it resolves
// source_id through.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	called := sdkCallsIn(t, mergedRegistry, "data_source.go", "patch_sources.go")
	assertMatch(t, dataSourceSDKMethods, called, "data_source.go+patch_sources.go")
}

// TestDataSourcePrivileges_Rendered guards the data source table rendered.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "patch-management-software-titles:read") {
		t.Fatalf("dataSourcePrivileges did not render the patch software title privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource ---

// TestListResourceSDKMethods_KnownToSDK fails on SDK drift for the list resource.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(mergedRegistry, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in merged SDK registry (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls keeps the list resource's privilege list
// in sync with list_resource.go plus the patch source catalogue reads a listing
// resolves source_id through. Those two reads live in patch_sources.go — the one
// place they happen for every construct in this package — so the listing's
// declaration is derived from the same pair of files as the resource's and the
// data source's.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	called := sdkCallsIn(t, mergedRegistry, "list_resource.go", "patch_sources.go")
	assertMatch(t, listResourceSDKMethods, called, "list_resource.go+patch_sources.go")
}

// TestListResourcePrivileges_Rendered guards the list resource table rendered.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "patch-management-software-titles:read") {
		t.Fatalf("listResourcePrivileges did not render the patch software title privileges:\n%s", listResourcePrivileges)
	}
}
