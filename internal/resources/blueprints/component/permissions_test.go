// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package component

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(blueprints.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in blueprints.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not call
// — keeping the privileges table honest as the Read path changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	src, err := os.ReadFile("data_source.go")
	if err != nil {
		t.Fatalf("reading data_source.go: %v", err)
	}

	known := map[string]bool{}
	for m := range blueprints.Privileges {
		known[m] = true
	}

	// The data source client is a field on the receiver (d.client.<Method>(...)),
	// so match any <recv>.<Method>( call and filter to methods the SDK registry
	// knows about.
	re := regexp.MustCompile(`\b\w+\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if known[m[1]] {
			called[m[1]] = true
		}
	}

	declared := map[string]bool{}
	for _, m := range dataSourceSDKMethods {
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
		t.Errorf("data_source.go calls SDK methods missing from dataSourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("dataSourceSDKMethods declares methods data_source.go does not call: %v", uncalled)
	}
}

// TestDataSourcePrivileges_Rendered is a guard that the table actually rendered
// into the data source description (catches an empty/parse-skipped registry).
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "blueprints:read") {
		t.Fatalf("dataSourcePrivileges did not render the blueprints privileges:\n%s", dataSourcePrivileges)
	}
}
