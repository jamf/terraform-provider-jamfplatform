// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

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
			proclassic.OsXConfigurationProfileGeneralLevelValues(),
			proclassic.OsXConfigurationProfileGeneralDistributionMethodValues(),
		),
		Absent: map[string]string{
			"Computer":                             "the write-side level. proclassic.OsXConfigurationProfileGeneralLevel generates \"computer\"/\"user\"; the wire probe found the endpoint accepts \"Computer\"/\"User\" on write and returns \"System\"/\"User\" on read, so the generated set disagrees with both sides and aliasing it would put a rejected value on the wire",
			"System":                               "the read-side level for Computer Level; see above",
			"Self Service":                         "self_service.notification_location has no generated enum; it collides with the patch-policy notification location, a different construct",
			"Self Service and Notification Center": "as above",
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
