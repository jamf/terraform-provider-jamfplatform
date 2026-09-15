// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRE matches an SDK client method call on the embedded
// appInstallerAction.client field: calls read a.client.<Method>(.
var clientCallRE = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// assertMatch fails when the registry-known SDK calls in filename do not exactly
// equal declared — keeping each action's privileges table honest as its Invoke
// path changes.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}

	want := map[string]bool{}
	for _, m := range declared {
		want[m] = true
	}

	// Restrict to names known to the SDK registry so the regex does not trip on
	// unrelated client.* helper calls.
	called := map[string]bool{}
	for _, m := range clientCallRE.FindAllStringSubmatch(string(src), -1) {
		if len(permissions.Missing(pro.Privileges, m[1])) == 0 {
			called[m[1]] = true
		}
	}

	var undeclared, uncalled []string
	for m := range called {
		if !want[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range want {
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

func TestRetryInstallationsSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, retryInstallationsSDKMethods...); len(missing) > 0 {
		t.Fatalf("retryInstallationsSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestRetryInstallationsSDKMethods_MatchInvokeCalls(t *testing.T) {
	assertMatch(t, "retry_installations.go", retryInstallationsSDKMethods)
}

func TestRetryInstallationsPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(retryInstallationsPrivileges, "applications:update") {
		t.Fatalf("retryInstallationsPrivileges did not render the App Installer privileges:\n%s", retryInstallationsPrivileges)
	}
}

func TestRetryAllInstallationsSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, retryAllInstallationsSDKMethods...); len(missing) > 0 {
		t.Fatalf("retryAllInstallationsSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestRetryAllInstallationsSDKMethods_MatchInvokeCalls(t *testing.T) {
	assertMatch(t, "retry_all_installations.go", retryAllInstallationsSDKMethods)
}

func TestRetryAllInstallationsPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(retryAllInstallationsPrivileges, "applications:update") {
		t.Fatalf("retryAllInstallationsPrivileges did not render the App Installer privileges:\n%s", retryAllInstallationsPrivileges)
	}
}

func TestUpdateVersionSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, updateVersionSDKMethods...); len(missing) > 0 {
		t.Fatalf("updateVersionSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestUpdateVersionSDKMethods_MatchInvokeCalls(t *testing.T) {
	assertMatch(t, "update_version.go", updateVersionSDKMethods)
}

func TestUpdateVersionPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(updateVersionPrivileges, "applications:update") {
		t.Fatalf("updateVersionPrivileges did not render the App Installer privileges:\n%s", updateVersionPrivileges)
	}
}
