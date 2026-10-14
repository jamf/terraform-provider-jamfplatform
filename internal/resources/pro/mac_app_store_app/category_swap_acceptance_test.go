// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// macAppConfigCategories returns an app in two of three Self Service
// categories, named by first and second, with feature_in set on the "a"
// category only.
func macAppConfigCategories(name, suffix, first, second string) string {
	entry := func(which string) string {
		if which == "a" {
			return `{ id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },`
		}
		return fmt.Sprintf(`{ id = jamfplatform_pro_category.%s.id, display_in = true },`, which)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_category" "a" {
  name     = "tf-acc-macapp-swap-a-%[2]s"
  priority = 9
}
resource "jamfplatform_pro_category" "b" {
  name     = "tf-acc-macapp-swap-b-%[2]s"
  priority = 9
}
resource "jamfplatform_pro_category" "c" {
  name     = "tf-acc-macapp-swap-c-%[2]s"
  priority = 9
}

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
      %[3]s
      %[4]s
    ]
  }
}
`, name, suffix, entry(first), entry(second))
}

// checkMacAppCategory checks that the app's category set holds an element
// whose id is the id of the category resource named which, carrying that
// category's own name and the given feature_in.
func checkMacAppCategory(which, featureIn string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		cat, ok := s.RootModule().Resources["jamfplatform_pro_category."+which]
		if !ok {
			return fmt.Errorf("category %s not in state", which)
		}
		app, ok := s.RootModule().Resources[macAppResourceAddr]
		if !ok {
			return fmt.Errorf("%s not in state", macAppResourceAddr)
		}
		attrs := app.Primary.Attributes
		n, err := strconv.Atoi(attrs["self_service.self_service_categories.#"])
		if err != nil {
			return fmt.Errorf("self_service_categories count: %w", err)
		}
		for i := range n {
			prefix := fmt.Sprintf("self_service.self_service_categories.%d.", i)
			if attrs[prefix+"id"] != cat.Primary.ID {
				continue
			}
			if got, want := attrs[prefix+"name"], cat.Primary.Attributes["name"]; got != want {
				return fmt.Errorf("category %s: name %q, want %q", which, got, want)
			}
			if got := attrs[prefix+"feature_in"]; got != featureIn {
				return fmt.Errorf("category %s: feature_in %q, want %q", which, got, featureIn)
			}
			return nil
		}
		return fmt.Errorf("no self_service_categories element with category %s's id %s", which, cat.Primary.ID)
	}
}

// TestAccResource_ProMacApp_SelfServiceCategorySwap re-points one Self Service
// category at another and then reorders the set. The framework pairs set
// elements with prior state by position, so the re-pointed element used to
// plan the old category's name and feature_in, and the apply failed with
// "planned set element ... does not correlate with any element in actual"
// (reproduced live 2026-10-14). The name is now Computed-only and the flags
// carry by id, so the new element takes category c's own name and Jamf Pro's
// default feature_in, and a reorder plans nothing.
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
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.self_service_categories.#", "2"),
					checkMacAppCategory("a", "true"),
					checkMacAppCategory("b", "false"),
				),
			},
			{
				Config: macAppConfigCategories(name, suffix, "a", "c"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.self_service_categories.#", "2"),
					checkMacAppCategory("a", "true"),
					checkMacAppCategory("c", "false"),
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
