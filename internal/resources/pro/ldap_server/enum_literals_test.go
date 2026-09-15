// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

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
			proclassic.LdapServerConnectionServerTypeValues(),
			proclassic.LdapServerConnectionAuthenticationTypeValues(),
			proclassic.LdapServerConnectionReferralResponseValues(),
			proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllValues(),
			proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeValues(),
			proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsUserGroupMembershipStoredInValues(),
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

// TestMappingVocabulariesAgree backs the single object-class and search-scope
// constant pairs shared across the three mapping blocks. The classic spec
// declares each vocabulary once per block — user, user-group and
// user-group-membership — with identical members, so one pair serves all three.
// If a future SDK release diverges them, one shared pair is wrong for at least
// one block and this fails.
func TestMappingVocabulariesAgree(t *testing.T) {
	objectClass := map[string][]string{
		"user":                  proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllValues(),
		"user group":            proclassic.LdapServerMappingsForUsersUserGroupMappingsMapObjectClassToAnyOrAllValues(),
		"user group membership": proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsMapObjectClassToAnyOrAllValues(),
	}
	searchScope := map[string][]string{
		"user":                  proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeValues(),
		"user group":            proclassic.LdapServerMappingsForUsersUserGroupMappingsSearchScopeValues(),
		"user group membership": proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsSearchScopeValues(),
	}

	for name, sets := range map[string]map[string][]string{"map_object_class_to_any_or_all": objectClass, "search_scope": searchScope} {
		want := sets["user"]
		for block, got := range sets {
			if len(got) != len(want) {
				t.Errorf("%s: the %s block declares %d values, the user block declares %d", name, block, len(got), len(want))
				continue
			}
			for _, v := range want {
				if !slices.Contains(got, v) {
					t.Errorf("%s: the user block carries %q but the %s block does not", name, v, block)
				}
			}
		}
	}
}
