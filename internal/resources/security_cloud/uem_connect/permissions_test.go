// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchReadCalls fails if crud.go calls an SDK
// method not declared in resourceSDKMethods, or declares one it does not
// call — keeping the privileges table honest as the Read path changes.
func TestResourceSDKMethods_MatchReadCalls(t *testing.T) {
	src, err := os.ReadFile("crud.go")
	if err != nil {
		t.Fatalf("reading crud.go: %v", err)
	}
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		// Only count calls that resolve to a known SDK method so that
		// unrelated client.<helper> calls do not pollute the comparison.
		if _, ok := securitycloud.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	declared := map[string]bool{}
	for _, m := range resourceSDKMethods {
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
		t.Errorf("crud.go calls SDK methods missing from resourceSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("resourceSDKMethods declares methods crud.go does not call: %v", uncalled)
	}
}

// TestPrivileges_Rendered guards that each construct's table rendered the
// uem-connect row *and* the action that construct actually performs. Asserting
// the action — not just the capability — is what makes this a drift guard: a row
// that ticked the wrong boxes would still contain the capability name.
func TestPrivileges_Rendered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		scoped   string
	}{
		{"resourcePrivileges", resourcePrivileges, "uem-connect:create"},
		{"dataSourcePrivileges", dataSourcePrivileges, "uem-connect:read"},
		{"listResourcePrivileges", listResourcePrivileges, "uem-connect:read"},
	} {
		if !permissions.Renders(tc.rendered, tc.scoped) {
			t.Errorf("%s did not render %s:\n%s", tc.name, tc.scoped, tc.rendered)
		}
	}
}

// TestDataSourceSDKMethods_KnownToSDK is the same drift guard for the data source.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls keeps the data source's declared method
// list honest as its Read path changes.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	src, err := os.ReadFile("data_source.go")
	if err != nil {
		t.Fatalf("reading data_source.go: %v", err)
	}
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if _, ok := securitycloud.Privileges[m[1]]; ok {
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

// TestRedundantSDKMethodsAreNotCalled pins the two deliberate omissions recorded in
// crud.go. Both are wire-verified duplicates of methods this resource already
// calls, and adding a call to either would double a request for no gain — or, in
// DisableUemConnectorV1's case, split one idempotent code path into two.
func TestRedundantSDKMethodsAreNotCalled(t *testing.T) {
	for _, file := range []string{"crud.go", "data_source.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for _, method := range []string{"GetUemConnectorSyncSettingsV1", "DisableUemConnectorV1"} {
			if strings.Contains(string(src), "client."+method+"(") {
				t.Errorf("%s calls %s, which crud.go documents as a redundant duplicate", file, method)
			}
		}
	}
}
