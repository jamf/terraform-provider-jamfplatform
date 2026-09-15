// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// DomainStatus is the one vocabulary this package touches, and it touches it only
// to read: the status is Computed, so there is no OneOf validator and no
// restated value — verificationStatusUILabels keys on the SDK's constants and the
// documented list is built from DomainStatusValues(). The guard is live all the
// same, and that is its point: it fires the moment somebody writes "PENDING" into
// this package rather than aliasing the constant.
//
// The four error codes are Absent rather than Ignore. The Jamf Account SDK
// package generates no error-code vocabulary at all — ApiErrorItem.Code is a
// plain string and the values its doc comment names are prose — so these are
// values the SDK has yet to generate, not members of some other set that happens
// to share a spelling. Two of the four were only ever seen on the wire and appear
// in no published list, so they are the least likely to gain a constant and the
// most important to record.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			account.DomainStatusValues(),
		),
		Absent: map[string]string{
			codeConflict:        "the Jamf Account SDK generates no error-code enum; CONFLICT is also undocumented, observed only on the wire refusing a duplicate claim",
			codeBadRequest:      "the Jamf Account SDK generates no error-code enum; BAD_REQUEST is named in ApiErrorItem's doc comment as prose, with no constant behind it",
			codeFieldValidation: "the Jamf Account SDK generates no error-code enum; FIELD_VALIDATION is also undocumented, observed only on the wire refusing a blank domain",
			codeNotFound:        "the Jamf Account SDK generates no error-code enum; NOT_FOUND is named in ApiErrorItem's doc comment as prose, with no constant behind it",
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
