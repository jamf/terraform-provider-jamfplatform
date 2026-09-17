// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
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
			pro.GroupDtoV1GroupTypeValues(),
		),
		Absent: map[string]string{
			"HAS_DEPENDENCIES":  "The group specs type every error body as a bare ApiError whose `code` is an unconstrained string and enumerate no values, so the SDK generates no constant. Observed on DELETE /pro/v3/computer-groups/static-groups/{id} and DELETE /pro/v2/groups/{id}, 2026-09-17.",
			"INVALID_DEVICE":    "Same: no generated constant. Observed on POST and PUT of both static group endpoints, 2026-09-17.",
			"DUPLICATE_FIELD":   "Same: no generated constant. Observed on POST /pro/v3/computer-groups/static-groups, 2026-09-17.",
			"INVALID_PRIVILEGE": "Same: no generated constant. Observed on the mobile creates with siteId absent and on a computer PUT naming a nonexistent site, 2026-09-17.",
		},
		Ignore: map[string]string{
			"-1":                               "The Jamf Pro no-site sentinel, not a member of any vocabulary.",
			"progroups.platform_id.forbidden":  "A providerdata.FiredOnce latch key.",
			"progroups.platform_id.transient":  "A providerdata.FiredOnce latch key.",
			"progroups.platform_id.ambiguous":  "A providerdata.FiredOnce latch key.",
			"progroups.platform_id.unresolved": "A providerdata.FiredOnce latch key.",
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
