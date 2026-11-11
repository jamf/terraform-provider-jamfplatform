// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_installer_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProAppInstaller_CategoryAndSiteClear proves "-1" is the
// clear value for category_id and site_id (#567): "" and "0" are refused at
// plan time, "-1" clears both on the server, and the cleared state plans
// empty. Jamf Pro answers "" and "0" with 400 INVALID_ID on this endpoint.
//
// The deployment stays disabled, so it needs no smart group, and it is
// siteless again before destroy, so the delete is not site-scoped.
func TestAccResource_ProAppInstaller_CategoryAndSiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-app-installer-clear-" + suffix
	c := pro.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID, siteID string) string {
		return catalogDS + fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-ai-clear-cat-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_site" "clear" {
				name = "tf-acc-ai-clear-site-%[1]s"
			}
			resource "jamfplatform_pro_app_installer" "test" {
				name            = %[2]q
				app_title_name  = %[3]s
				deployment_type = "SELF_SERVICE"
				update_behavior = "AUTOMATIC"
				category_id     = %[4]s
				site_id         = %[5]s
			}
		`, suffix, name, titleNameRef, categoryID, siteID)
	}
	live := func(wantCategory, wantSite string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(resourceAddr,
			func(ctx context.Context, id string) (*pro.AppTitleDeploymentRead, error) {
				return c.GetAppInstallerDeploymentV1(ctx, id)
			},
			func(d *pro.AppTitleDeploymentRead) error {
				return errors.Join(
					testhelpers.RequireEqual("categoryId", wantCategory, d.CategoryID),
					testhelpers.RequireEqual("siteId", wantSite, d.SiteID),
				)
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppInstallerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`""`, `"-1"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid category ID`),
			},
			{
				Config:      config(`"-1"`, `"0"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
			{
				Config: config("jamfplatform_pro_category.clear.id", "jamfplatform_pro_site.clear.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(resourceAddr, "category_id", "jamfplatform_pro_category.clear", "id"),
					resource.TestCheckResourceAttrPair(resourceAddr, "site_id", "jamfplatform_pro_site.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`, `"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddr, "category_id", "-1"),
					resource.TestCheckResourceAttr(resourceAddr, "site_id", "-1"),
					live("-1", "-1"),
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
