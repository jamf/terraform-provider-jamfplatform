// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package advanced_volume_purchasing_content_search_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProAdvancedVolumePurchasingContentSearch_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProAdvancedVolumePurchasingContentSearch_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-advanced-volume-purchasi-siteclear-" + suffix
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_advanced_volume_purchasing_content_search.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
		  name    = %[1]q
		  site_id = %[2]s
		  criteria = [
		    { name = "Name", search_type = "like", value = "Office" },
		  ]
		  display_fields = ["Name"]
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*pro.AdvancedUserContentSearch, error) {
				return c.GetAdvancedUserContentSearchV1(ctx, id)
			},
			func(o *pro.AdvancedUserContentSearch) error {
				return testhelpers.RequireEqual("site", want, testhelpers.Deref(o.SiteID))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAVPCSDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("advanced-volume-purc", suffix, addr, "site_id", config, live),
	})
}
