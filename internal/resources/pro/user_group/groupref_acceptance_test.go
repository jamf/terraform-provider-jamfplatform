// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package user_group_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProUserGroup_JamfGroupMemberOf covers the Jamf Pro 11.29
// name->id regression for a user group's "User Group" membership criterion (the
// classic /usergroups surface echoes the group id on read; see
// spike/JAMF_GROUP_MEMBER_OF_CRITERIA_SPIKE.md). The Smart_AllOperators sweep
// asserts the create round-trip; this test adds the perpetual-diff guard and the
// name<->id swap no-op that the sweep does not exercise.
//
// The "member of" target is a static fixture user group created via the classic
// SDK (so its numeric id is known for the swap step) and referenced by name; it is
// deleted on cleanup.
//
// The id-echo occurs only in the regressed window [11.29.0, 11.30.1) (PI-1394);
// 11.30.1 restored the name round-trip (wire-probed), so on 11.30.1+ the write-side
// workaround is off and this test degrades to a plain round-trip + swap-no-op check.
// All assertions hold on both sides of the fix, so no version branch is needed.
func TestAccResource_ProUserGroup_JamfGroupMemberOf(t *testing.T) {
	testhelpers.AccPreCheck(t)
	// The "member of" directory-service-group operator is rejected before Jamf
	// Pro 11.29.
	testhelpers.RequireMinJamfProVersion(t, "11.29.0")

	suffix := testhelpers.RunSuffix()
	fixtureName := "tf-acc-ug-cg-fixture-" + suffix
	groupName := "tf-acc-ug-cg-memberof-" + suffix

	ctx := context.Background()
	client := proclassic.New(testhelpers.NewAcceptanceClient(t))
	isSmart := false
	created, err := client.CreateUserGroupByID(ctx, "0", &proclassic.UserGroup{
		Name:    &fixtureName,
		IsSmart: &isSmart,
	})
	if err != nil {
		t.Fatalf("create fixture user group: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatal("create fixture user group: response missing id")
	}
	fixtureID := strconv.Itoa(*created.ID)
	t.Cleanup(func() {
		_ = client.DeleteUserGroupByID(context.Background(), fixtureID)
	})

	cfg := func(value string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_user_group" "test" {
				name       = %q
				group_type = "smart"
				criteria = [{
					name        = "User Group"
					search_type = "member of"
					value       = %q
				}]
			}
		`, groupName, value)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				// Author by name; state must hold the NAME, not the numeric id.
				Config: cfg(fixtureName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.0.value", fixtureName),
				),
			},
			{
				// Same config -> no perpetual diff.
				Config:           cfg(fixtureName),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				// Swap to the equivalent numeric id -> recognised as a no-op.
				Config:           cfg(fixtureID),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}
