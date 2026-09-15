// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes
// come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// The vocabulary that matters here is the verification status, and it is taken
// entirely from the SDK's generated constants — which is what makes this guard
// worth having: the status is what the action's whole outcome classification keys
// on, and a restated one would fail to match a value the service really sends.
//
// The two error codes are checked individually rather than as a set. The Jamf
// Account SSO spec documents its error codes as prose inside ApiErrorItem.Code's
// description rather than as an enum schema, so the generator emits no type and no
// constants for any of them — unlike the domain status, which the same spec does
// declare as an enum.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: account.DomainStatusValues(),
		Absent: map[string]string{
			"BAD_REQUEST": "the code the five-minute verification refusal arrives under, matched together with " +
				"its description because the same code also covers an invalid domain and a malformed " +
				"identifier. The Jamf Account SSO spec declares no error-code enum, so no generated constant exists",
			"NOT_FOUND": "the code for an identifier this organization has no domain for; absent for the same reason",
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
