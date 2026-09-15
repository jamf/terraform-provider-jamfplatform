// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"slices"
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
			pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIosValues(),
			pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIpadValues(),
		),
		Absent: map[string]string{
			"Default Names":  "names.assignNamesUsing is typed as a plain string in the spec; no enum is generated",
			"List of Names":  "names.assignNamesUsing is typed as a plain string in the spec; no enum is generated",
			"Serial Numbers": "names.assignNamesUsing is typed as a plain string in the spec; no enum is generated",
			"Single Name":    "names.assignNamesUsing is typed as a plain string in the spec; no enum is generated",
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

// TestMinimumOsVocabulariesAgree backs the single
// prestageMinimumOsTargetVersionValues shared by the iOS and iPadOS min-OS
// enforcement attributes. The API declares the same five values for each, so one
// var is right for both — but only while that holds.
func TestMinimumOsVocabulariesAgree(t *testing.T) {
	ios := pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIosValues()
	ipad := pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIpadValues()

	for _, v := range ios {
		if !slices.Contains(ipad, v) {
			t.Errorf("the iOS vocabulary carries %q but the iPadOS one does not", v)
		}
	}
	for _, v := range ipad {
		if !slices.Contains(ios, v) {
			t.Errorf("the iPadOS vocabulary carries %q but the iOS one does not", v)
		}
	}
}
