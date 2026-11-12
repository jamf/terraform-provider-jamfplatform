//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMobileApp_VPPAdminAccountRefusals proves "" and "0" are
// refused at plan time for vpp.vpp_admin_account_id (#573, wire-probed
// 2026-11-12). Jamf Pro answers 409 to both, and to "-1" while
// assign_vpp_device_based_licenses is true, so the app keeps the old location.
func TestAccResource_ProMobileApp_VPPAdminAccountRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-mobileapp-vppadmin-" + testhelpers.RunSuffix()

	config := func(id string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_app" "test" {
  general = {
    name      = %q
    version   = "1.0"
    bundle_id = "com.example.tfacc.mobileapp.vppadmin"
    os_type   = "iOS"
    is_free   = true
  }
  vpp = { vpp_admin_account_id = %s }
}
`, name, id)
	}

	steps := []resource.TestStep{}
	for _, id := range []string{`""`, `"0"`} {
		steps = append(steps, resource.TestStep{Config: config(id), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid volume purchasing location ID`)})
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    steps,
	})
}
