// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package accountprivileges

import (
	"slices"
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
			proclassic.AccountPrivilegeSetValues(),
			proclassic.GroupPrivilegeSetValues(),
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

// TestPrivilegeSetVocabulariesAgree backs the single administratorPrivilegeSet
// constant used for both the account and the group lookup. The classic API
// declares the same four values for each, so one constant is right for both —
// but only while that holds.
func TestPrivilegeSetVocabulariesAgree(t *testing.T) {
	accounts := proclassic.AccountPrivilegeSetValues()
	groups := proclassic.GroupPrivilegeSetValues()

	for _, v := range accounts {
		if !slices.Contains(groups, v) {
			t.Errorf("AccountPrivilegeSet carries %q but GroupPrivilegeSet does not", v)
		}
	}
	for _, v := range groups {
		if !slices.Contains(accounts, v) {
			t.Errorf("GroupPrivilegeSet carries %q but AccountPrivilegeSet does not", v)
		}
	}
}
