// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package computer_prestage_enrollment_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const strictCollisionProvider = `
provider "jamfplatform" {
  name_collision_check = { include_unenforced = true }
}
`

func collisionPrestageResource(label, name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" %q {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}
  skip_setup_items       = {}

  scope_serial_numbers = []
}
`, label, name, adeFixtureRef)
}

func TestAccResource_ProComputerPrestageEnrollment_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-prestage-collide-" + suffix
	fixture := adeFixtureBlock(suffix, token)
	existing := fixture + collisionPrestageResource("existing", name)
	withDuplicate := existing + collisionPrestageResource("duplicate", strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: existing,
			},
			{
				Config:      strictCollisionProvider + withDuplicate,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use\s+\(duplicate\s+check\).*already\s+exists\s+in\s+Jamf\s+Pro.*accepts\s+duplicates`),
			},
			{
				Config:             withDuplicate,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-prestage-managed-one-" + suffix
	two := "tf-acc-prestage-managed-two-" + suffix
	fixture := adeFixtureBlock(suffix, token)
	pair := func(nameOne, nameTwo string) string {
		return fixture + collisionPrestageResource("one", nameOne) + collisionPrestageResource("two", nameTwo)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: strictCollisionProvider + pair(one, two),
			},
			{
				Config:      strictCollisionProvider + pair(one, one),
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
