// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// This package originally wrote all six error codes as string literals, including
// INVALID_FIELD and NOT_ENTITLED, which securitycloud.ApiErrorItemCode does carry.
// Reviewers caught it twice by eye, which is exactly the job to hand to a test.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.ApiErrorItemCodeValues(),
		),
		Absent: map[string]string{
			"GROUP_ALREADY_EXISTS": "the name-collision code from create and update. securitycloud.ApiErrorItemCode is the DNS namespace's error schema and carries no group-specific code; the spelling appears nowhere in the SDK package",
			"RESERVED_GROUP_NAME":  "the refusal for a name resolving to a reserved one; absent from the SDK for the same reason as above",
			"GROUP_NOT_FOUND":      "the 404 code from read and update. The SDK generates the zone-shaped ZONE_NOT_FOUND but no group equivalent",
			"BAD_PERMISSIONS":      "the gateway's code for both a privilege gap and an unmapped route, deliberately not translated into a diagnostic. It is a gateway-level code, not a Security Cloud schema one, so no generated enum carries it",
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
