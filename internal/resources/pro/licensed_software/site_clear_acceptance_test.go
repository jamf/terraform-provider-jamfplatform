// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package licensed_software_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProLicensedSoftware_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProLicensedSoftware_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-licensed-software-siteclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_licensed_software.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
		  name    = %[1]q
		  site_id = %[2]s
		  software_definitions = [
		    { name = "Acme Editor", version = "1.0", compare_type = "is" },
		  ]
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.LicensedSoftware, error) {
				return c.GetLicensedSoftwareByID(ctx, id)
			},
			func(o *proclassic.LicensedSoftware) error {
				return testhelpers.RequireEqual("site", want, fmt.Sprint(testhelpers.Deref(o.General.Site.ID)))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("licensed-software", suffix, addr, "site_id", config, live),
	})
}
