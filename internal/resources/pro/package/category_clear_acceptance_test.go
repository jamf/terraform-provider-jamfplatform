// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package pkg_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProPackage_CategoryClear proves "-1" is the clear value for
// category_id (#567): "" is refused at plan time, "-1" clears the category on
// the server, and the cleared state plans empty. Jamf Pro answers "" with a
// 400 INVALID_ID on /pro/v1/packages. The record is metadata-only, so the test
// needs no distribution point.
func TestAccResource_ProPackage_CategoryClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-catclear-" + suffix
	fileName := name + ".pkg"
	c := pro.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-package-catclear-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_package" "test" {
				display_name = %[2]q
				file_name    = %[3]q
				category_id  = %[4]s
			}
		`, suffix, name, fileName, categoryID)
	}
	liveCategory := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(packageResourceAddr,
			func(ctx context.Context, id string) (*pro.Package, error) { return c.GetPackageV1(ctx, id) },
			func(p *pro.Package) error { return testhelpers.RequireEqual("categoryId", want, p.CategoryID) })
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`""`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid category ID`),
			},
			{
				Config: config("jamfplatform_pro_category.clear.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(packageResourceAddr, "category_id", "jamfplatform_pro_category.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "category_id", "-1"),
					liveCategory("-1"),
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
