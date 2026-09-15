// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact_alert_notification_settings

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls on either the resource (r.client.<M>())
// or data source (d.client.<M>()) receiver.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsIn returns the distinct SDK client method names invoked in the named source
// file, filtered to those known to the SDK privilege registry. Filtering keeps the
// assertion robust against incidental matches that are not SDK methods.
func sdkCallsIn(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		if _, ok := pro.Privileges[name]; !ok {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	called := sdkCallsIn(t, filename)

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
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

// TestResourceSDKMethods_KnownToSDK fails if a declared method has been renamed or
// removed in the SDK privilege registry.
func TestResourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, resourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("resourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestResourceSDKMethods_MatchCRUDCalls fails if crud.go calls an SDK method not
// declared in resourceSDKMethods, or declares one it does not call.
func TestResourceSDKMethods_MatchCRUDCalls(t *testing.T) {
	assertMatch(t, "crud.go", resourceSDKMethods)
}

// TestResourcePrivileges_Rendered guards that the table actually rendered into the
// resource description (catches an empty/parse-skipped registry).
func TestResourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(resourcePrivileges, "impact-alert-notification-settings:update") {
		t.Fatalf("resourcePrivileges did not render the impact-alert-notification-settings privileges:\n%s", resourcePrivileges)
	}
}

// TestDataSourceSDKMethods_KnownToSDK fails if a declared method has been renamed or
// removed in the SDK privilege registry.
func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchReadCalls fails if data_source.go calls an SDK method
// not declared in dataSourceSDKMethods, or declares one it does not call.
func TestDataSourceSDKMethods_MatchReadCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

// TestDataSourcePrivileges_Rendered guards that the table actually rendered into the
// data source description.
func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "impact-alert-notification-settings:read") {
		t.Fatalf("dataSourcePrivileges did not render the impact-alert-notification-settings privileges:\n%s", dataSourcePrivileges)
	}
}
