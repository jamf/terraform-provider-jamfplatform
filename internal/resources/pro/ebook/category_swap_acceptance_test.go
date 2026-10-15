// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ebook_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const ebookCategories = "self_service.categories"

// ebookConfigCategorySwap returns an ebook in two of the three swap-fixture
// categories, named by first and second, with feature_in set on "a" only.
func ebookConfigCategorySwap(name, suffix, first, second string) string {
	entry := func(which string) string {
		if which == "a" {
			return `{ id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },`
		}
		return fmt.Sprintf(`{ id = jamfplatform_pro_category.%s.id, display_in = true },`, which)
	}
	return testhelpers.CategorySwapFixtures("tf-acc-ebook-swap", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_ebook" "test" {
  general = {
    name            = %[1]q
    author          = "TF Acc"
    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
    file_type       = "PDF"
    version         = "1.0"
    deployment_type = "Make Available in Self Service"
  }
  self_service = {
    categories = [
      %[2]s
      %[3]s
    ]
  }
}
`, name, entry(first), entry(second))
}

// TestAccResource_ProEbook_SelfServiceCategorySwap re-points one Self Service
// category at another and then reorders the set. The framework pairs set
// elements with prior state by position, so the re-pointed element used to
// plan the old category's name and flags and the apply failed with "Provider
// produced inconsistent result after apply". The name is now Computed-only and
// it and the flags carry by id, so the new element takes category c's own name
// and Jamf Pro's default feature_in, and a reorder plans nothing.
func TestAccResource_ProEbook_SelfServiceCategorySwap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ebook-catswap-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookConfigCategorySwap(name, suffix, "a", "b"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, ebookCategories+".#", "2"),
					testhelpers.CheckCategoryElement(ebookResourceAddr, ebookCategories, "a", map[string]string{"feature_in": "true"}),
					testhelpers.CheckCategoryElement(ebookResourceAddr, ebookCategories, "b", map[string]string{"feature_in": "false"}),
				),
			},
			{
				Config: ebookConfigCategorySwap(name, suffix, "a", "c"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, ebookCategories+".#", "2"),
					testhelpers.CheckCategoryElement(ebookResourceAddr, ebookCategories, "a", map[string]string{"feature_in": "true"}),
					testhelpers.CheckCategoryElement(ebookResourceAddr, ebookCategories, "c", map[string]string{"feature_in": "false"}),
				),
			},
			{
				Config: ebookConfigCategorySwap(name, suffix, "c", "a"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
