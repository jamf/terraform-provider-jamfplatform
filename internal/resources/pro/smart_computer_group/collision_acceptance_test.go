// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package smart_computer_group_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_smart_computer_group" "test" {
  name = %q
  criteria = [
    {
      name        = "Operating System Version"
      search_type = "like"
      value       = "26."
    },
  ]
}
`, name)
}

func TestAccResource_ProSmartComputerGroup_NameCollisionWithStaticGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-smcg-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandProStaticComputerGroup(t, name) },
				Config:      collisionConfig(strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*static\s+computer\s+group.*already\s+exists`),
			},
		},
	})
}

// twoSmartComputerGroupsConfig declares two smart groups. The second depends on the first so
// the creates are sequential: Jamf's duplicate check is not atomic, and two
// concurrent creates of one name can both succeed, which would leave a test
// asserting on a race instead of on the refusal.
func twoSmartComputerGroupsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_smart_computer_group" "one" {
  name = %[1]q
  criteria = [
    {
      name        = "Operating System Version"
      search_type = "like"
      value       = "26."
    },
  ]
}

resource "jamfplatform_pro_smart_computer_group" "two" {
  depends_on = [jamfplatform_pro_smart_computer_group.one]

  name = %[2]q
  criteria = [
    {
      name        = "Operating System Version"
      search_type = "like"
      value       = "26."
    },
  ]
}
`, nameOne, nameTwo)
}

func TestAccResource_ProSmartComputerGroup_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoSmartComputerGroupsConfig(one, two),
			},
			{
				Config:      twoSmartComputerGroupsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProSmartComputerGroup_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoSmartComputerGroupsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)already\s+exists.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
