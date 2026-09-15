// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.ComputerPrestageV3RecoveryLockPasswordTypeValues(),
			pro.ComputerPrestageV3PrestageMinimumOsTargetVersionTypeValues(),
			pro.AccountSettingsRequestUserAccountTypeValues(),
		),
		Absent: map[string]string{
			"CUSTOM":       "prefillType is documented in prose only \u2014 pro/types.go says \"Values accepted are only CUSTOM and DEVICE_OWNER\" above the field \u2014 and generates no enum",
			"DEVICE_OWNER": "prefillType is documented in prose only and generates no enum",
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
