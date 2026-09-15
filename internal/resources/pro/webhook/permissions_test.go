// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkCallRE matches client.<Method>( calls on any receiver (client., r.client.,
// d.client.) — the leading word boundary anchors on "client".
var sdkCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK methods invoked in the named source
// file that are present in the proclassic privilege registry. Filtering by the
// registry keeps non-SDK helpers (e.g. context.WithTimeout) out of the set.
func calledSDKMethods(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range sdkCallRE.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	out := make([]string, 0, len(called))
	for m := range called {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// assertSetEquals fails if declared and called diverge, reporting both
// directions so a stale privileges var or a new SDK call is caught.
func assertSetEquals(t *testing.T, file string, declared, called []string) {
	t.Helper()
	want := map[string]bool{}
	for _, m := range declared {
		want[m] = true
	}
	have := map[string]bool{}
	for _, m := range called {
		have[m] = true
	}
	var undeclared, uncalled []string
	for m := range have {
		if !want[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range want {
		if !have[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)
	if len(undeclared) > 0 {
		t.Errorf("%s calls SDK methods missing from the declared list: %v", file, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", file, uncalled)
	}
}

// --- resource (crud.go) ---

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertSetEquals(t, "crud.go", resourceSDKMethods, calledSDKMethods(t, "crud.go"))
}

// --- data source (data_source.go) ---

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertSetEquals(t, "data_source.go", dataSourceSDKMethods, calledSDKMethods(t, "data_source.go"))
}

// --- list resource (list_resource.go) ---

// TestListResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestListResourceSDKMethods_MatchCalls fails if list_resource.go calls an SDK
// method not declared in listResourceSDKMethods, or declares one it does not call.
func TestListResourceSDKMethods_MatchCalls(t *testing.T) {
	assertSetEquals(t, "list_resource.go", listResourceSDKMethods, calledSDKMethods(t, "list_resource.go"))
}

// TestPrivileges_Rendered guards that the tables actually rendered into the
// construct descriptions (catches an empty/parse-skipped registry).
func TestPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "webhooks:create") {
		t.Fatalf("resourcePrivileges did not render the webhooks privileges:\n%s", resourcePrivileges)
	}
	if !permissions.Renders(dataSourcePrivileges, "webhooks:read") {
		t.Fatalf("dataSourcePrivileges did not render the webhooks privileges:\n%s", dataSourcePrivileges)
	}
	if !permissions.Renders(listResourcePrivileges, "webhooks:read") {
		t.Fatalf("listResourcePrivileges did not render the webhooks privileges:\n%s", listResourcePrivileges)
	}
}
