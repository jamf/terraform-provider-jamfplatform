// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const macAppCategories = "self_service.self_service_categories"

// macAppConfigCategories returns an app in two of the three swap-fixture
// categories, named by first and second, with feature_in set on "a" only.
func macAppConfigCategories(name, suffix, first, second string) string {
	entry := func(which string) string {
		if which == "a" {
			return `{ id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },`
		}
		return fmt.Sprintf(`{ id = jamfplatform_pro_category.%s.id, display_in = true },`, which)
	}
	return testhelpers.CategorySwapFixtures("tf-acc-macapp-swap", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mac_app_store_app" "test" {
  general = {
    name            = %[1]q
    version         = "1.0"
    bundle_id       = "com.example.tfacc.macapp"
    url             = "https://apps.apple.com/app/id000000001"
    deployment_type = "Make Available in Self Service"
  }
  self_service = {
    self_service_categories = [
      %[2]s
      %[3]s
    ]
  }
}
`, name, entry(first), entry(second))
}

// TestAccResource_ProMacApp_SelfServiceCategorySwap re-points one Self Service
// category at another and then reorders the set. The framework pairs set
// elements with prior state by position, so the re-pointed element used to
// plan the old category's name and feature_in, and the apply failed with
// "planned set element ... does not correlate with any element in actual"
// (reproduced live 2026-10-14). The name is now Computed-only and it and the
// flags carry by id, so the new element takes category c's own name and Jamf
// Pro's default feature_in, and a reorder plans nothing.
func TestAccResource_ProMacApp_SelfServiceCategorySwap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-macapp-catswap-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppConfigCategories(name, suffix, "a", "b"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, macAppCategories+".#", "2"),
					testhelpers.CheckCategoryElement(macAppResourceAddr, macAppCategories, "a", map[string]string{"feature_in": "true"}),
					testhelpers.CheckCategoryElement(macAppResourceAddr, macAppCategories, "b", map[string]string{"feature_in": "false"}),
				),
			},
			{
				Config: macAppConfigCategories(name, suffix, "a", "c"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, macAppCategories+".#", "2"),
					testhelpers.CheckCategoryElement(macAppResourceAddr, macAppCategories, "a", map[string]string{"feature_in": "true"}),
					testhelpers.CheckCategoryElement(macAppResourceAddr, macAppCategories, "c", map[string]string{"feature_in": "false"}),
				),
			},
			{
				Config: macAppConfigCategories(name, suffix, "c", "a"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
