// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// The five directory-binding type values stay literals: the classic
// /directorybindings payload has no generated enum. Two of them collide with
// proclassic.LdapServerConnectionServerType, which is why that vocabulary is
// named here rather than left out — checking against it is what makes the
// exemption an explicit decision instead of a silent gap, and what will fail if
// a future SDK release generates the binding type under that name.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			proclassic.LdapServerConnectionServerTypeValues(),
		),
		Ignore: map[string]string{
			"Active Directory": "the classic /directorybindings type vocabulary has no generated enum; proclassic.LdapServerConnectionServerType is the LDAP *server* type, a different construct that happens to share two spellings",
			"Open Directory":   "as above \u2014 sharing a spelling with the LDAP server type does not make it the same vocabulary",
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
