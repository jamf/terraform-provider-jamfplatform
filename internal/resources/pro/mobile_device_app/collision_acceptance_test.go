// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_app_test

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
resource "jamfplatform_pro_mobile_device_app" "%s" {
	general = {
		name                 = %q
		version              = "1.0"
		bundle_id            = "com.example.tfacc.collision"
		os_type              = "iOS"
		deploy_automatically = false
	}
}
`, label, name)
}

// TestAccResource_ProMobileApp_StrictNameCollision proves the strict check on a type
// that stores duplicates. A second resource at another address plans a name the
// first already holds, in a different case. With include_unenforced set the plan
// fails naming the existing object. With the default configuration the same
// config applies and the tenant ends up with two objects, which is the behaviour
// the strict check exists to flag.
func TestAccResource_ProMobileApp_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdapp-" + testhelpers.RunSuffix()
	duplicate := strings.ToUpper(name)

	first := collisionResource("existing", name)
	both := first + collisionResource("duplicate", duplicate)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
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
					resource.TestCheckResourceAttrSet("jamfplatform_pro_mobile_device_app.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_mobile_device_app.duplicate", "id"),
				),
			},
		},
	})
}

// TestAccResource_ProMobileApp_RenameIntoManagedNameOffersNoImportStrict proves that
// renaming one resource onto the name of a sibling this configuration already manages is
// refused without offering an import block.
func TestAccResource_ProMobileApp_RenameIntoManagedNameOffersNoImportStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-mdapp-one-" + suffix
	two := "tf-acc-collide-mdapp-two-" + suffix

	pair := func(nameOne, nameTwo string) string {
		return collisionProviderBlock +
			collisionResource("one", nameOne) +
			strings.Replace(collisionResource("two", nameTwo), "\tgeneral", "\tdepends_on = [jamfplatform_pro_mobile_device_app.one]\n\tgeneral", 1)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{Config: pair(one, two)},
			{
				Config:      pair(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
