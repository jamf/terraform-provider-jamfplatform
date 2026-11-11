// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package macos_configuration_profile_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// profileCategoryValidationConfig renders a profile whose Self Service
// categories are the given HCL list, beside the a/b/c category fixtures.
func profileCategoryValidationConfig(name, suffix, categories string) string {
	return testhelpers.CategorySwapFixtures("tf-acc-macprofile-catval", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %[1]q
    payloads = <<EOF
%[2]s
EOF
  }
  self_service = {
    categories = %[3]s
  }
}
`, name, minimalPayload, categories)
}

// TestAccResource_MacOSConfigurationProfile_SelfServiceCategoryValidation
// pins the plan-time rejection of the category shapes Jamf Pro rewrites. The
// list keeps identical entries, which Jamf Pro stores as one, and a repeated
// id with different flags is merged; display_in = false is dropped. The last
// steps apply two categories with both flags, in reverse id order, and check
// the next plan is empty. The rejected configs use literal ids, because an id
// that references a category created in the same apply is unknown at plan
// time.
func TestAccResource_MacOSConfigurationProfile_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-macprofile-catval-" + suffix

	both := `[
      { id = jamfplatform_pro_category.b.id, display_in = true, feature_in = true },
      { id = jamfplatform_pro_category.a.id, display_in = true, feature_in = true },
    ]`
	flags := map[string]string{"display_in": "true", "feature_in": "true"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      profileCategoryValidationConfig(name, suffix, `[{ id = "1" }, { id = "1" }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config:      profileCategoryValidationConfig(name, suffix, `[{ id = "1", feature_in = true }, { id = "1", display_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config:      profileCategoryValidationConfig(name, suffix, `[{ id = "1", display_in = false, feature_in = true }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`feature_in requires display_in`),
			},
			{
				Config:      profileCategoryValidationConfig(name, suffix, `[{ id = "1", display_in = false }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category with display_in = false`),
			},
			{
				Config: profileCategoryValidationConfig(name, suffix, both),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(profileCategorySwapAddr, profileCategories+".#", "2"),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "a", flags),
					testhelpers.CheckCategoryElement(profileCategorySwapAddr, profileCategories, "b", flags),
				),
			},
			{
				Config: profileCategoryValidationConfig(name, suffix, both),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
