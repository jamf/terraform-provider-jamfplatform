// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"os"
	"regexp"
	"sort"
	"testing"

	devSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// --- SDK drift guards: every declared method must exist in its registry ---

func TestSendBlankPushSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(resolveSerialMerged, sendBlankPushSDKMethods...); len(missing) > 0 {
		t.Fatalf("sendBlankPushSDKMethods not present in SDK registries (drift): %v", missing)
	}
}

func TestRenewMdmProfileSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, renewMdmProfileSDKMethods...); len(missing) > 0 {
		t.Fatalf("renewMdmProfileSDKMethods not present in the pro registry (drift): %v", missing)
	}
}

func TestFlushMdmCommandsSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(proclassic.Privileges, flushMdmCommandsSDKMethods...); len(missing) > 0 {
		t.Fatalf("flushMdmCommandsSDKMethods not present in the proclassic registry (drift): %v", missing)
	}
}

// --- file <-> declared-method reachability guards ---
//
// Each MDM command action reaches the SDK two ways: via direct calls on its
// configured client surfaces (a.client.*, a.devices.*, a.classic.*) and via the
// shared helpers in helpers.go (resolveManagementIDs' serial path ->
// resolveSerialNumbers -> ListDevices).
//
// reachableSDKMethods reads an action's own source file, resolves the helper
// indirections it invokes, and returns the full set of SDK privilege-registry
// method names it actually exercises — the honest input to the privileges
// table. The per-action test asserts this equals the declared list.

// helperSDKMethods maps a shared helper to the registry method names it calls.
// ResolveDeviceIDBySerialNumber is intentionally surfaced as "ListDevices": the
// resolver has no registry entry of its own and hits the same /v1/devices list
// endpoint, so its required privilege is that of ListDevices.
var helperSDKMethods = map[string][]string{
	"resolveManagementIDs": {"ListDevices"}, // serial path -> resolveSerialNumbers -> ListDevices (RSQL in= set)
	"resolveSerialNumbers": {"ListDevices"},
}

// directSDKAliases maps SDK methods callable directly in an action file to their
// registry method names (identity for most; the serial resolver folds onto
// ListDevices for the privilege it actually requires).
var directSDKAliases = map[string]string{
	"ResolveDeviceIDBySerialNumber": "ListDevices",
}

// knownRegistryMethod reports whether name is a method tracked by any of the
// SDK registries this package draws on.
func knownRegistryMethod(name string) bool {
	if _, ok := pro.Privileges[name]; ok {
		return true
	}
	if _, ok := devSDK.Privileges[name]; ok {
		return true
	}
	if _, ok := proclassic.Privileges[name]; ok {
		return true
	}
	return false
}

func reachableSDKMethods(t *testing.T, filename string) []string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	text := string(src)

	set := map[string]bool{}

	// Direct SDK client calls: a.client.X(, a.devices.X(, a.classic.X(.
	directRe := regexp.MustCompile(`\ba\.(?:client|devices|classic)\.([A-Za-z0-9]+)\(`)
	for _, m := range directRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if alias, ok := directSDKAliases[name]; ok {
			name = alias
		}
		if knownRegistryMethod(name) {
			set[name] = true
		}
	}

	// Helper indirections: a.<helper>(.
	helperRe := regexp.MustCompile(`\ba\.([A-Za-z0-9]+)\(`)
	for _, m := range helperRe.FindAllStringSubmatch(text, -1) {
		for _, name := range helperSDKMethods[m[1]] {
			set[name] = true
		}
	}

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	got := reachableSDKMethods(t, filename)
	want := append([]string(nil), declared...)
	sort.Strings(want)

	gotSet := map[string]bool{}
	for _, m := range got {
		gotSet[m] = true
	}
	wantSet := map[string]bool{}
	for _, m := range want {
		wantSet[m] = true
	}

	var undeclared, unused []string
	for m := range gotSet {
		if !wantSet[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range wantSet {
		if !gotSet[m] {
			unused = append(unused, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(unused)

	if len(undeclared) > 0 {
		t.Errorf("%s reaches SDK methods missing from declared list: %v", filename, undeclared)
	}
	if len(unused) > 0 {
		t.Errorf("declared list has methods %s does not reach: %v", filename, unused)
	}
}

func TestSendBlankPushSDKMethods_MatchFile(t *testing.T) {
	assertMatch(t, "send_blank_push.go", sendBlankPushSDKMethods)
}

func TestRenewMdmProfileSDKMethods_MatchFile(t *testing.T) {
	assertMatch(t, "renew_mdm_profile.go", renewMdmProfileSDKMethods)
}

func TestFlushMdmCommandsSDKMethods_MatchFile(t *testing.T) {
	assertMatch(t, "flush_mdm_commands.go", flushMdmCommandsSDKMethods)
}

// --- rendered-table guards: each section actually produced a privileges block ---

func TestSendBlankPushPrivileges_Rendered(t *testing.T) {
	// Both privileges, deliberately. devices:read comes from ListDevices, the
	// serial-resolution helper, so asserting it alone would leave the privilege
	// the command itself needs unpinned — and an operator who granted only the
	// resolver's privilege would be refused at POST /v2/mdm/blank-push by a
	// table that told them they were done.
	for _, want := range []string{"device-actions:execute", "devices:read"} {
		if !permissions.Renders(sendBlankPushPrivileges, want) {
			t.Errorf("sendBlankPushPrivileges did not render %q:\n%s", want, sendBlankPushPrivileges)
		}
	}
}

func TestRenewMdmProfilePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(renewMdmProfilePrivileges, "device-actions:execute") {
		t.Fatalf("renewMdmProfilePrivileges did not render the expected privilege:\n%s", renewMdmProfilePrivileges)
	}
}

func TestFlushMdmCommandsPrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(flushMdmCommandsPrivileges, "device-actions:delete") {
		t.Fatalf("flushMdmCommandsPrivileges did not render the expected privilege:\n%s", flushMdmCommandsPrivileges)
	}
}
