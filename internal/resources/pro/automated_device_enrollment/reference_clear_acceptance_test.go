//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package automated_device_enrollment_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProAutomatedDeviceEnrollment_SiteClear proves "-1" is the clear
// value for site_id (#573, wire-probed 2026-11-12): "" and "0" are refused at plan
// time (the server answers 400 INVALID_ID for both), "-1" clears the site, and
// the cleared state plans empty. supervision_identity_id takes the same
// validator; it has no acceptance fixture because creating a supervision
// identity needs a certificate.
func TestAccResource_ProAutomatedDeviceEnrollment_SiteClear(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping ADE acceptance test", adeTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ade-siteclear-" + suffix
	const addr = "jamfplatform_pro_automated_device_enrollment.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_site" "clear" {
  name = "tf-acc-ade-siteclear-%s"
}
resource "jamfplatform_pro_automated_device_enrollment" "test" {
  name                    = %q
  server_token            = %q
  server_token_wo_version = 1
  site_id                 = %s
}
`, suffix, name, token, siteExpr)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAutomatedDeviceEnrollmentDestroy(t),
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
