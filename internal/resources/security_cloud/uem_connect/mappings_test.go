// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"slices"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestAutoDeleteBehaviourMappingIsBijective pins that the forward and reverse
// tables agree, which reverseMapping only guarantees when the forward table has no
// duplicate values.
func TestAutoDeleteBehaviourMappingIsBijective(t *testing.T) {
	if len(uemAutoDeleteBehaviourToWire) != len(uemAutoDeleteBehaviourFromWire) {
		t.Fatalf("forward table has %d entries, reverse has %d — two values collide",
			len(uemAutoDeleteBehaviourToWire), len(uemAutoDeleteBehaviourFromWire))
	}
	for local, wire := range uemAutoDeleteBehaviourToWire {
		if got := uemAutoDeleteBehaviourFromWire[wire]; got != local {
			t.Errorf("%q maps to %q, which maps back to %q", local, wire, got)
		}
	}
}

// TestAutoDeleteBehaviourCoversTheSDKSet fails when Jamf adds an auto-delete
// value, or renames one, leaving this resource unable to express it.
//
// This is the coverage guard the naming reference asks for: the label table is
// keyed on the SDK's constants, so a rename breaks the build, but a *new* value
// would compile fine and silently be unreachable.
func TestAutoDeleteBehaviourCoversTheSDKSet(t *testing.T) {
	covered := make(map[string]bool, len(uemAutoDeleteBehaviourFromWire))
	for wire := range uemAutoDeleteBehaviourFromWire {
		covered[wire] = true
	}

	var uncovered []string
	for _, wire := range securitycloud.SyncSettingsAutoDeviceDeletionValues() {
		if !covered[wire] {
			uncovered = append(uncovered, wire)
		}
	}
	if len(uncovered) > 0 {
		t.Errorf("auto-delete values with no attribute value: %v", uncovered)
	}

	known := make(map[string]bool)
	for _, wire := range securitycloud.SyncSettingsAutoDeviceDeletionValues() {
		known[wire] = true
	}
	for wire := range uemAutoDeleteBehaviourFromWire {
		if !known[wire] {
			t.Errorf("attribute value maps to %q, which the SDK no longer accepts", wire)
		}
	}
}

// TestAutoDeleteBehaviourValuesAreSorted pins the stable ordering the validator and
// the rendered documentation both depend on.
func TestAutoDeleteBehaviourValuesAreSorted(t *testing.T) {
	values := uemAutoDeleteBehaviourValues()
	if len(values) != len(uemAutoDeleteBehaviourToWire) {
		t.Fatalf("got %d values, want %d", len(values), len(uemAutoDeleteBehaviourToWire))
	}
	if !sort.StringsAreSorted(values) {
		t.Errorf("values are not sorted: %v", values)
	}
}

// TestDeviceFieldMappingVocabulariesAreSortedAndUnique guards the restated
// vocabularies against the two mistakes hand-maintained lists make: a duplicate,
// and an ordering that reshuffles a validator's error message between builds.
func TestDeviceFieldMappingVocabulariesAreSortedAndUnique(t *testing.T) {
	vocabularies := map[string][]string{
		"device_name":  deviceNameMappingValues,
		"user_name":    userNameMappingValues,
		"user_id":      userIDMappingValues,
		"phone_number": phoneNumberMappingValues,
		"email.source": userEmailMappingTypeValues,
	}

	for name, values := range vocabularies {
		t.Run(name, func(t *testing.T) {
			if len(values) == 0 {
				t.Fatal("vocabulary is empty")
			}
			if !sort.StringsAreSorted(values) {
				t.Errorf("not sorted: %v", values)
			}
			seen := map[string]bool{}
			for _, v := range values {
				if seen[v] {
					t.Errorf("duplicate value %q", v)
				}
				seen[v] = true
			}
		})
	}
}

