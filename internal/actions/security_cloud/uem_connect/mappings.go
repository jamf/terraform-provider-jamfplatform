// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"regexp"
	"sort"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes the UEM Connect actions translate. Wire-probed
// against production EU on 2026-08-28 (synchronize) and 2026-08-29 (deploy).
//
// NOT_ENTITLED comes from the SDK's generated enum. The rest are literals because
// securitycloud.ApiErrorItemCode is the DNS namespace's error schema and carries
// no UEM Connect code — checked value by value, not as a set, and pinned by
// enum_literals_test.go so a future SDK release promoting one of them fails a test
// rather than passing review.
const (
	codeNotEntitled                = securitycloud.ApiErrorItemCodeNotEntitled
	codeConnectorDisabled          = "CONNECTOR_DISABLED"
	codeConnectorNotConnected      = "CONNECTOR_NOT_CONNECTED"
	codeConnectorMisconfigured     = "CONNECTOR_MISCONFIGURED"
	codeActivationProfileNotFound  = "ACTIVATION_PROFILE_NOT_FOUND"
	codeMultipleActivationProfiles = "MULTIPLE_ACTIVATION_PROFILES"
	codeValidationFailed           = "VALIDATION_FAILED"
)

// uemJamfPro is the only UEM platform the deploy accepts.
//
// The admin UI offers thirteen under "Select your UEM solution", but the API
// refuses every one but Jamf Pro — wire-confirmed, since `INTUNE` comes back with
// "not one of the values accepted for Enum class: [JAMF]". An attribute with one
// legal value would be friction with nothing to choose, so this stays a constant
// and never reaches the schema. The uem_connect resource makes the same call with
// its own vendor constant.
//
// No generated constant exists to key on: the spec declares this enum inline on
// the request body rather than as a named schema, so the generator emits no type
// for it. That is why it is a literal here and why enum_literals_test.go names it.
const uemJamfPro = "JAMF"

// osToWire maps this action's `os` values to the ones Jamf Security Cloud accepts.
//
// Three vocabularies exist for the same four choices and none of them agree. The
// wire calls the field `platform`; the admin UI calls it "Select your OS" and never
// says "platform"; and the configuration profiles the deploy creates in Jamf Pro
// are named a third way again ("… | Supervised Mac"). The UI wins, per STYLE_GUIDE
// §Attribute names mirror the admin UI — including the half about a name that
// differs materially without being cryptic.
//
// The macOS entry is the one that earns the table on its own: the wire value says
// SUPERVISED_MAC, and the UI tile says only "macOS", with no mention of
// supervision.
//
//	os value            UI label ("Select your OS")               wire
//	ios_supervised      iOS and iPadOS supervised                 SUPERVISED_IOS
//	ios_unsupervised    iOS and iPadOS unsupervised               UNSUPERVISED_IOS
//	ios_byod            iOS and iPadOS for user enrollment/BYOD   BYOD_IOS
//	macos               macOS                                     SUPERVISED_MAC
//
// Keyed on the SDK's generated constants, so a renamed value fails the build rather
// than sending a string the server rejects. osValues() derives the accepted list
// from the SDK's own *Values() helper, so a value the SDK gains with no entry here
// fails a test instead of silently becoming unreachable.
var osToWire = map[string]string{
	"ios_supervised":   securitycloud.ActivationProfileDeployRequestPlatformSupervisedIos,
	"ios_unsupervised": securitycloud.ActivationProfileDeployRequestPlatformUnsupervisedIos,
	"ios_byod":         securitycloud.ActivationProfileDeployRequestPlatformByodIos,
	"macos":            securitycloud.ActivationProfileDeployRequestPlatformSupervisedMac,
}

// osValues returns the accepted `os` values, sorted, for the validator and the
// documented list.
//
// Both are built from this one function so a value can never appear in the
// description without being accepted, or vice versa.
func osValues() []string {
	values := make([]string, 0, len(osToWire))
	for value := range osToWire {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// macOSValue is the one `os` value that scopes to computer groups rather than
// mobile device groups. Named so the diagnostic for a wrong-type group id can say
// which kind was expected.
const macOSValue = "macos"

// jamfProGroupIDPattern is what a Jamf Pro group ID looks like on this field:
// digits, nothing else.
//
// Worth validating at plan time even though the server checks it, because the
// server's refusal is a 422 naming the value but not the field, and the mistake it
// catches is a natural one — the group mapping on the uem_connect resource takes
// the same idea spelled `computer_30`, and that spelling is rejected here.
var jamfProGroupIDPattern = regexp.MustCompile(`^[0-9]+$`)

// invalidGroupIDMarker is the fragment of Jamf Security Cloud's VALIDATION_FAILED
// description that identifies a malformed group ID.
//
// VALIDATION_FAILED is a catch-all this service uses for every body problem,
// including enum violations, so the code alone is not enough to translate on. The
// substring keeps the diagnostic from claiming a group-ID problem for an unrelated
// validation failure — a plan-time validator should mean neither ever reaches the
// server.
const invalidGroupIDMarker = "Invalid scoping group ID"
