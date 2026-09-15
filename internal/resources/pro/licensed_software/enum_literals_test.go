// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			proclassic.LicensedSoftwareDefintionCompareTypeValues(),
			proclassic.LicensedSoftwareLicensesLicenseItemLicenseTypeValues(),
			proclassic.PolicyGeneralNetworkRequirementsValues(),
		),
		Ignore: map[string]string{
			"Any": "the platform attribute (Any / Mac / Windows) has no generated enum. proclassic.PolicyGeneralNetworkRequirements carries \"Any\" for an unrelated field",
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
