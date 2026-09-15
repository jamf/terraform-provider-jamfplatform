// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// Nothing here is an enum. Both exemptions are English nouns this package
// interpolates into attribute descriptions, colliding with a classic profile
// level vocabulary whose members are lowercase. The test is here so that a
// scope value genuinely taken from a wire vocabulary cannot be added as a
// literal later.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			proclassic.OsXConfigurationProfileGeneralLevelValues(),
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
