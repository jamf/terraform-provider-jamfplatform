// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package macos_configuration_profile_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const (
	profileCategorySwapAddr = "jamfplatform_pro_macos_configuration_profile.test"
	profileCategories       = "self_service.categories"
)

// profileConfigCategorySwap returns a profile in two of the three swap-fixture
// categories, in the order first then second, with feature_in set on "a" only.
func profileConfigCategorySwap(name, suffix, first, second string) string {
	entry := func(which string) string {
		if which == "a" {
			return `{ id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },`
		}
		return fmt.Sprintf(`{ id = jamfplatform_pro_category.%s.id, display_in = true },`, which)
	}
	return testhelpers.CategorySwapFixtures("tf-acc-macprofile-swap", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %[1]q
    payloads = <<EOF
%[2]s
EOF
  }
  self_service = {
    categories = [
      %[3]s
      %[4]s
    ]
  }
}
`, name, minimalPayload, entry(first), entry(second))
}

// TestAccResource_MacOSConfigurationProfile_SelfServiceCategorySwap re-points
// one Self Service category at another and then reverses the list. categories
// is a List, so the framework pairs elements with prior state by index, and the
// positional carry handed a re-pointed or moved element the previous
// occupant's name and flags; the apply then failed with "Provider produced
// inconsistent result after apply". The name is now Computed-only and it and
// the flags carry by id, so each element keeps its own category's values
// wherever it moves.
func TestAccResource_MacOSConfigurationProfile_SelfServiceCategorySwap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-macprofile-catswap-" + suffix
	featured := map[string]string{"feature_in": "true"}
	plain := map[string]string{"feature_in": "false"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: profileConfigCategorySwap(name, suffix, "a", "b"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(profileCategorySwapAddr, profileCategories+".#", "2"),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "a", featured),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "b", plain),
				),
			},
			{
				Config: profileConfigCategorySwap(name, suffix, "a", "c"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(profileCategorySwapAddr, profileCategories+".#", "2"),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "a", featured),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "c", plain),
				),
			},
			{
				Config: profileConfigCategorySwap(name, suffix, "c", "a"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(profileCategorySwapAddr, profileCategories+".#", "2"),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "a", featured),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "c", plain),
				),
			},
			{
				Config: profileConfigCategorySwap(name, suffix, "c", "a"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
