// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package vpp_invitation_test

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

func collisionInvitation(label, name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" %q {
  name                        = %q
  vpp_account_id              = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method         = "Make available in Self Service only"
  auto_register_managed_users = false
}
`, label, name)
}

// TestAccResource_ProVPPInvitation_StrictNameCollision proves the strict check on
// a type that stores duplicates. A second invitation at another address plans a
// name the first already holds, in a different case. With include_unenforced set
// the plan fails naming the existing invitation. With the default configuration
// the same config applies and the tenant holds two.
func TestAccResource_ProVPPInvitation_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := vppToken(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-collide-vppi-" + suffix

	first := vppLocationFixture(token, suffix) + collisionInvitation("existing", name)
	both := first + collisionInvitation("duplicate", strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
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
					resource.TestCheckResourceAttrSet("jamfplatform_pro_vpp_invitation.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_vpp_invitation.duplicate", "id"),
				),
			},
		},
	})
}
