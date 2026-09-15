// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package gsx_connection

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCalls extracts the distinct client.<Method>( calls from a source file
// that resolve to a method in the SDK privilege registry.
func clientCalls(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// diffSets reports method names called-but-undeclared and declared-but-uncalled.
func diffSets(declaredList []string, called map[string]bool) (undeclared, uncalled []string) {
	declared := map[string]bool{}
	for _, m := range declaredList {
		declared[m] = true
	}
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
	return undeclared, uncalled
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed
// or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method
// not declared in resourceSDKMethods, or declares one it does not call —
// keeping the privileges table honest as the CRUD path changes.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	called := clientCalls(t, "crud.go")
	undeclared, uncalled := diffSets(resourceSDKMethods, called)
	if len(undeclared) > 0 {
		t.Errorf("crud.go calls SDK methods missing from resourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("resourceSDKMethods declares methods crud.go does not call: %v", uncalled)
	}
}

// TestResourcePrivileges_Rendered is a guard that the table actually rendered
// into the resource description (catches an empty/parse-skipped registry). It
// names every capability-action pair the table renders, not one of them: the
// GSX read and write both require the APNS push certificate alongside the GSX
// connection itself, and a row dropped from the table is invisible to a check
// that only looks for the row it kept.
func TestResourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{
		"gsx-connection:read",
		"gsx-connection:update",
		"push-certificates:read",
		"push-certificates:update",
	} {
		if !permissions.Renders(resourcePrivileges, want) {
			t.Errorf("resourcePrivileges did not render %q:\n%s", want, resourcePrivileges)
		}
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not
// call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	called := clientCalls(t, "data_source.go")
	undeclared, uncalled := diffSets(dataSourceSDKMethods, called)
	if len(undeclared) > 0 {
		t.Errorf("data_source.go calls SDK methods missing from dataSourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("dataSourceSDKMethods declares methods data_source.go does not call: %v", uncalled)
	}
}

// TestDataSourcePrivileges_Rendered is a guard that the table actually rendered
// into the data source description. The read alone needs both capabilities, so
// both are named.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	for _, want := range []string{
		"gsx-connection:read",
		"push-certificates:read",
	} {
		if !permissions.Renders(dataSourcePrivileges, want) {
			t.Errorf("dataSourcePrivileges did not render %q:\n%s", want, dataSourcePrivileges)
		}
	}
}
