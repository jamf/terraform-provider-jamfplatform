// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// Covered spans both vocabularies this package touches: the platform enum, whose
// values it re-spells for HCL, and the Security Cloud error codes. The platform
// labels are lowercase and so are not literals of the SDK's values; the mapping
// table is keyed on the SDK constants so a rename breaks the build instead.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.ApiErrorItemCodeValues(),
			securitycloud.PublicApiCreateActivationProfileRequestPlatformsValues(),
			securitycloud.PublicApiCreateActivationProfileRequestOriginValues(),
		),
		Absent: map[string]string{
			"STATE_CONFLICT": "returned by pause and resume against an already-deleted code, and the only surface that reveals this construct's soft delete. securitycloud.ApiErrorItemCode is the DNS namespace's error schema and carries no equivalent; the spelling appears nowhere in the SDK package",
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
