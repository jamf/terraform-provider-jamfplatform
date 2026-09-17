// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// One vocabulary is in reach here. The group type is what the platform
// identifier bridge narrows on, and this package reaches it through
// progroups.PlatformIDsByJamfProID, which takes the SDK's own constant for it;
// naming it in Covered is what fails the build the day someone writes the value
// out here instead. The error codes the group endpoints answer with are not
// restated in this package either — progroups owns them, and its own guard
// carries the Absent entries recording that the SDK generates no constant for
// them.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.GroupDtoV1GroupTypeValues(),
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
