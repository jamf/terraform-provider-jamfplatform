// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ebook_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const ebookCategoryAddr = "jamfplatform_pro_ebook.test"

// ebookCategoryValidationConfig renders a Self Service ebook whose categories are
// the given HCL list, beside the a/b/c category fixtures.
func ebookCategoryValidationConfig(name, suffix, categories string) string {
	return testhelpers.CategorySwapFixtures("tf-acc-ebook-catval", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_ebook" "test" {
  general = {
    name            = %q
    author          = "TF Acc"
    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
    file_type       = "PDF"
    version         = "1.0"
    deployment_type = "Make Available in Self Service"
  }
  self_service = {
    categories = %s
  }
}
`, name, categories)
}

// TestAccResource_ProEbook_SelfServiceCategoryValidation pins the plan-time
// rejection of the category shapes Jamf Pro rewrites (a repeated id is merged,
// display_in = false is dropped), then applies the shape that replaces them:
// two categories, each with both flags, which round-trips and plans clean.
// The rejected configs use literal ids: an id that references a category
// created in the same apply is unknown at plan time, and the validator
// defers on it.
func TestAccResource_ProEbook_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ebook-catval-" + suffix

	both := `[
      { id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },
      { id = jamfplatform_pro_category.b.id, display_in = true, feature_in = true },
    ]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      ebookCategoryValidationConfig(name, suffix, `[{ id = "1", feature_in = true }, { id = "1", display_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config:      ebookCategoryValidationConfig(name, suffix, `[{ id = "1", display_in = false, feature_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`feature_in requires display_in`),
			},
			{
				Config:      ebookCategoryValidationConfig(name, suffix, `[{ id = "1", display_in = false }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category with display_in = false`),
			},
			{
				Config: ebookCategoryValidationConfig(name, suffix, both),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookCategoryAddr, "self_service.categories.#", "2"),
					testhelpers.CheckCategoryElement(ebookCategoryAddr, "self_service.categories", "a", map[string]string{"display_in": "true", "feature_in": "true"}),
					testhelpers.CheckCategoryElement(ebookCategoryAddr, "self_service.categories", "b", map[string]string{"display_in": "true", "feature_in": "true"}),
				),
			},
			{
				Config: ebookCategoryValidationConfig(name, suffix, both),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
