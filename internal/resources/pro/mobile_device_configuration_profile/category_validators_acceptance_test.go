// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_configuration_profile_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const mobileProfileCategoryAddr = "jamfplatform_pro_mobile_device_configuration_profile.test"

// orderedCategoryFixtures returns categories c1, c2 and c3, each created after
// the one before it, so Jamf Pro assigns them ascending ids and a list written
// c3, c2, c1 is in descending id order.
func orderedCategoryFixtures(suffix string) string {
	var out strings.Builder
	for i := 1; i <= 3; i++ {
		dependsOn := ""
		if i > 1 {
			dependsOn = fmt.Sprintf("\n  depends_on = [jamfplatform_pro_category.c%d]", i-1)
		}
		fmt.Fprintf(&out, `
resource "jamfplatform_pro_category" "c%[1]d" {
  name     = "tf-acc-mobprofile-catorder-%[1]d-%[2]s"
  priority = 9%[3]s
}
`, i, suffix, dependsOn)
	}
	return out.String()
}

func mobileProfileCategoryConfig(name, payload, suffix, categories string) string {
	return orderedCategoryFixtures(suffix) + fmt.Sprintf(`
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

// categoryOrder renders a categories list naming the fixtures in the given
// order, and checks that state holds them in that order.
func categoryOrder(fixtures ...int) (string, resource.TestCheckFunc) {
	var hcl []string
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(mobileProfileCategoryAddr, "self_service.categories.#", fmt.Sprint(len(fixtures))),
	}
	for i, f := range fixtures {
		hcl = append(hcl, fmt.Sprintf("{ id = jamfplatform_pro_category.c%d.id }", f))
		checks = append(checks,
			resource.TestCheckResourceAttrPair(mobileProfileCategoryAddr, fmt.Sprintf("self_service.categories.%d.id", i), fmt.Sprintf("jamfplatform_pro_category.c%d", f), "id"),
			resource.TestCheckResourceAttrPair(mobileProfileCategoryAddr, fmt.Sprintf("self_service.categories.%d.name", i), fmt.Sprintf("jamfplatform_pro_category.c%d", f), "name"),
		)
	}
	return "[" + strings.Join(hcl, ", ") + "]", resource.ComposeAggregateTestCheckFunc(checks...)
}

// TestAccResource_MobileDeviceConfigurationProfile_SelfServiceCategoryValidation
// pins two things about self_service.categories.
//
// A repeated category id, identical or not, is rejected at plan time: the
// list keeps both, Jamf Pro stores one, and the apply used to fail with
// "inconsistent result after apply". This resource exposes no display or
// feature flag, so that is the only rewritten shape a configuration can reach.
// The rejected config uses a literal id, because an id that references a
// category created in the same apply is unknown at plan time.
//
// Jamf Pro stores the list sorted by id, so categories written in descending
// id order used to fail the apply the same way. The state builder now puts
// the read back into the planned order: the descending list plans empty
// afterwards with and without refresh, and reordering it is a real update
// that settles on the new order.
func TestAccResource_MobileDeviceConfigurationProfile_SelfServiceCategoryValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mobprofile-catval-" + suffix
	payload := freshPayload(t, "profile_48.mobileconfig")
	descending, checkDescending := categoryOrder(3, 2, 1)
	reordered, checkReordered := categoryOrder(2, 3, 1)
	settled := resource.ConfigPlanChecks{
		PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}

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
				Config:           mobileProfileCategoryConfig(name, payload, suffix, descending),
				ConfigPlanChecks: settled,
				Check:            checkDescending,
			},
			{
				Config: mobileProfileCategoryConfig(name, payload, suffix, reordered),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(mobileProfileCategoryAddr, plancheck.ResourceActionUpdate)},
					PostApplyPreRefresh:  settled.PostApplyPreRefresh,
					PostApplyPostRefresh: settled.PostApplyPostRefresh,
				},
				Check: checkReordered,
			},
		},
	})
}
