// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package enrollment_customization_test

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

func ecCollisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc collision"
			%s
			text_panes = [
				{ display_name = "Welcome", rank = 0, title = "Hello", body = "Body",
				  previous_button_text = "Back", next_button_text = "Next" },
			]
		}
	`, name, configCommon())
}

func TestAccResource_ProEnrollmentCustomization_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-ec-collide-" + testhelpers.RunSuffix()
	planned := ecCollisionConfig(strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandEnrollmentCustomization(t, name) },
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
