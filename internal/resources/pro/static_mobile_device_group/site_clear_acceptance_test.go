// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package static_mobile_device_group_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProStaticMobileDeviceGroup_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProStaticMobileDeviceGroup_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-static-mobile-device-gro-siteclear-" + suffix
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_static_mobile_device_group.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_static_mobile_device_group" "test" {
		  name    = %[1]q
		  site_id = %[2]s
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*pro.StaticGroup, error) {
				return c.GetStaticMobileDeviceGroupV2(ctx, id)
			},
			func(o *pro.StaticGroup) error {
				return testhelpers.RequireEqual("site", want, o.SiteID)
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("static-mobile-device", suffix, addr, "site_id", config, live),
	})
}
