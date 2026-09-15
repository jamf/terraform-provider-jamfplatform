// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"sort"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestErrorCodes pins each code against the body captured during the wire probes.
// codeNotEntitled is aliased from securitycloud.ApiErrorItemCode, so this asserts
// the alias resolves to the spelling the wire actually sent; the rest have no
// generated constant to key on, and a typo in those would only show up as a
// diagnostic that silently never fires. enum_literals_test.go keeps the split
// honest.
func TestErrorCodes(t *testing.T) {
	want := map[string]string{
		"codeNotEntitled":                "NOT_ENTITLED",
		"codeConnectorDisabled":          "CONNECTOR_DISABLED",
		"codeConnectorNotConnected":      "CONNECTOR_NOT_CONNECTED",
		"codeConnectorMisconfigured":     "CONNECTOR_MISCONFIGURED",
		"codeActivationProfileNotFound":  "ACTIVATION_PROFILE_NOT_FOUND",
		"codeMultipleActivationProfiles": "MULTIPLE_ACTIVATION_PROFILES",
		"codeValidationFailed":           "VALIDATION_FAILED",
	}
	got := map[string]string{
		"codeNotEntitled":                codeNotEntitled,
		"codeConnectorDisabled":          codeConnectorDisabled,
		"codeConnectorNotConnected":      codeConnectorNotConnected,
		"codeConnectorMisconfigured":     codeConnectorMisconfigured,
		"codeActivationProfileNotFound":  codeActivationProfileNotFound,
		"codeMultipleActivationProfiles": codeMultipleActivationProfiles,
		"codeValidationFailed":           codeValidationFailed,
	}

	for name, value := range got {
		if value != want[name] {
			t.Errorf("%s = %q, want %q", name, value, want[name])
		}
	}
}

// TestOSToWire_CoversEveryPlatformTheSDKKnows is the coverage guard the naming
// review asks for: the accepted values are derived from osToWire, so a platform the
// SDK gains with no entry here would silently become unreachable rather than
// failing anything.
func TestOSToWire_CoversEveryPlatformTheSDKKnows(t *testing.T) {
	mapped := map[string]string{}
	for value, wire := range osToWire {
		if existing, ok := mapped[wire]; ok {
			t.Errorf("wire value %q is mapped from both %q and %q", wire, existing, value)
		}
		mapped[wire] = value
	}

	for _, wire := range securitycloud.ActivationProfileDeployRequestPlatformValues() {
		if _, ok := mapped[wire]; !ok {
			t.Errorf("the SDK accepts platform %q but osToWire has no value for it", wire)
		}
	}
	if len(mapped) != len(securitycloud.ActivationProfileDeployRequestPlatformValues()) {
		t.Errorf("osToWire maps %d wire values, the SDK accepts %d — a value has outlived its platform",
			len(mapped), len(securitycloud.ActivationProfileDeployRequestPlatformValues()))
	}
}

// TestOSToWire_Labels pins the four pairings against the admin UI's "Select your
// OS" tiles, read off the activation profile deployment page on 2026-08-29.
//
// A round-trip test would pass with any consistent pairing, so the pairings are
// spelled out. macos → SUPERVISED_MAC is the one worth having written down: the UI
// tile says only "macOS" and never mentions supervision, so the pairing looks wrong
// until you know it came off a screen.
func TestOSToWire_Labels(t *testing.T) {
	want := map[string]string{
		"ios_supervised":   securitycloud.ActivationProfileDeployRequestPlatformSupervisedIos,
		"ios_unsupervised": securitycloud.ActivationProfileDeployRequestPlatformUnsupervisedIos,
		"ios_byod":         securitycloud.ActivationProfileDeployRequestPlatformByodIos,
		"macos":            securitycloud.ActivationProfileDeployRequestPlatformSupervisedMac,
	}

	for value, wire := range want {
		got, ok := osToWire[value]
		if !ok {
			t.Errorf("osToWire has no entry for %q", value)
			continue
		}
		if got != wire {
			t.Errorf("osToWire[%q] = %q, want %q", value, got, wire)
		}
	}
}

// TestOSValues_SortedAndComplete pins that the validator and the documented list —
// both built from osValues — agree with the map and with each other.
func TestOSValues_SortedAndComplete(t *testing.T) {
	values := osValues()

	if len(values) != len(osToWire) {
		t.Fatalf("osValues returned %d values, osToWire has %d", len(values), len(osToWire))
	}
	if !sort.StringsAreSorted(values) {
		t.Errorf("osValues is not sorted: %v", values)
	}
	for _, value := range values {
		if _, ok := osToWire[value]; !ok {
			t.Errorf("osValues returned %q, which osToWire does not map", value)
		}
	}
}

// TestMacOSValue_IsMapped pins the constant the group-kind diagnostic branches on.
// Get it wrong and the diagnostic names computer groups for iOS and vice versa,
// which is worse than the server's own message.
func TestMacOSValue_IsMapped(t *testing.T) {
	wire, ok := osToWire[macOSValue]
	if !ok {
		t.Fatalf("macOSValue = %q, which osToWire does not map", macOSValue)
	}
	if wire != securitycloud.ActivationProfileDeployRequestPlatformSupervisedMac {
		t.Errorf("macOSValue maps to %q, want the SDK's Mac platform", wire)
	}
}

// TestUEMJamfPro pins the only UEM value the deploy accepts. Wire-verified: any
// other value is refused with "not one of the values accepted for Enum class:
// [JAMF]".
func TestUEMJamfPro(t *testing.T) {
	if uemJamfPro != "JAMF" {
		t.Errorf("uemJamfPro = %q, want %q", uemJamfPro, "JAMF")
	}
}

// TestJamfProGroupIDPattern covers the plan-time refusal that keeps the server's
// unhelpful 422 out of the picture — including the mistake it exists for, the
// `computer_30` spelling the sibling resource's group mapping uses.
func TestJamfProGroupIDPattern(t *testing.T) {
	accept := []string{"1", "30", "15622", "0"}
	reject := []string{"", "computer_30", "mobile_20", "-1", "1.0", " 1", "1 ", "1a", "all"}

	for _, value := range accept {
		if !jamfProGroupIDPattern.MatchString(value) {
			t.Errorf("group ID %q was rejected, want accepted", value)
		}
	}
	for _, value := range reject {
		if jamfProGroupIDPattern.MatchString(value) {
			t.Errorf("group ID %q was accepted, want rejected", value)
		}
	}
}

// TestInvalidGroupIDMarker pins the substring against the description Jamf Security
// Cloud actually sent, because VALIDATION_FAILED is a catch-all and the marker is
// what stops the group-ID diagnostic claiming an unrelated validation failure.
func TestInvalidGroupIDMarker(t *testing.T) {
	sent := "Invalid scoping group ID (expected integer): 'computer_29'"
	if !strings.Contains(sent, invalidGroupIDMarker) {
		t.Errorf("invalidGroupIDMarker %q does not appear in the wire description %q",
			invalidGroupIDMarker, sent)
	}

	enumFailure := "JSON parse error: Cannot deserialize value of type " +
		"`…ActivationProfilePlatform` from String \"NOPE\": not one of the values accepted for Enum class"
	if strings.Contains(enumFailure, invalidGroupIDMarker) {
		t.Errorf("invalidGroupIDMarker %q matches an enum validation failure, so the diagnostic would "+
			"claim a group problem for one", invalidGroupIDMarker)
	}
}
