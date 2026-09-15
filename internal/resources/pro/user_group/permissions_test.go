// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form client.Method( or
// r.client.Method( / d.client.Method( — the receiver suffix is always
// "client." across this package's constructs.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// calledSDKMethods returns the distinct SDK method names invoked on a client
// value in the named source file, restricted to those known to the SDK
// registry so non-privileged helper calls don't leak into the assertion.
func calledSDKMethods(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := proclassic.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMethodsMatch fails if the file's client.<Method> calls differ from the
// declared SDK method list — keeping the privileges table honest as the code
// path changes.
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

// --- SDK drift guards: declared methods must exist in the SDK registry. ---

func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

func TestListResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, listResourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("listResourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
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

// --- Render guards: the table actually rendered into each description. ---

func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "user-groups:create") {
		t.Fatalf("resourcePrivileges did not render the user-groups privileges:\n%s", resourcePrivileges)
	}
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "user-groups:read") {
		t.Fatalf("dataSourcePrivileges did not render the user-groups privileges:\n%s", dataSourcePrivileges)
	}
}

func TestPluralDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(pluralDataSourcePrivileges, "user-groups:read") {
		t.Fatalf("pluralDataSourcePrivileges did not render the user-groups privileges:\n%s", pluralDataSourcePrivileges)
	}
}

func TestListResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(listResourcePrivileges, "user-groups:read") {
		t.Fatalf("listResourcePrivileges did not render the user-groups privileges:\n%s", listResourcePrivileges)
	}
}
