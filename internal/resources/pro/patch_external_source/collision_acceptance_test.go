// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package patch_external_source_test

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
resource "jamfplatform_pro_patch_external_source" "%s" {
	name                           = %q
	enabled                        = true
	host_name                      = "definitions.example.com/v2/"
	port                           = 8443
	ssl_enabled                    = true
	certificate_validation_enabled = false
}
`, label, name)
}

// TestAccResource_ProPatchExternalSource_StrictNameCollision proves the strict check on a type
// that stores duplicates. A second resource at another address plans a name the
// first already holds, in a different case. With include_unenforced set the plan
// fails naming the existing object. With the default configuration the same
// config applies and the tenant ends up with two objects, which is the behaviour
// the strict check exists to flag.
func TestAccResource_ProPatchExternalSource_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pes-" + testhelpers.RunSuffix()
	duplicate := strings.ToUpper(name)

	first := collisionResource("existing", name)
	both := first + collisionResource("duplicate", duplicate)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchExternalSourceDestroy(t),
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
					resource.TestCheckResourceAttrSet("jamfplatform_pro_patch_external_source.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_patch_external_source.duplicate", "id"),
				),
			},
		},
	})
}
