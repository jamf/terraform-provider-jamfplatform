// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_prestage_enrollment_test

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
resource "jamfplatform_pro_mobile_device_prestage_enrollment" %q {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  location_information   = {}
  purchasing_information = {}
  skip_setup_items       = {}
  names                  = {}

  scope_serial_numbers = []
}
`, label, name, adeFixtureRef)
}

func TestAccResource_ProMobileDevicePrestageEnrollment_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mobile-prestage-collide-" + suffix
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
