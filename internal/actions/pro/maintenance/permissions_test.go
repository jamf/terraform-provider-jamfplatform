// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package maintenanceactions

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// callRe matches SDK client method calls regardless of the receiver field name
// (the maintenance actions call either a.client.<Method> or a.classic.<Method>).
// The results are filtered against the relevant SDK registry so that local /
// stdlib helper calls (fmt.Sprintf, resp.SendProgress, ...) are ignored.
var callRe = regexp.MustCompile(`\b\w+\.([A-Za-z0-9]+)\(`)

// calledSDKMethods scans the named source file for method calls present in the
// given registry, returning the distinct set.
func calledSDKMethods(t *testing.T, filename string, reg permissions.Registry) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	seen := map[string]bool{}
	for _, m := range callRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := reg[m[1]]; ok {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// assertMatch fails if the source file's SDK calls and the declared method list
// disagree in either direction.
func assertMatch(t *testing.T, declared, called []string) {
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
		t.Errorf("source calls SDK methods missing from the declared list: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list contains methods the source does not call: %v", uncalled)
	}
}

// TestFlushPolicyLogsSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestFlushPolicyLogsSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, flushPolicyLogsSDKMethods...); len(missing) > 0 {
		t.Fatalf("flushPolicyLogsSDKMethods not present in proclassic.Privileges (SDK drift): %v", missing)
	}
}

// TestFlushPolicyLogsSDKMethods_MatchCalls fails if flush_policy_logs.go calls
// an SDK method not declared in flushPolicyLogsSDKMethods, or declares one it
// does not call.
func TestFlushPolicyLogsSDKMethods_MatchCalls(t *testing.T) {
	called := calledSDKMethods(t, "flush_policy_logs.go", proclassic.Privileges)
	assertMatch(t, flushPolicyLogsSDKMethods, called)
}

// TestFlushPolicyLogsPrivileges_Rendered guards that the table actually rendered
// into the action description (catches an empty/parse-skipped registry).
func TestFlushPolicyLogsPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(flushPolicyLogsPrivileges, "flush-policy-logs:execute") {
		t.Fatalf("flushPolicyLogsPrivileges did not render the flush-policy-logs privileges:\n%s", flushPolicyLogsPrivileges)
	}
}

// TestRedeployManagementFrameworkSDKMethods_KnownToSDK fails if a declared
// method has been renamed or removed in the SDK privilege registry.
func TestRedeployManagementFrameworkSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, redeployManagementFrameworkSDKMethods...); len(missing) > 0 {
		t.Fatalf("redeployManagementFrameworkSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestRedeployManagementFrameworkSDKMethods_MatchCalls fails if
// redeploy_management_framework.go calls an SDK method not declared in
// redeployManagementFrameworkSDKMethods, or declares one it does not call.
func TestRedeployManagementFrameworkSDKMethods_MatchCalls(t *testing.T) {
	called := calledSDKMethods(t, "redeploy_management_framework.go", pro.Privileges)
	assertMatch(t, redeployManagementFrameworkSDKMethods, called)
}

// TestRedeployManagementFrameworkPrivileges_Rendered guards that the table
// actually rendered into the action description.
func TestRedeployManagementFrameworkPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(redeployManagementFrameworkPrivileges, "device-actions:execute") {
		t.Fatalf("redeployManagementFrameworkPrivileges did not render the redeploy privileges:\n%s", redeployManagementFrameworkPrivileges)
	}
}
