// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// The distribution method is covered: the mobile-device profile spec generates
// its own vocabulary for the field, under the wire name deploymentMethod rather
// than distributionMethod, and mappings.go aliases it.
//
// self_service.notification_location is the one vocabulary no Covered entry
// could name — the SDK generates no NotificationLocation for any construct, and
// the spellings collide with the patch policy's notification location, a
// different construct. Naming a foreign vocabulary in Covered just to exempt
// that collision would make the guard report a promotion the day that unrelated
// spec changed. The mobile-device profile does not expose the field, so nothing
// here needs exempting; the macOS sibling's guard carries those exemptions.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			proclassic.MobileDeviceConfigurationProfileGeneralLevelValues(),
			proclassic.MobileDeviceConfigurationProfileGeneralDeploymentMethodValues(),
		),
		Absent: map[string]string{
			"Device": "the write-side spelling for Device Level. proclassic.MobileDeviceConfigurationProfileGeneralLevel generates only the read-side pair (System / User), which the read constants alias \u2014 as does the User Level write constant, that spelling being symmetric across write and read",
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
