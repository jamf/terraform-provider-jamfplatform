// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_installer_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// appInstallerCategoriesConfig renders a Self Service deployment whose
// categories are the given HCL list, beside the a/b/c category fixtures.
func appInstallerCategoriesConfig(name, suffix, categories string) string {
	return catalogDS + testhelpers.CategorySwapFixtures("tf-acc-ai-catval", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_app_installer" "test" {
  name            = %q
  app_title_name  = %s
  deployment_type = "SELF_SERVICE"
  update_behavior = "AUTOMATIC"
  self_service_settings = {
    categories = %s
  }
}
`, name, titleNameRef, categories)
}

// TestAccResource_ProAppInstaller_SelfServiceCategoryValidation pins the
// plan-time rejection of a repeated category_id (Jamf Pro keeps the last entry
// it receives, and a set has no order), then applies two categories with
// different featured values and checks the next plan is empty. The rejected
// config uses a literal id, because an id that references a category created
// in the same apply is unknown at plan time.
func TestAccResource_ProAppInstaller_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ai-catval-" + suffix

	two := `[
      { category_id = jamfplatform_pro_category.a.id, featured = true },
      { category_id = jamfplatform_pro_category.b.id, featured = false },
    ]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppInstallerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      appInstallerCategoriesConfig(name, suffix, `[{ category_id = "1", featured = true }, { category_id = "1", featured = false }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config: appInstallerCategoriesConfig(name, suffix, two),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddr, "self_service_settings.categories.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceAddr, "self_service_settings.categories.*", map[string]string{"featured": "true"}),
					resource.TestCheckTypeSetElemNestedAttrs(resourceAddr, "self_service_settings.categories.*", map[string]string{"featured": "false"}),
				),
			},
			{
				Config: appInstallerCategoriesConfig(name, suffix, two),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
