// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMacApp_CategoryAndSiteClear proves "-1" is the clear
// value for general.category_id and general.site_id (#567): "" and "0" are
// refused at plan time, "-1" clears both on the server, and the cleared state
// plans empty. The input builder drops "", and Jamf Pro keeps the old value
// when the element is omitted, so before the plan-time refusal "" failed the
// post-apply consistency check; "0" draws a 409.
func TestAccResource_ProMacApp_CategoryAndSiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-catclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID, siteID string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-macapp-catclear-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_site" "clear" {
				name = "tf-acc-macapp-siteclear-%[1]s"
			}
			resource "jamfplatform_pro_mac_app_store_app" "test" {
				general = {
					name        = %[2]q
					version     = "1.0"
					bundle_id   = "com.example.tfacc.macapp.catclear"
					url         = "https://apps.apple.com/app/id000000001"
					category_id = %[3]s
					site_id     = %[4]s
				}
			}
		`, suffix, name, categoryID, siteID)
	}
	live := func(wantCategory, wantSite int) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(macAppResourceAddr,
			func(ctx context.Context, id string) (*proclassic.MacApplication, error) {
				return c.GetMacApplicationByID(ctx, id)
			},
			func(a *proclassic.MacApplication) error {
				g := a.General
				if g == nil || g.Category == nil || g.Site == nil {
					return fmt.Errorf("general.category or general.site: absent")
				}
				return errors.Join(
					testhelpers.RequireEqual("general.category.id", wantCategory, testhelpers.Deref(g.Category.ID)),
					testhelpers.RequireEqual("general.site.id", wantSite, testhelpers.Deref(g.Site.ID)),
				)
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`""`, `"-1"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid category ID`),
			},
			{
				Config:      config(`"-1"`, `""`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
			{
				Config:      config(`"-1"`, `"0"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
			{
				Config: config("jamfplatform_pro_category.clear.id", "jamfplatform_pro_site.clear.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "general.category_id", "jamfplatform_pro_category.clear", "id"),
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "general.site_id", "jamfplatform_pro_site.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`, `"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.category_id", "-1"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.site_id", "-1"),
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "general.site_name"),
					live(-1, -1),
				),
			},
			{
				Config: config(`"-1"`, `"-1"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
