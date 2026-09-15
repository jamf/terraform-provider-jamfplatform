// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

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
			proclassic.MobileDeviceApplicationGeneralDeploymentTypeValues(),
		),
		Absent: map[string]string{
			"iOS":  "general.os_type has no generated vocabulary. MobileDeviceApplicationGeneral.OsType is a plain *string with no \"Allowed values\" annotation, and no proclassic construct declares an os_type enum, so there is no constant to alias. pro.MobileDeviceResponseDeviceType carries this spelling but is the *inventory* discriminator — a different vocabulary in a different package, and one that also admits visionOS and watchOS, which this attribute does not",
			"tvOS": "the other half of general.os_type; see above",
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
