// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_privileges

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// Nothing here needs aliasing. The one literal the audit flagged — catAttrs
// ["all"], the data source attribute holding the flat union of every privilege —
// is an attribute name that happens to spell an unrelated enum member, and it is
// an assignment inside a function rather than a declaration, so the walker
// correctly never sees it. The test still earns its place: it fails the moment
// this package starts declaring a privilege-set value of its own.
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
