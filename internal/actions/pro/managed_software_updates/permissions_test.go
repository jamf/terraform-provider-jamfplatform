// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls on the action's client field
// (a.client.<Method>( ). The leading receiver is captured loosely so the test
// is robust to the exact field/receiver name.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// sdkCallsIn returns the distinct SDK client method calls in the named source
// file that are known to the pro privilege registry. Filtering to known methods
// keeps the comparison honest: the action also calls non-SDK client methods
// (e.g. resp helpers) that are not privilege-bearing.
func sdkCallsIn(t *testing.T, filename string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	return called
}

// assertMatch fails if the source file calls an SDK method not declared, or
// declares one it does not call — keeping the privileges table honest as the
// action's Invoke path changes.
func assertMatch(t *testing.T, filename string, declaredList []string) {
	t.Helper()
	called := sdkCallsIn(t, filename)
	declared := map[string]bool{}
	for _, m := range declaredList {
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
		t.Errorf("%s calls SDK methods missing from declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

// TestAbandonFeatureToggleSDKMethods_KnownToSDK fails if a declared method has
// been renamed or removed in the SDK privilege registry.
func TestAbandonFeatureToggleSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, abandonFeatureToggleSDKMethods...); len(missing) > 0 {
		t.Fatalf("abandonFeatureToggleSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestAbandonFeatureToggleSDKMethods_MatchCalls fails if abandon.go calls an SDK
// method not declared in abandonFeatureToggleSDKMethods, or declares one it does
// not call.
func TestAbandonFeatureToggleSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "abandon.go", abandonFeatureToggleSDKMethods)
}

// TestAbandonFeatureTogglePrivileges_Rendered guards that the table actually
// rendered into the action description.
func TestAbandonFeatureTogglePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(abandonFeatureTogglePrivileges, "managed-software-updates:update") {
		t.Fatalf("abandonFeatureTogglePrivileges did not render the managed-software-updates privileges:\n%s", abandonFeatureTogglePrivileges)
	}
}

// TestPlanSDKMethods_KnownToSDK fails if a declared method has been renamed or
// removed in the SDK privilege registry.
func TestPlanSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, planSDKMethods...); len(missing) > 0 {
		t.Fatalf("planSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestPlanSDKMethods_MatchCalls fails if plan.go calls an SDK method not
// declared in planSDKMethods, or declares one it does not call.
func TestPlanSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "plan.go", planSDKMethods)
}

// TestPlanPrivileges_Rendered guards that the table actually rendered into the
// action description.
func TestPlanPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(planPrivileges, "managed-software-updates:create") {
		t.Fatalf("planPrivileges did not render the managed-software-updates privileges:\n%s", planPrivileges)
	}
}
