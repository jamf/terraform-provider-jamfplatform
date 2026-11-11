// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const policyCategoryAddr = "jamfplatform_pro_policy.test"

// policyCategoriesConfig renders a Self Service policy whose categories are
// the given HCL list, beside the a/b/c category fixtures.
func policyCategoriesConfig(name, suffix, categories string) string {
	return testhelpers.CategorySwapFixtures("tf-acc-policy-catval", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = { name = %q }
  self_service = {
    use_for_self_service = true
    categories           = %s
  }
}
`, name, categories)
}

// TestAccResource_ProPolicy_SelfServiceCategoryValidation pins the plan-time
// rejection of the category shapes Jamf Pro rewrites (a repeated id is merged,
// display_in = false is dropped), then applies the shape that replaces them:
// two categories, each with both flags, which round-trips and plans clean.
// The rejected configs use literal ids: an id that references a category
// created in the same apply is unknown at plan time, and the validator
// defers on it.
func TestAccResource_ProPolicy_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-catval-" + suffix

	both := `[
      { id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },
      { id = jamfplatform_pro_category.b.id, display_in = true, feature_in = true },
    ]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      policyCategoriesConfig(name, suffix, `[{ id = "1", feature_in = true }, { id = "1", display_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config:      policyCategoriesConfig(name, suffix, `[{ id = "1", display_in = false, feature_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`feature_in requires display_in`),
			},
			{
				Config:      policyCategoriesConfig(name, suffix, `[{ id = "1", display_in = false }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category with display_in = false`),
			},
			{
				Config: policyCategoriesConfig(name, suffix, both),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyCategoryAddr, "self_service.categories.#", "2"),
					testhelpers.CheckCategoryElement(policyCategoryAddr, "self_service.categories", "a", map[string]string{"display_in": "true", "feature_in": "true"}),
					testhelpers.CheckCategoryElement(policyCategoryAddr, "self_service.categories", "b", map[string]string{"display_in": "true", "feature_in": "true"}),
				),
			},
			{
				Config: policyCategoriesConfig(name, suffix, both),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
