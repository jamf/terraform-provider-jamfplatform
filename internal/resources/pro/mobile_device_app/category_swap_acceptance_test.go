// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_app_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const mobileAppCategories = "self_service.self_service_categories"

// mobileAppConfigCategorySwap returns an app in two of the three swap-fixture
// categories, named by first and second. A mobile device app stores no
// feature_in, so display_in is the only flag; the second entry leaves it unset
// so a positional carry would have something to hand the first.
func mobileAppConfigCategorySwap(name, suffix, first, second string) string {
	return testhelpers.CategorySwapFixtures("tf-acc-mobileapp-swap", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_app" "test" {
  general = {
    name                 = %[1]q
    version              = "1.0"
    bundle_id            = "com.example.tfacc.mobileapp"
    os_type              = "iOS"
    deploy_automatically = false
  }
  self_service = {
    self_service_categories = [
      { id = jamfplatform_pro_category.%[2]s.id, display_in = true },
      { id = jamfplatform_pro_category.%[3]s.id },
    ]
  }
}
`, name, first, second)
}

// TestAccResource_ProMobileApp_SelfServiceCategorySwap re-points one Self
// Service category at another and then reorders the set. The framework pairs
// set elements with prior state by position, so the re-pointed element used to
// plan the old category's name and the apply failed with "Provider produced
// inconsistent result after apply". The name is now Computed-only and carried
// by id, so the new element takes category c's own name and a reorder plans
// nothing.
func TestAccResource_ProMobileApp_SelfServiceCategorySwap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mobileapp-catswap-" + suffix
	shown := map[string]string{"display_in": "true"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mobileAppConfigCategorySwap(name, suffix, "a", "b"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, mobileAppCategories+".#", "2"),
					testhelpers.CheckCategoryElement(mobileAppResourceAddr, mobileAppCategories, "a", shown),
					testhelpers.CheckCategoryElement(mobileAppResourceAddr, mobileAppCategories, "b", shown),
				),
			},
			{
				Config: mobileAppConfigCategorySwap(name, suffix, "a", "c"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, mobileAppCategories+".#", "2"),
					testhelpers.CheckCategoryElement(mobileAppResourceAddr, mobileAppCategories, "a", shown),
					testhelpers.CheckCategoryElement(mobileAppResourceAddr, mobileAppCategories, "c", shown),
				),
			},
			{
				Config: mobileAppConfigCategorySwap(name, suffix, "c", "a"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
