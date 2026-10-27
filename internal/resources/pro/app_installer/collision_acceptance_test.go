// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_installer_test

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
resource "jamfplatform_pro_app_installer" "%s" {
	name            = %q
	app_title_name  = %s
	deployment_type = "SELF_SERVICE"
	update_behavior = "AUTOMATIC"
}
`, label, name, titleNameRef)
}

// TestAccResource_ProAppInstaller_StrictNameCollision proves the strict check on a type
// that stores duplicates. A second resource at another address plans a name the
// first already holds, in a different case. With include_unenforced set the plan
// fails naming the existing object. With the default configuration the same
// config applies and the tenant ends up with two objects, which is the behaviour
// the strict check exists to flag.
func TestAccResource_ProAppInstaller_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-appinst-" + testhelpers.RunSuffix()
	duplicate := strings.ToUpper(name)

	first := catalogDS + collisionResource("existing", name)
	both := first + collisionResource("duplicate", duplicate)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppInstallerDestroy(t),
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
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_installer.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_installer.duplicate", "id"),
				),
			},
		},
	})
}
