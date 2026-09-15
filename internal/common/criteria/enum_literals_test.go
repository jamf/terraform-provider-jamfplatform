// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.SiteObjectObjectTypeValues(),
			proclassic.CriterionAndOrValues(),
			proclassic.LicensedSoftwareDefintionCompareTypeValues(),
		),
		Ignore: map[string]string{
			"is":                  "a criterion operator; collides with the licensed-software compare vocabulary for the same reason \"like\" does",
			"like":                "a smart-group / advanced-search criterion operator. Collides with proclassic.LicensedSoftwareDefintionCompareType, which is the licensed-software definition compare vocabulary \u2014 a different set that happens to share this one spelling; the classic search operator vocabulary has no generated enum",
			"Computer Group":      "a criterion *name* (the per-object-class group-membership criterion), not an object type. Collides with pro.SiteObjectObjectType, a 47-member catalogue of site-assignable object kinds",
			"Mobile Device Group": "a criterion name; see above",
			"User Group":          "a criterion name; see above",
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
