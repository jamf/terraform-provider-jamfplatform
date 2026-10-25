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
