// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes
// come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// Both codes this package translates — INVALID_FIELD and NOT_ENTITLED — are
// generated into securitycloud.ApiErrorItemCode, so Absent is empty. Each was
// checked individually against ApiErrorItemCodeValues() rather than reasoned about
// as a set: sibling packages shipped literals for exactly these two twice, on the
// strength of a comment claiming the SDK carried none of their codes.
//
// Ignore is empty too: the fixed Terraform state ID reaches this package through
// helpers.SingletonID rather than as a literal, which the guard confirms by refusing
// an exemption nothing uses.
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
