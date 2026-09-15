// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// This resource is hybrid, and the two sides land in different buckets. The wire
// spellings it writes to Pro v1 alias pro.UserAccount*. The config-facing labels
// match proclassic.AccountAccessLevel and proclassic.AccountPrivilegeSet
// exactly — Jamf's UI labels are the classic API's spellings — but the fields
// they configure never reach the classic endpoint, so they stay literals rather
// than pinning the provider's public schema to a spec it does not call.
//
// Both classic vocabularies are named in Covered anyway, so that decision is an
// explicit exemption per label instead of a gap nobody looked at.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.UserAccountAccessLevelValues(),
			pro.UserAccountPrivilegeLevelValues(),
			pro.UserAccountAccountStatusValues(),
			pro.UserAccountAccountTypeValues(),
			proclassic.AccountAccessLevelValues(),
			proclassic.AccountPrivilegeSetValues(),
		),
		Ignore: map[string]string{
			"Full Access":     "config-facing label; see schema_types.go \u2014 the field is written to Pro v1, not to the classic endpoint whose vocabulary this matches",
			"Site Access":     "config-facing label; as above",
			"Group Access":    "config-facing label; as above",
			"Administrator":   "config-facing label; as above",
			"Auditor":         "config-facing label; as above",
			"Enrollment Only": "config-facing label; as above",
			"Custom":          "config-facing label; as above",
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
