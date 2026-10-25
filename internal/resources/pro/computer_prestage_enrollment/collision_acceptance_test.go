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

func collisionPrestageConfig(name, instanceID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %q
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
`, name, instanceID)
}

func TestAccResource_ProComputerPrestageEnrollment_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	instanceID := testhelpers.FirstDeviceEnrollmentInstanceID(t)
	name := "tf-acc-prestage-collide-" + testhelpers.RunSuffix()
	planned := collisionPrestageConfig(strings.ToUpper(name), instanceID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandComputerPrestage(t, name, instanceID) },
				Config:      strictCollisionProvider + planned,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use\s+\(duplicate\s+check\).*already\s+exists\s+in\s+Jamf\s+Pro.*accepts\s+duplicates`),
			},
			{
				Config:             planned,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
