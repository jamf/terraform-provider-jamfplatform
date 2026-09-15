// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// Seven of Jamf's vocabularies reach this construct, and every accepted set, every
// documented list and every translation is built from the SDK's own values. That
// makes the guard's job here mostly to catch a regression: it fires the moment
// somebody writes "WAAD" or "CLIENT_SECRET_POST" into this package rather than
// aliasing the constant. Note that this package's *renamed* vocabularies —
// `generic_oidc`, `client_secret`, `disabled` and the rest — are not exemptions:
// they are this provider's own names for Jamf's values, they collide with nothing
// Jamf generates, and mappings_test.go is what keeps them in step with it.
//
// The exemptions below are Absent rather than Ignore, which is the load-bearing
// distinction. Absent asserts the SDK carries no constant for the value and is
// therefore checked against the covered set, so an SDK release that starts
// generating one fails this test and says to alias it. Each is a value this
// package genuinely has to name and the SDK genuinely does not supply:
//
//   - The four error codes. The Jamf Account SDK package generates no error-code
//     vocabulary at all — ApiErrorItem.Code is a plain string, and the values its
//     doc comment names are prose with no constant behind them. FIELD_VALIDATION
//     is not even in that documented list; it was observed only on the wire.
//   - The two group-filter operators. The filter is an opaque document Jamf
//     publishes no schema for, so neither its properties nor its values appear in
//     any generated vocabulary.
//   - The three claim-mapping modes. Same reason, and weaker still: these are a
//     survey of one organization's connections rather than a declared set, which
//     is why an unrecognised mode is a warning and not a refusal.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			account.ConnectionTypeValues(),
			account.RegionValues(),
			account.TokenEndpointAuthMethodValues(),
			account.PkceAuthTypeValues(),
			account.EntraGroupsScopeValues(),
			account.EntraIdentityApiValues(),
			account.ProductValues(),
		),
		Absent: map[string]string{
			codeUpstreamError:       "the Jamf Account SDK generates no error-code enum; UPSTREAM_ERROR is named in ApiErrorItem's doc comment as prose, with no constant behind it",
			codeBadRequest:          "the Jamf Account SDK generates no error-code enum; BAD_REQUEST is named in ApiErrorItem's doc comment as prose, with no constant behind it",
			codeFieldValidation:     "the Jamf Account SDK generates no error-code enum; FIELD_VALIDATION is also undocumented, observed only on the wire refusing an absent top-level field",
			codeNotFound:            "the Jamf Account SDK generates no error-code enum; NOT_FOUND is named in ApiErrorItem's doc comment as prose, with no constant behind it",
			filterOpOr:              "the group filter is an opaque document Jamf publishes no schema for, so its operator has no generated vocabulary",
			filterOpAnd:             "the group filter is an opaque document Jamf publishes no schema for, so its operator has no generated vocabulary",
			mappingModeBindAll:      "the claim mapping is an opaque document Jamf publishes no schema for; this mode is a survey finding across every readable connection, not a declared value",
			mappingModeBasicProfile: "the claim mapping is an opaque document Jamf publishes no schema for; this mode is a survey finding across every readable connection, not a declared value",
			mappingModeUseMap:       "the claim mapping is an opaque document Jamf publishes no schema for; this mode is a survey finding across every readable connection, not a declared value",
		},
	})
	if err != nil {
		t.Fatalf("enumguard.Check: %v", err)
	}
	for _, problem := range got.Problems() {
		t.Error(problem)
	}
	if got.Examined == 0 {
		t.Fatal("no string literals parsed — the guard found nothing to check")
	}
}
