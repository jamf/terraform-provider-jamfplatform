// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// SiteClearFixtureAddr is the address of the site SiteClearSteps creates for a
// resource to reference.
const SiteClearFixtureAddr = "jamfplatform_pro_site.clear"

// siteClearFixture returns the site resource SiteClearSteps prepends to the
// configuration under test.
//
// The site name carries the tag as well as the run suffix: the suffix is the
// same for every package started in one second, and Jamf Pro refuses two sites
// of one name.
func siteClearFixture(tag, suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_site" "clear" {
  name = "tf-acc-siteclear-%s-%s"
}
`, tag, suffix)
}

// SiteRefusalSteps returns the plan-only steps asserting an empty and a "0"
// site reference are refused at plan time (#573). config receives the HCL
// expression to assign to the site attribute and returns the resource under
// test; the diagnostic must say "Invalid site ID".
func SiteRefusalSteps(tag, suffix string, config func(siteExpr string) string) []resource.TestStep {
	return []resource.TestStep{
		{
			Config:      siteClearFixture(tag, suffix) + config(`""`),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Invalid site ID`),
		},
		{
			Config:      siteClearFixture(tag, suffix) + config(`"0"`),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Invalid site ID`),
		},
	}
}

// SiteClearSteps returns the steps proving "-1" clears a site reference on the
// server (#573): the plan-time refusals of SiteRefusalSteps, a create pointing
// at a real site, an update to "-1" asserted against the live object with
// live("-1") and in state, and a follow-up plan that must be empty. live
// returns a check that reads the object back from Jamf Pro and compares its
// site with want.
func SiteClearSteps(tag, suffix, addr, attr string, config func(siteExpr string) string, live func(want string) resource.TestCheckFunc) []resource.TestStep {
	steps := SiteRefusalSteps(tag, suffix, config)
	return append(steps,
		resource.TestStep{
			Config: siteClearFixture(tag, suffix) + config(SiteClearFixtureAddr+".id"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrPair(addr, attr, SiteClearFixtureAddr, "id"),
			),
		},
		resource.TestStep{
			Config: siteClearFixture(tag, suffix) + config(`"-1"`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(addr, attr, "-1"),
				live("-1"),
			),
		},
		resource.TestStep{
			Config: siteClearFixture(tag, suffix) + config(`"-1"`),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
			},
		},
	)
}
