// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patchactions

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestRetryPatchPolicyLogsSDKMethods_KnownToSDK fails if a declared method has
// been renamed or removed in the SDK privilege registry.
func TestRetryPatchPolicyLogsSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, retryPatchPolicyLogsSDKMethods...); len(missing) > 0 {
		t.Fatalf("retryPatchPolicyLogsSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

// TestRetryPatchPolicyLogsSDKMethods_MatchInvokeCalls fails if
// retry_patch_policy_logs.go calls an SDK method not declared in
// retryPatchPolicyLogsSDKMethods, or declares one it does not call — keeping the
// privileges table honest as the action's Invoke path changes.
func TestRetryPatchPolicyLogsSDKMethods_MatchInvokeCalls(t *testing.T) {
	src, err := os.ReadFile("retry_patch_policy_logs.go")
	if err != nil {
		t.Fatalf("reading retry_patch_policy_logs.go: %v", err)
	}
	// The action holds its SDK client in the embedded patchAction.client field,
	// so calls read a.client.<Method>( — match the client.<Method>( tail.
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	declared := map[string]bool{}
	for _, m := range retryPatchPolicyLogsSDKMethods {
		declared[m] = true
	}
	// Restrict to method names known to the SDK registry so the regex does not
	// trip on unrelated client.* helper calls.
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if len(permissions.Missing(pro.Privileges, name)) == 0 {
			called[name] = true
		}
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
		t.Errorf("retry_patch_policy_logs.go calls SDK methods missing from retryPatchPolicyLogsSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("retryPatchPolicyLogsSDKMethods declares methods retry_patch_policy_logs.go does not call: %v", uncalled)
	}
}

// TestRetryPatchPolicyLogsPrivileges_Rendered guards that the table actually
// rendered into the action description (catches an empty/parse-skipped registry).
func TestRetryPatchPolicyLogsPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(retryPatchPolicyLogsPrivileges, "patch-policies:update") {
		t.Fatalf("retryPatchPolicyLogsPrivileges did not render the patch-policies privileges:\n%s", retryPatchPolicyLogsPrivileges)
	}
}
