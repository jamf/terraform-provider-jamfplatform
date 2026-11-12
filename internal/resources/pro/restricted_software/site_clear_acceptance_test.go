// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package restricted_software_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProRestrictedSoftware_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProRestrictedSoftware_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-restricted-software-siteclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_restricted_software.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
		  general = {
		    name         = %[1]q
		    process_name = "tf-acc-siteclear.app"
		    site_id      = %[2]s
		  }
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.RestrictedSoftware, error) {
				return c.GetRestrictedSoftwareByID(ctx, id)
			},
			func(o *proclassic.RestrictedSoftware) error {
				return testhelpers.RequireEqual("site", want, fmt.Sprint(testhelpers.Deref(o.General.Site.ID)))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRestrictedSoftwareDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("restricted-software", suffix, addr, "general.site_id", config, live),
	})
}
