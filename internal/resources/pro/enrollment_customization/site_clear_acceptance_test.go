// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package enrollment_customization_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProEnrollmentCustomization_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProEnrollmentCustomization_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-enrollment-customization-siteclear-" + suffix
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_enrollment_customization.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
		  display_name = %[1]q
		  description  = "tf acc site clear"
		  site_id      = %[2]s
		  branding_settings = {
		    body_text_color   = "333333"
		    button_color      = "0066cc"
		    button_text_color = "ffffff"
		    background_color  = "ffffff"
		  }
		  text_panes = [
		    { display_name = "Welcome", rank = 0, title = "Hello", body = "Body",
		      previous_button_text = "Back", next_button_text = "Next" },
		  ]
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*pro.EnrollmentCustomizationV2, error) {
				return c.GetEnrollmentCustomizationV2(ctx, id)
			},
			func(o *pro.EnrollmentCustomizationV2) error {
				return testhelpers.RequireEqual("site", want, o.SiteID)
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("enrollment-customiza", suffix, addr, "site_id", config, live),
	})
}
