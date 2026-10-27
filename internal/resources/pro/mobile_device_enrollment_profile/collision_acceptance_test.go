// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_enrollment_profile_test

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

func collisionResource(label, name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_enrollment_profile" "%s" {
	name = %q
}
`, label, name)
}

// TestAccResource_ProMobileDeviceEnrollmentProfile_StrictNameCollision proves the strict check on a type
// that stores duplicates. A second resource at another address plans a name the
// first already holds, in a different case. With include_unenforced set the plan
// fails naming the existing object. With the default configuration the same
// config applies and the tenant ends up with two objects, which is the behaviour
// the strict check exists to flag.
func TestAccResource_ProMobileDeviceEnrollmentProfile_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdep-" + testhelpers.RunSuffix()
	duplicate := strings.ToUpper(name)

	first := collisionResource("existing", name)
	both := first + collisionResource("duplicate", duplicate)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
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
					resource.TestCheckResourceAttrSet("jamfplatform_pro_mobile_device_enrollment_profile.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_mobile_device_enrollment_profile.duplicate", "id"),
				),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceEnrollmentProfile_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix
	pair := func(nameOne, nameTwo string) string {
		return collisionResource("one", nameOne) + collisionResource("two", nameTwo)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: collisionProviderBlock + pair(one, two),
			},
			{
				Config:      collisionProviderBlock + pair(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
