// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			devicegroups.GroupTypeV1Values(),
			devicegroups.DeviceTypeV1Values(),
			devicegroups.JoinTypeV1Values(),
		),
		Ignore: map[string]string{
			"computer": "the Terraform-facing device_type value. devicegroups.DeviceTypeV1 spells the wire values COMPUTER / MOBILE, so there is no constant for the lowercase form this schema exposes",
			"mobile":   "as above",
			"and":      "the Terraform-facing and_or value. devicegroups.JoinTypeV1 spells the wire values AND / OR, so there is no constant for the lowercase form",
			"or":       "as above",
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
