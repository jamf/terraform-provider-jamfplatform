//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMacApp_VPPAdminAccountRefusals proves "" and "0" are refused
// at plan time for vpp.vpp_admin_account_id (#573, wire-probed 2026-11-12).
// Jamf Pro answers 409 to both, and to "-1" while
// assign_vpp_device_based_licenses is true, so the app keeps the old location.
func TestAccResource_ProMacApp_VPPAdminAccountRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-macapp-vppadmin-" + testhelpers.RunSuffix()

	config := func(id string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_mac_app_store_app" "test" {
  general = {
    name            = %q
    version         = "1.0"
    bundle_id       = "com.example.tfacc.macapp"
    url             = "https://apps.apple.com/app/id000000001"
    deployment_type = "Make Available in Self Service"
  }
  scope = { all_computers = true }
  vpp   = { vpp_admin_account_id = %s }
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
