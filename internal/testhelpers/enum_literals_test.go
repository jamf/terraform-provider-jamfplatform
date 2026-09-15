// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package testhelpers

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for the acceptance fixtures.
//
// Fixtures are held to the same rule as the resources they exercise: a fixture
// that writes a hand-typed enum value can drift from the schema it is meant to
// prove, and the failure looks like a tenant problem rather than a typo.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			compliancebenchmarks.BenchmarkV2SyncStateValues(),
			pro.ComputerSectionV4Values(),
			pro.CloudDistributionPointCdnTypeValues(),
			proclassic.LdapServerConnectionServerTypeValues(),
			proclassic.LdapServerConnectionAuthenticationTypeValues(),
			proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllValues(),
			proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeValues(),
			proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsUserGroupMembershipStoredInValues(),
			proclassic.WebhookContentTypeValues(),
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
