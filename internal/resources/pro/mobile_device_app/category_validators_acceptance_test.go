// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_app_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// mobileAppCategoriesConfig renders a mobile device app whose Self Service
// categories are the given HCL list, beside the a/b/c category fixtures.
func mobileAppCategoriesConfig(name, suffix, categories string) string {
	return testhelpers.CategorySwapFixtures("tf-acc-mobileapp-catval", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_app" "test" {
  general = {
    name                 = %q
    version              = "1.0"
    bundle_id            = "com.example.tfacc.mobileapp"
    os_type              = "iOS"
    deploy_automatically = false
  }
  self_service = {
    self_service_categories = %s
  }
}
`, name, categories)
}

// TestAccResource_ProMobileApp_SelfServiceCategoryValidation pins the
// plan-time rejection of the category shapes Jamf Pro rewrites (a repeated id
// is merged, display_in = false is dropped; a mobile device app has no
// feature flag), then applies two categories and checks the next plan is
// empty. The rejected configs use literal ids, because an id that references
// a category created in the same apply is unknown at plan time.
func TestAccResource_ProMobileApp_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mobileapp-catval-" + suffix

	two := `[
      { id = jamfplatform_pro_category.a.id, display_in = true },
      { id = jamfplatform_pro_category.b.id },
    ]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      mobileAppCategoriesConfig(name, suffix, `[{ id = "1" }, { id = "1", display_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config:      mobileAppCategoriesConfig(name, suffix, `[{ id = "1", display_in = false }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category with display_in = false`),
			},
			{
				Config: mobileAppCategoriesConfig(name, suffix, two),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, mobileAppCategories+".#", "2"),
					testhelpers.CheckCategoryElement(mobileAppResourceAddr, mobileAppCategories, "a", map[string]string{"display_in": "true"}),
					testhelpers.CheckCategoryElement(mobileAppResourceAddr, mobileAppCategories, "b", map[string]string{"display_in": "true"}),
				),
			},
			{
				Config: mobileAppCategoriesConfig(name, suffix, two),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
