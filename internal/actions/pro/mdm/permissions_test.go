// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"os"
	"regexp"
	"sort"
	"strings"
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

func TestLostModeSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(deviceActionsCommandRegistry, deviceActionsCommandSDKMethods...); len(missing) > 0 {
		t.Fatalf("deviceActionsCommandSDKMethods not present in the narrowed registry (drift): %v", missing)
	}
}

func TestSetWallpaperSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(wallpaperRegistry, setWallpaperSDKMethods...); len(missing) > 0 {
		t.Fatalf("setWallpaperSDKMethods not present in the registries (drift): %v", missing)
	}
}

func TestTranche3SDKMethods_KnownToSDK(t *testing.T) {
	for name, c := range map[string]struct {
		reg     permissions.Registry
		methods []string
	}{
		"clear_passcode":          {deviceActionsCommandRegistry, clearPasscodeSDKMethods},
		"recovery lock":           {recoveryLockRegistry, recoveryLockSDKMethods},
		"set_auto_admin_password": {localAdminPasswordRegistry, setAutoAdminPasswordSDKMethods},
		"destructive commands":    {destructiveCommandRegistry, destructiveCommandSDKMethods},
	} {
		if missing := permissions.Missing(c.reg, c.methods...); len(missing) > 0 {
			t.Errorf("%s: SDK methods not present in the narrowed registry (drift): %v", name, missing)
		}
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
	"resolveManagementIDs":           {"ListDevices"}, // serial path -> resolveSerialNumbers -> ListDevices (RSQL in= set)
	"resolveSerialNumbers":           {"ListDevices"},
	"resolveSerialsForManagementIDs": {"ListDevices"},
	"sendCommandBatch":               {"SendMdmCommandV2"},
	"sendCommand":                    {"SendMdmCommandV2"},
	"resolveManagementID":            {"ListDevices"},
	"resolveUnlockToken":             {"ListMobileDevicesDetailV2", "GetMobileDeviceDetailV2"},
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

func TestLostModeSDKMethods_MatchFile(t *testing.T) {
	for _, file := range []string{"enable_lost_mode.go", "disable_lost_mode.go", "play_lost_mode_sound.go"} {
		assertMatch(t, file, deviceActionsCommandSDKMethods)
	}
}

func TestSetWallpaperSDKMethods_MatchFile(t *testing.T) {
	assertMatch(t, "set_wallpaper.go", setWallpaperSDKMethods)
}

func TestTranche3SDKMethods_MatchFile(t *testing.T) {
	assertMatch(t, "clear_passcode.go", clearPasscodeSDKMethods)
	assertMatch(t, "set_recovery_lock.go", recoveryLockSDKMethods)
	assertMatch(t, "clear_recovery_lock.go", recoveryLockSDKMethods)
	assertMatch(t, "set_auto_admin_password.go", setAutoAdminPasswordSDKMethods)
	assertMatch(t, "lock_device.go", destructiveCommandSDKMethods)
	assertMatch(t, "delete_user.go", destructiveCommandSDKMethods)
}

func TestSimpleCommandActions_MatchFile(t *testing.T) {
	for _, file := range []string{
		"enable_remote_desktop.go", "disable_remote_desktop.go", "clear_restrictions_password.go",
		"log_out_user.go", "unlock_user_account.go", "trigger_enhanced_log_collection.go",
		"cancel_enhanced_log_collection.go",
	} {
		assertMatch(t, file, deviceActionsCommandSDKMethods)
	}
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

// TestLostModePrivileges_NameOnlyTheCapabilityTheCommandNeeds is the guard that
// the narrowing took effect. SendMdmCommandV2's registry entry lists four
// alternatives; a table that rendered them all would send an operator to grant
// recovery lock and destructive device actions for a command that needs neither.
func TestLostModePrivileges_NameOnlyTheCapabilityTheCommandNeeds(t *testing.T) {
	sections := map[string]string{
		"enable_lost_mode":     enableLostModePrivileges,
		"disable_lost_mode":    disableLostModePrivileges,
		"play_lost_mode_sound": playLostModeSoundPrivileges,
	}
	for name, section := range sections {
		for _, want := range []string{"device-actions:execute", "devices:read"} {
			if !permissions.Renders(section, want) {
				t.Errorf("%s did not render %q:\n%s", name, want, section)
			}
		}
		for _, banned := range []string{"destructive-device-actions", "local-admin-passwords", "recovery-lock", "at least one of"} {
			if strings.Contains(section, banned) {
				t.Errorf("%s still mentions %q, so the alternatives set was not narrowed:\n%s", name, banned, section)
			}
		}
	}
}

func TestSetWallpaperPrivileges_Rendered(t *testing.T) {
	for _, want := range []string{"destructive-device-actions:execute", "devices:read"} {
		if !permissions.Renders(setWallpaperPrivileges, want) {
			t.Errorf("setWallpaperPrivileges did not render %q:\n%s", want, setWallpaperPrivileges)
		}
	}
}

// TestCommandPrivileges_NameOnlyTheirOwnCapability pins each action to the one
// capability the gateway checks for its command type, and to none of the other
// three the endpoint's alternatives set offers.
func TestCommandPrivileges_NameOnlyTheirOwnCapability(t *testing.T) {
	all := []string{"device-actions", "destructive-device-actions", "local-admin-passwords", "recovery-lock"}
	cases := map[string]struct {
		section string
		want    string
		other   []string
	}{
		"clear_passcode":                  {clearPasscodePrivileges, "device-actions:execute", []string{"mobile-devices:read"}},
		"set_recovery_lock":               {setRecoveryLockPrivileges, "recovery-lock:execute", nil},
		"clear_recovery_lock":             {clearRecoveryLockPrivileges, "recovery-lock:execute", nil},
		"set_auto_admin_password":         {setAutoAdminPasswordPrivileges, "local-admin-passwords:execute", nil},
		"enable_remote_desktop":           {enableRemoteDesktopPrivileges, "device-actions:execute", nil},
		"disable_remote_desktop":          {disableRemoteDesktopPrivileges, "device-actions:execute", nil},
		"clear_restrictions_password":     {clearRestrictionsPasswordPrivileges, "device-actions:execute", nil},
		"log_out_user":                    {logOutUserPrivileges, "device-actions:execute", nil},
		"unlock_user_account":             {unlockUserAccountPrivileges, "device-actions:execute", nil},
		"trigger_enhanced_log_collection": {triggerEnhancedLogCollectionPrivileges, "device-actions:execute", nil},
		"cancel_enhanced_log_collection":  {cancelEnhancedLogCollectionPrivileges, "device-actions:execute", nil},
		"lock_device":                     {lockDevicePrivileges, "destructive-device-actions:execute", nil},
		"delete_user":                     {deleteUserPrivileges, "destructive-device-actions:execute", nil},
	}
	for name, c := range cases {
		if !permissions.Renders(c.section, c.want) {
			t.Errorf("%s did not render %q:\n%s", name, c.want, c.section)
		}
		if !permissions.Renders(c.section, "devices:read") {
			t.Errorf("%s did not render devices:read:\n%s", name, c.section)
		}
		wantCap, _, _ := strings.Cut(c.want, ":")
		for _, capability := range all {
			if capability != wantCap && strings.Contains(c.section, "`"+capability+"`") {
				t.Errorf("%s names %s, which its command does not need:\n%s", name, capability, c.section)
			}
		}
		if strings.Contains(c.section, "at least one of") {
			t.Errorf("%s still renders an alternatives set:\n%s", name, c.section)
		}
	}
}
