// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package device_group_test

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

// TestAccResource_DeviceGroup_JamfGroupMemberOf covers the Jamf Pro 11.29
// name->id regression for a Platform device group's "Computer Group" membership
// criterion (the COMPUTER device type — wire-probed as the one device_group
// surface that echoes the group id on read; see
// spike/JAMF_GROUP_MEMBER_OF_CRITERIA_SPIKE.md).
//
// A static fixture computer group is created via the classic SDK (no Terraform
// resource exists for classic computer groups) and referenced by name. The test
// asserts:
//
//  1. Author by NAME -> state round-trips back to the NAME, NOT the numeric id the
//     11.29 server stores. (Without the fix this is "produced inconsistent result
//     after apply".)
//  2. Re-applying the same config is an empty plan (no perpetual name-vs-id diff).
//  3. Swapping the config to the equivalent numeric id is an empty plan (ModifyPlan
//     recognises the name<->id representation swap as a no-op).
//
// The id-echo occurs only in the regressed window [11.29.0, 11.30.1) (PI-1394);
// 11.30.1 restored the name round-trip (wire-probed), so on 11.30.1+ the write-side
// workaround is off and this test degrades to a plain round-trip + swap-no-op check.
// All four assertions hold on both sides of the fix, so no version branch is needed.
func TestAccResource_DeviceGroup_JamfGroupMemberOf(t *testing.T) {
	testhelpers.AccPreCheck(t)
	// The "member of" / "not member of" directory-service-group operators are
	// rejected before Jamf Pro 11.29.
	testhelpers.RequireMinJamfProVersion(t, "11.29.0")

	suffix := testhelpers.RunSuffix()
	fixtureName := "tf-acc-dg-cg-fixture-" + suffix
	groupName := "tf-acc-dg-cg-memberof-" + suffix

	ctx := context.Background()
	client := proclassic.New(testhelpers.NewAcceptanceClient(t))

	// Static, empty computer group to be the "member of" target.
	isSmart := false
	created, err := client.CreateComputerGroupByID(ctx, "0", &proclassic.ComputerGroupPost{
		Name:    &fixtureName,
		IsSmart: &isSmart,
	})
	if err != nil {
		t.Fatalf("create fixture computer group: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatal("create fixture computer group: response missing id")
	}
	fixtureID := strconv.Itoa(*created.ID)
	t.Cleanup(func() {
		_ = client.DeleteComputerGroupByID(context.Background(), fixtureID)
	})

	cfgOp := func(op, value string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_device_group" "test" {
				name        = %q
				description = "Acceptance test — safe to delete"
				group_type  = "smart"
				device_type = "computer"
				criteria = [{
					criteria = "Computer Group"
					operator = %q
					value    = %q
				}]
			}
		`, groupName, op, value)
	}
	cfg := func(value string) string { return cfgOp("member of", value) }
	cfgNot := func(value string) string { return cfgOp("not member of", value) }

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				// (1) Author by name; state must hold the NAME, not the id.
				Config: cfg(fixtureName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test", "criteria.0.criteria", "Computer Group"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test", "criteria.0.operator", "member of"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test", "criteria.0.value", fixtureName),
				),
			},
			{
				// (2) Same config -> no perpetual diff.
				Config: cfg(fixtureName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// (3) Swap to the equivalent numeric id -> recognised as a no-op.
				Config: cfg(fixtureID),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// (4) "not member of" regresses identically (probed id "1"); confirm
				// it too round-trips back to the authored name.
				Config: cfgNot(fixtureName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_device_group.test", "criteria.0.operator", "not member of"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test", "criteria.0.value", fixtureName),
				),
			},
		},
	})
}
