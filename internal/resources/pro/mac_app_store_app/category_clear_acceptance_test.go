// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMacApp_CategoryClear proves "-1" is the clear value for
// general.category_id (#567): "" is refused at plan time, "-1" clears the
// category on the server, and the cleared state plans empty. Before the
// plan-time refusal, "" was dropped from the request, Jamf Pro kept the old
// category, and the apply failed the post-apply consistency check.
func TestAccResource_ProMacApp_CategoryClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-catclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-macapp-catclear-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_mac_app_store_app" "test" {
				general = {
					name        = %[2]q
					version     = "1.0"
					bundle_id   = "com.example.tfacc.macapp.catclear"
					url         = "https://apps.apple.com/app/id000000001"
					category_id = %[3]s
				}
			}
		`, suffix, name, categoryID)
	}
	liveCategory := func(want int) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(macAppResourceAddr,
			func(ctx context.Context, id string) (*proclassic.MacApplication, error) {
				return c.GetMacApplicationByID(ctx, id)
			},
			func(a *proclassic.MacApplication) error {
				if a.General == nil || a.General.Category == nil {
					return fmt.Errorf("general.category: absent")
				}
				return testhelpers.RequireEqual("general.category.id", want, testhelpers.Deref(a.General.Category.ID))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`""`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid category ID`),
			},
			{
				Config: config("jamfplatform_pro_category.clear.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "general.category_id", "jamfplatform_pro_category.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.category_id", "-1"),
					liveCategory(-1),
				),
			},
			{
				Config: config(`"-1"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
