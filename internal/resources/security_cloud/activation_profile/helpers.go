// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes Jamf Security Cloud returns on the activation
// profile endpoints, wire-probed against the EU gateway on 2026-09-01.
//
// Each code was checked individually against the SDK's generated ApiErrorItemCode
// enum — which is the Security Cloud DNS namespace's error schema rather than a
// namespace-wide one — rather than reasoning about the set, per STYLE_GUIDE §Enum
// values and error codes come from the SDK. NOT_ENTITLED is in it. STATE_CONFLICT
// is genuinely absent and is declared as an exemption in enum_literals_test.go,
// where a later SDK release that adds it fails the test. INVALID_FIELD is in the
// enum but is deliberately not referenced here — see appendWriteDiagnostics.
const (
	codeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled

	// codeStateConflict is returned by pause and resume against a code that has
	// already been deleted. The SDK carries no constant for it.
	codeStateConflict = "STATE_CONFLICT"
)

// appendNoCapabilityEnabledError raises the field-attributed diagnostic for a
// profile that enables no service capability.
//
// Two paths reach the same rule — the config validator at plan time, and
// buildCreateRequest once every value is known — so the wording lives here
// rather than being written out at both, where the two copies would be free to
// drift and report one server rule as two different errors.
func appendNoCapabilityEnabledError(diags *diag.Diagnostics) {
	diags.AddAttributeError(
		path.Root("capabilities"),
		"No service capability enabled",
		"An activation profile must enable at least one service capability. Set `content_controls`, "+
			"`network_security`, or both.",
	)
}

// appendWriteDiagnostics turns a create or state-assertion failure into the most
// specific diagnostic the error body supports, and reports whether it recognised
// one.
//
// Two codes are worth translating. NOT_ENTITLED is the Security Cloud staple: the
// credentials are valid and the tenant simply does not hold the surface, which a
// bare 403 hides. STATE_CONFLICT is specific to this construct and names a
// situation Terraform cannot see for itself — the profile was deleted out of
// band, which the read surface reports as a healthy profile because deletion here
// is a soft delete that GET does not reflect.
//
// INVALID_FIELD is deliberately not translated. Every field constraint this
// surface enforces is checked at plan time, so an INVALID_FIELD reaching apply
// means the server enforces something the schema does not yet model, and the
// server's own field-attributed message is more informative than anything this
// function could add.
//
// A detail carrying any other code is surfaced verbatim rather than dropped, but
// only once some detail in the same response has been recognised. That asymmetry
// is deliberate: when nothing matched, this function reports false and the caller
// prints the whole error itself, so repeating a detail here would duplicate it —
// whereas a response mixing a recognised code with an unrecognised one takes the
// caller's fallback away, and the unrecognised detail would then be reported
// nowhere at all.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	var recognised, unrecognised diag.Diagnostics
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeNotEntitled:
			recognised.AddError(
				"Jamf Security Cloud activation profiles not available for this tenant",
				"The credentials are valid, but this tenant is not entitled to Jamf Security Cloud activation "+
					"profiles. Confirm the tenant holds Jamf Security Cloud and that the API integration carries "+
					"the activation profile privileges. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeStateConflict:
			recognised.AddError(
				"Activation profile has already been deleted",
				"Jamf Security Cloud reports this activation profile as deleted. Deleting an activation profile is "+
					"a soft delete that reads back as a healthy profile, so Terraform cannot detect it during a "+
					"refresh — the profile was most likely deleted outside Terraform. Remove it from state with "+
					"`terraform state rm` and apply again to mint a replacement. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		default:
			unrecognised.AddError(
				"Jamf Security Cloud reported a further error",
				"Jamf Security Cloud reported this alongside the error above, and the provider has no more "+
					"specific explanation for it. Code "+detail.Code+": "+detail.Description,
			)
		}
	}
	if len(recognised) == 0 {
		return false
	}
	diags.Append(recognised...)
	diags.Append(unrecognised...)
	return true
}
