// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package vpp_assignment_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const collisionProviderBlock = `
provider "jamfplatform" {
  name_collision_check = { include_unenforced = true }
}
`

func collisionAssignment(label, name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_vpp_assignment" %q {
  name                 = %q
  vpp_admin_account_id = jamfplatform_pro_volume_purchasing_location.vpp.id
}
`, label, name)
}

// TestAccResource_ProVPPAssignment_StrictNameCollision proves the strict check on
// a type that stores duplicates. A second assignment at another address plans a
// name the first already holds, in a different case. With include_unenforced set
// the plan fails naming the existing assignment. With the default configuration
// the same config applies and the tenant holds two.
func TestAccResource_ProVPPAssignment_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := vppToken(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-collide-vppa-" + suffix

	first := vppLocationFixture(token, suffix) + collisionAssignment("existing", name)
	both := first + collisionAssignment("duplicate", strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPAssignmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: first,
			},
			{
				Config:      collisionProviderBlock + both,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*duplicate\s+check`),
			},
			{
				Config: both,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_vpp_assignment.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_vpp_assignment.duplicate", "id"),
				),
			},
		},
	})
}

// twoAssignmentsConfig declares two assignments on one location. The second
// depends on the first so the creates are sequential.
func twoAssignmentsConfig(token, suffix, nameOne, nameTwo string) string {
	return collisionProviderBlock + vppLocationFixture(token, suffix) +
		collisionAssignment("one", nameOne) +
		strings.Replace(
			collisionAssignment("two", nameTwo),
			`resource "jamfplatform_pro_vpp_assignment" "two" {`,
			"resource \"jamfplatform_pro_vpp_assignment\" \"two\" {\n  depends_on = [jamfplatform_pro_vpp_assignment.one]\n",
			1,
		)
}

// TestAccResource_ProVPPAssignment_RenameIntoManagedNameOffersNoImport covers the
// managed-elsewhere wording for a type that stores duplicates, which only the
// strict check reaches. There is no two-new-with-one-name test: Jamf Pro accepts
// duplicate names, so the create is never refused and the conflict hint never
// fires.
func TestAccResource_ProVPPAssignment_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := vppToken(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPAssignmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: twoAssignmentsConfig(token, suffix, one, two),
			},
			{
				Config:      twoAssignmentsConfig(token, suffix, one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
