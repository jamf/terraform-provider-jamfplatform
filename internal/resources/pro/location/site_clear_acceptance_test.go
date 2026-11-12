//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package location_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProVolumePurchasingLocation_SiteClear proves "-1" is the clear
// value for site_id (#573, wire-probed 2026-11-12): "" and "0" are refused at
// plan time (the server answers 400 INVALID_ID for both), "-1" clears the site,
// and the cleared state plans empty.
func TestAccResource_ProVolumePurchasingLocation_SiteClear(t *testing.T) {
	token := testhelpers.AccEnv(vppTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping VPP location acceptance test", vppTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	const addr = "jamfplatform_pro_volume_purchasing_location.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_site" "clear" {
  name = "tf-acc-vpp-siteclear-%[1]s"
}
resource "jamfplatform_pro_volume_purchasing_location" "test" {
  name                     = "tf-acc-vpp-siteclear-%[1]s"
  service_token            = %[2]q
  service_token_wo_version = 1
  site_id                  = %[3]s
}
`, suffix, token, siteExpr)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVolumePurchasingLocationDestroy(t),
		Steps: []resource.TestStep{
			{Config: config(`""`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid site ID`)},
			{Config: config(`"0"`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid site ID`)},
			{
				Config: config("jamfplatform_pro_site.clear.id"),
				Check:  resource.TestCheckResourceAttrPair(addr, "site_id", "jamfplatform_pro_site.clear", "id"),
			},
			{
				Config: config(`"-1"`),
				Check:  resource.TestCheckResourceAttr(addr, "site_id", "-1"),
			},
			{
				Config:           config(`"-1"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}