// TestEmailSourceVocabularyIsASubsetOfTheSDKSet pins the relationship the SDK's
// own doc note describes: EmailMappingTypeValues() spans every UEM vendor, so the
// Jamf Pro set must be a strict subset of it. A value here that the SDK does not
// know would be a typo; the SDK carrying values this omits is expected, and the
// two it omits are named so a reader can see the omission is deliberate.
func TestEmailSourceVocabularyIsASubsetOfTheSDKSet(t *testing.T) {
	sdkSet := map[string]bool{}
	for _, v := range securitycloud.EmailMappingTypeValues() {
		sdkSet[v] = true
	}

	for _, v := range userEmailMappingTypeValues {
		if !sdkSet[v] {
			t.Errorf("%q is not in the SDK's EmailMappingType set", v)
		}
	}

	if len(userEmailMappingTypeValues) >= len(sdkSet) {
		t.Errorf("expected a strict subset: got %d values against the SDK's %d",
			len(userEmailMappingTypeValues), len(sdkSet))
	}

	// Wire-verified 2026-08-28: a Jamf Pro connector's 422 enumerates its
	// accepted set, and these two are absent from it.
	local := map[string]bool{}
	for _, v := range userEmailMappingTypeValues {
		local[v] = true
	}
	for _, rejected := range []string{
		securitycloud.EmailMappingTypeExternalUserID,
		securitycloud.EmailMappingTypeCustom,
	} {
		if local[rejected] {
			t.Errorf("%q is accepted by this resource but refused by a Jamf Pro connector", rejected)
		}
	}
}

// TestDocumentedDefaultsAreInTheirVocabularies catches a default that names a value
// the matching validator would reject — which would make the documented default
// unusable if written out explicitly.
func TestDocumentedDefaultsAreInTheirVocabularies(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		allowed []string
	}{
		{"device_name", defaultDeviceNameMapping, deviceNameMappingValues},
		{"user_name", defaultUserNameMapping, userNameMappingValues},
		{"user_id", defaultUserIDMapping, userIDMappingValues},
		{"phone_number", defaultPhoneNumberMapping, phoneNumberMappingValues},
		{"email.source", defaultUserEmailMappingType, userEmailMappingTypeValues},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if slices.Contains(tc.allowed, tc.value) {
				return
			}
			t.Errorf("default %q is not in the accepted set %v", tc.value, tc.allowed)
		})
	}
}

// TestDefaultAutoDeleteBehaviourIsAccepted pins the schema default against the
// validator that guards the same attribute.
func TestDefaultAutoDeleteBehaviourIsAccepted(t *testing.T) {
	if _, ok := uemAutoDeleteBehaviourToWire[defaultUEMAutoDeleteBehaviour]; !ok {
		t.Errorf("schema default %q has no wire equivalent", defaultUEMAutoDeleteBehaviour)
	}
}

// TestVendorIsJamfProOnly documents the deliberate narrowing: the SDK enumerates
// ten vendors and this resource accepts one, because everything downstream of
// vendor — five mapping vocabularies and the group identifier format — was
// established for Jamf Pro only.
//
// The set checked is the create envelope's discriminator, the only vocabulary that
// enumerates every vendor. The generic request enum this once had to be
// distinguished from is gone, the spec having given all ten vendors their own
// request schema.
func TestVendorIsJamfProOnly(t *testing.T) {
	if vendorJamfPro != "JAMF_PRO" {
		t.Errorf("vendorJamfPro = %q, want JAMF_PRO", vendorJamfPro)
	}

	vendors := securitycloud.ConnectorCreateRequestBodyVendorValues()
	if len(vendors) < 2 {
		t.Error("the SDK now enumerates fewer than two vendors; revisit whether the narrowing still makes sense")
	}
	if !slices.Contains(vendors, vendorJamfPro) {
		t.Errorf("the chosen vendor %q is not in the create envelope's vocabulary %v", vendorJamfPro, vendors)
	}
}

// TestErrorCodesPreferTheSDKEnum pins which of the translated error codes come from
// the SDK and which are string literals, and — more usefully — fails if the SDK
// grows a constant for one of the literals.
//
// The split is not a style choice: only NOT_ENTITLED is in the generated
// ApiErrorItemCode enum, because that enum is declared by the DNS namespace's spec
// and the UEM Connect spec declares no equivalent. A literal is the only option for
// the rest today, and this test is what notices when that stops being true.
func TestErrorCodesPreferTheSDKEnum(t *testing.T) {
	inSDK := map[string]bool{}
	for _, v := range securitycloud.ApiErrorItemCodeValues() {
		inSDK[v] = true
	}

	if codeNotEntitled != securitycloud.ApiErrorItemCodeNotEntitled {
		t.Errorf("codeNotEntitled = %q; it must be taken from the SDK constant", codeNotEntitled)
	}

	for name, code := range map[string]string{
		"codeConfigAlreadyExists": codeConfigAlreadyExists,
		"codeConnectionFailed":    codeConnectionFailed,
		"codeConnectorDisabled":   codeConnectorDisabled,
		"codeValidationFailed":    codeValidationFailed,
		"codeNotFound":            codeNotFound,
	} {
		if inSDK[code] {
			t.Errorf("%s is the literal %q, but the SDK now generates a constant for it — use the constant", name, code)
		}
	}
}
