// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// The criterion join is the one vocabulary this package's writes touch, and it
// is not restated here: the shared criterion schema owns the default and the
// accepted set, and the shared builder emits the value. Naming it as covered is
// what makes the guard fire the day a literal join creeps back in.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.MobileDeviceSmartGroupCriteriaV2AndOrValues(),
		),
		Ignore: map[string]string{
			"groupId":   "A filter selector this collection accepts, not an enum value. Selectors keep their API spelling because they travel to Jamf Pro verbatim.",
			"groupName": "Same: a filter selector.",
			"siteId":    "Same: a filter selector.",
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
