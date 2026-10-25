// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

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
resource "jamfplatform_pro_policy" "test" {
  general = {
    name    = %q
    enabled = false
  }
}
`, name)
}

func TestAccResource_ProPolicy_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-policy-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandClassicPolicy(t, name) },
				Config:      collisionConfig(strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*policy.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProPolicy_NameDifferingByAccentDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-policy-acc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { testhelpers.CreateOutOfBandClassicPolicy(t, name) },
				Config:             collisionConfig(strings.Replace(name, "e", "é", 1)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
