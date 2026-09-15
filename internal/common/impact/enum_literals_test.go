// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			devicegroups.DeviceTypeV1Values(),
			devicegroups.GroupTypeV1Values(),
			pro.ComputerSectionV4Values(),
			pro.MobileDeviceSectionValues(),
			pro.GroupDtoV1GroupTypeValues(),
		),
		Ignore: map[string]string{
			"COMPUTER": "impact.DeviceType is this package's own classification of the two Jamf Pro estates, never sent on the wire. It spans DeviceTypeAny (the empty string) which no API vocabulary has, and the wire values it is compared against come from pro.GroupDtoV1GroupType, which the comparison now names directly",
			"MOBILE":   "as above",
			"script":   "impact.DependencyKind is this package's own label for a policy-dependency family, not a wire value",
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
