// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form <recv>.client.<Method>(
// and bare client.<Method>(. The leading `client.` substring is shared by the
// resource (r.client), data source (d.client), and list resource (r.client)
// receivers, so one pattern covers every construct in this package.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct method names a source file invokes on
// the SDK client, restricted to names the SDK privilege registry knows about so
// unrelated helper calls do not pollute the set.
func calledSDKMethods(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if _, known := proclassic.Privileges[name]; known {
			called[name] = true
		}
	}
	out := make([]string, 0, len(called))
	for m := range called {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// assertSetEquals compares the declared SDK method list for a construct against
// the set actually called in its source file.
func assertSetEquals(t *testing.T, construct string, declared []string, called []string) {
	t.Helper()
	declaredSet := map[string]bool{}
	for _, m := range declared {
		declaredSet[m] = true
	}
	calledSet := map[string]bool{}
	for _, m := range called {
		calledSet[m] = true
	}

	var undeclared, uncalled []string
	for m := range calledSet {
		if !declaredSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declaredSet {
		if !calledSet[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("%s calls SDK methods missing from its declared list: %v", construct, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("%s declares methods its source does not call: %v", construct, uncalled)
	}
}

// --- resource ---------------------------------------------------------------

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls keeps the resource privileges table
// honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertSetEquals(t, "crud.go", resourceSDKMethods, calledSDKMethods(t, "crud.go"))
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into
// the resource description.
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "configuration-profiles:create") {
		t.Fatalf("resourcePrivileges did not render the profile privileges:\n%s", resourcePrivileges)
	}
}

// --- data source ------------------------------------------------------------

// TestDataSourceSDKMethods_KnownToSDK fails on SDK drift for the data source.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps the data source privileges table
// in sync with data_source.go.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertSetEquals(t, "data_source.go", dataSourceSDKMethods, calledSDKMethods(t, "data_source.go"))
}

// TestDataSourcePrivileges_Rendered guards the data source table rendered.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "configuration-profiles:read") {
		t.Fatalf("dataSourcePrivileges did not render the profile privileges:\n%s", dataSourcePrivileges)
	}
}

// --- list resource ----------------------------------------------------------

// TestListResourceSDKMethods_KnownToSDK fails on SDK drift for the list resource.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls keeps the list resource privileges
// table in sync with list_resource.go.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertSetEquals(t, "list_resource.go", listResourceSDKMethods, calledSDKMethods(t, "list_resource.go"))
}

// TestListResourcePrivileges_Rendered guards the list resource table rendered.
func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "configuration-profiles:read") {
		t.Fatalf("listResourcePrivileges did not render the profile privileges:\n%s", listResourcePrivileges)
	}
}
