// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes
// come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// All three codes this package translates — INVALID_FIELD, LIST_SIZE_EXCEEDED and
// NOT_ENTITLED — are generated into securitycloud.ApiErrorItemCode, so Absent is
// empty. Each was checked individually against ApiErrorItemCodeValues() rather than
// reasoned about as a set: sibling packages shipped literals for exactly these codes
// twice, on the strength of a comment claiming the SDK carried none of theirs.
//
// The duplicate-hostname failure has no code at all — a 500 with an empty errors
// array — so there is nothing for this guard to check there, and nothing to exempt
// either. It is matched on status, not on a code string.
//
// Ignore is empty too: the fixed Terraform state ID reaches this package through
// helpers.SingletonID rather than as a literal.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.ApiErrorItemCodeValues(),
		),
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
