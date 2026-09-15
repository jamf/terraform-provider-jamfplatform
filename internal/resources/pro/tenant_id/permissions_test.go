// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenant_id

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK
// method not declared in dataSourceSDKMethods, or declares one it does not
// call — keeping the privileges table honest as the Read path changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	src, err := os.ReadFile("data_source.go")
	if err != nil {
		t.Fatalf("reading data_source.go: %v", err)
	}
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		// Only count calls that resolve to a known SDK method so that
		// unrelated client.<helper> calls do not pollute the comparison.
		if _, ok := pro.Privileges[m[1]]; ok {
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

// TestDataSourcePrivileges_RendersTheNoPrivilegeVariant pins that this data
// source's privileges section is the "None" variant rather than a table.
//
// Worth pinning rather than leaving implicit: an empty section and a
// deliberate "no privileges required" section are indistinguishable in the
// rendered docs, and the difference matters to an operator deciding what to
// grant. If Jamf ever starts gating the tenant identifier read, this test fails
// and the description stops claiming otherwise.
func TestDataSourcePrivileges_RendersTheNoPrivilegeVariant(t *testing.T) {
	if dataSourcePrivileges == "" {
		t.Fatal("dataSourcePrivileges rendered empty; the method resolved to no registry entry at all")
	}
	if !strings.Contains(dataSourcePrivileges, "None") {
		t.Fatalf("expected the no-privilege variant, got:\n%s", dataSourcePrivileges)
	}
}

// TestGetCsaTenantIdV1_RequiresNoPrivileges reads the claim straight off the SDK
// registry, so the description's "any authenticated integration" wording rests
// on the SDK rather than on this package's recollection of it.
func TestGetCsaTenantIdV1_RequiresNoPrivileges(t *testing.T) {
	mp, ok := pro.Privileges["GetCsaTenantIdV1"]
	if !ok {
		t.Fatal("GetCsaTenantIdV1 absent from pro.Privileges")
	}
	if len(mp.Scoped) != 0 {
		t.Errorf("GetCsaTenantIdV1 now declares privileges %v; the data source description says it needs none", mp.Scoped)
	}
}
