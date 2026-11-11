// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package advanced_mobile_device_search_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProAdvancedMobileDeviceSearch_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProAdvancedMobileDeviceSearch_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-advanced-mobile-device-s-siteclear-" + suffix
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_advanced_mobile_device_search.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_mobile_device_search" "test" {
		  name    = %[1]q
		  site_id = %[2]s
		  criteria = [
		    { name = "Managed", search_type = "is", value = "Unmanaged" },
		  ]
		  display_fields = ["Display Name"]
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*pro.AdvancedSearch, error) {
				return c.GetAdvancedMobileDeviceSearchV1(ctx, id)
			},
			func(o *pro.AdvancedSearch) error {
				return testhelpers.RequireEqual("site", want, testhelpers.Deref(o.SiteID))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAMDSDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("advanced-mobile-devi", suffix, addr, "site_id", config, live),
	})
}
