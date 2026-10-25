// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package patch_policy_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func policyBlock(label, name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_patch_policy" %q {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = %q
			target_version                  = %q
			enabled                         = false
			distribution_method             = "selfservice"
			allow_downgrade                 = false
			patch_unknown                   = false

			user_interaction = {
				install_button_text = "Update"

				deadlines = {
					enabled = true
					period  = 7
				}
			}
		}
	`, label, name, accTitleVersion)
}

func TestAccResource_ProPatchPolicy_NameCollisionWithinTitle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-collide-pp-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fixtureTitle(suffix) + policyBlock("first", name),
			},
			{
				Config:      fixtureTitle(suffix) + policyBlock("first", name) + policyBlock("second", strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*patch\s+policy.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}
