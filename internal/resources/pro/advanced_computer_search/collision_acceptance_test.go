// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package advanced_computer_search_test

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

func TestAccResource_ProAdvancedComputerSearch_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-acs-collide-" + testhelpers.RunSuffix()
	planned := acsConfigCreate(strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandClassicAdvancedComputerSearch(t, name) },
				Config:      strictCollisionProvider + planned,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use\s+\(duplicate\s+check\).*already\s+exists\s+in\s+Jamf\s+Pro.*accepts\s+duplicates`),
			},
			{
				Config:             planned,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func twoSearchesConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_computer_search" "one" {
			name = %q

			criteria = [
				{
					name        = "Computer Name"
					search_type = "like"
					value       = "lab"
				},
			]

			display_fields = ["Computer Name", "Username"]
		}

		resource "jamfplatform_pro_advanced_computer_search" "two" {
			depends_on = [jamfplatform_pro_advanced_computer_search.one]

			name = %q

			criteria = [
				{
					name        = "Computer Name"
					search_type = "like"
					value       = "lab"
				},
			]

			display_fields = ["Computer Name", "Username"]
		}
	`, nameOne, nameTwo)
}

func TestAccResource_ProAdvancedComputerSearch_RenameIntoManagedNameOffersNoImportStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-acs-one-" + suffix
	two := "tf-acc-collide-acs-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: strictCollisionProvider + twoSearchesConfig(one, two)},
			{
				Config:      strictCollisionProvider + twoSearchesConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
