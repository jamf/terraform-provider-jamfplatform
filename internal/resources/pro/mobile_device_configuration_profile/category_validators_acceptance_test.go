// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_configuration_profile_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const mobileProfileCategoryAddr = "jamfplatform_pro_mobile_device_configuration_profile.test"

func mobileProfileCategoryConfig(name, payload, suffix, categories string) string {
	return testhelpers.CategorySwapFixtures("tf-acc-mobprofile-catval", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  self_service = {
    categories = %s
  }
}
`, name, payload, categories)
}

// TestAccResource_MobileDeviceConfigurationProfile_SelfServiceCategoryValidation
// pins the plan-time rejection of a repeated category id, identical or not:
// the list keeps both, Jamf Pro stores one, and the apply used to fail with
// "inconsistent result after apply". This resource exposes no display or
// feature flag, so that is the only rewritten shape a configuration can
// reach. The last steps apply one category and check the next plan is empty.
// The rejected config uses a literal id, because an id that references a
// category created in the same apply is unknown at plan time.
func TestAccResource_MobileDeviceConfigurationProfile_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mobprofile-catval-" + suffix
	payload := freshPayload(t, "profile_48.mobileconfig")
	one := `[{ id = jamfplatform_pro_category.a.id }]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      mobileProfileCategoryConfig(name, payload, suffix, `[{ id = "1" }, { id = "1" }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Self Service category listed more than once`),
			},
			{
				Config: mobileProfileCategoryConfig(name, payload, suffix, one),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileProfileCategoryAddr, "self_service.categories.#", "1"),
					testhelpers.CheckCategoryElement(mobileProfileCategoryAddr, "self_service.categories", "a", nil),
				),
			},
			{
				Config: mobileProfileCategoryConfig(name, payload, suffix, one),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
