// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package deviceactions

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// sdkCallRe extracts SDK client method calls on an action's typed client field
// (a.actions.<Method>(...)). Actions invoke the deviceactions client via the
// embedded deviceAction.actions field, so the receiver pattern is a.actions
// rather than the resource-style client/r.client receiver.
var sdkCallRe = regexp.MustCompile(`\ba\.actions\.([A-Za-z0-9]+)\(`)

// matchActionCalls reads an action's source file and asserts the set of
// a.actions.<Method> calls it makes equals the declared SDK method list, after
// filtering to methods known to the deviceactions registry. This keeps each
// action's privileges table honest as its Invoke path changes.
func matchActionCalls(t *testing.T, filename string, declared []string) {
	t.Helper()

	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}

	called := map[string]bool{}
	for _, m := range sdkCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		// Only consider names the SDK registry knows about, so unrelated
		// field accesses can never poison the comparison.
		if _, ok := deviceactions.Privileges[name]; ok {
			called[name] = true
		}
	}

	declaredSet := map[string]bool{}
	for _, m := range declared {
		declaredSet[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !declaredSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declaredSet {
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
		t.Errorf("declared SDK method list has methods %s does not call: %v", filename, uncalled)
	}
}

// --- Erase ---

// TestEraseDeviceSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestEraseDeviceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(deviceactions.Privileges, eraseDeviceSDKMethods...); len(missing) > 0 {
		t.Fatalf("eraseDeviceSDKMethods not present in deviceactions.Privileges (SDK drift): %v", missing)
	}
}

// TestEraseDeviceSDKMethods_MatchInvokeCalls fails if erase.go calls an SDK
// method not declared in eraseDeviceSDKMethods, or declares one it does not
// call.
func TestEraseDeviceSDKMethods_MatchInvokeCalls(t *testing.T) {
	matchActionCalls(t, "erase.go", eraseDeviceSDKMethods)
}

// --- Restart ---

func TestRestartDeviceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(deviceactions.Privileges, restartDeviceSDKMethods...); len(missing) > 0 {
		t.Fatalf("restartDeviceSDKMethods not present in deviceactions.Privileges (SDK drift): %v", missing)
	}
}

func TestRestartDeviceSDKMethods_MatchInvokeCalls(t *testing.T) {
	matchActionCalls(t, "restart.go", restartDeviceSDKMethods)
}

// --- Shutdown ---

func TestShutdownDeviceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(deviceactions.Privileges, shutdownDeviceSDKMethods...); len(missing) > 0 {
		t.Fatalf("shutdownDeviceSDKMethods not present in deviceactions.Privileges (SDK drift): %v", missing)
	}
}

func TestShutdownDeviceSDKMethods_MatchInvokeCalls(t *testing.T) {
	matchActionCalls(t, "shutdown.go", shutdownDeviceSDKMethods)
}

// --- Unmanage ---

func TestUnmanageDeviceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(deviceactions.Privileges, unmanageDeviceSDKMethods...); len(missing) > 0 {
		t.Fatalf("unmanageDeviceSDKMethods not present in deviceactions.Privileges (SDK drift): %v", missing)
	}
}

func TestUnmanageDeviceSDKMethods_MatchInvokeCalls(t *testing.T) {
	matchActionCalls(t, "unmanage.go", unmanageDeviceSDKMethods)
}

// TestActionPrivileges_Rendered guards that the tables actually rendered into
// the action descriptions (catches an empty/parse-skipped registry). Erase and
// unmanage carry the destructive privilege; restart and shutdown carry the
// ordinary one.
func TestActionPrivileges_Rendered(t *testing.T) {
	for name, tc := range map[string]struct {
		rendered string
		want     string
	}{
		"eraseDevicePrivileges":    {eraseDevicePrivileges, "destructive-device-actions:execute"},
		"restartDevicePrivileges":  {restartDevicePrivileges, "device-actions:execute"},
		"shutdownDevicePrivileges": {shutdownDevicePrivileges, "device-actions:execute"},
		"unmanageDevicePrivileges": {unmanageDevicePrivileges, "destructive-device-actions:execute"},
	} {
		if !permissions.Renders(tc.rendered, tc.want) {
			t.Errorf("%s did not render %q:\n%s", name, tc.want, tc.rendered)
		}
	}
}
