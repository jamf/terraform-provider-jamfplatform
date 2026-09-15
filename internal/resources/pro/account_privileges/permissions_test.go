// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_privileges

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the proclassic SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if the data source's Read path
// reaches an SDK method not declared in dataSourceSDKMethods, or declares one it
// does not reach — keeping the privileges table honest as the read path changes.
//
// The data source delegates its SDK access to accountprivileges.DiscoverCategorized,
// so the actual proclassic client calls live in the discovery helper rather than
// in data_source.go. The test scans both files for receiver.<Method>( calls and
// keeps only those names present in the proclassic privilege registry — that
// filter excludes framework/helper calls (req.Config.Get, etc.) while capturing
// every real SDK method the read path exercises.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	sources := []string{
		"data_source.go",
		"../../../common/accountprivileges/discovery.go",
	}

	re := regexp.MustCompile(`\b\w+\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, path := range sources {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			if _, ok := proclassic.Privileges[name]; ok {
				called[name] = true
			}
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
		t.Errorf("read path calls SDK methods missing from dataSourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("dataSourceSDKMethods declares methods the read path does not call: %v", uncalled)
	}
}

// TestDataSourcePrivileges_Rendered is a guard that the table actually rendered
// into the data source description (catches an empty/parse-skipped registry).
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "accounts:read") {
		t.Fatalf("dataSourcePrivileges did not render the accounts privileges:\n%s", dataSourcePrivileges)
	}
}
