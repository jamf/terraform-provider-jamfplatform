// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package class_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProClass_SiteClear proves "-1" is the only value that
// clears site_id (#573): "" and "0" are refused at plan time, "-1" clears the
// site on the server, and the cleared state plans empty.
func TestAccResource_ProClass_SiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-class-siteclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_class.test"

	config := func(siteExpr string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_class" "test" {
		  name    = %[1]q
		  site_id = %[2]s
		}
`, name, siteExpr)
	}
	live := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.Class, error) { return c.GetClassByID(ctx, id) },
			func(o *proclassic.Class) error {
				return testhelpers.RequireEqual("site", want, fmt.Sprint(testhelpers.Deref(o.Site.ID)))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckClassDestroy(t),
		Steps:                    testhelpers.SiteClearSteps("class", suffix, addr, "site_id", config, live),
	})
}
