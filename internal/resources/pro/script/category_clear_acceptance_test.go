// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package script_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProScript_CategoryClear proves "-1" is the clear value for
// category_id (#567): "" is refused at plan time, "-1" clears the category on
// the server, and the cleared state plans empty. Jamf Pro answers "" with a
// 500 on /pro/v1/scripts, which is why the plan-time refusal exists.
func TestAccResource_ProScript_CategoryClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-script-catclear-" + suffix
	c := pro.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-script-catclear-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_script" "test" {
				name            = %[2]q
				priority        = "AFTER"
				script_contents = "#!/bin/sh\necho catclear"
				category_id     = %[3]s
			}
		`, suffix, name, categoryID)
	}
	liveCategory := func(want string) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject("jamfplatform_pro_script.test",
			func(ctx context.Context, id string) (*pro.Script, error) { return c.GetScriptV1(ctx, id) },
			func(s *pro.Script) error {
				return testhelpers.RequireEqual("categoryId", want, helpers.DerefString(s.CategoryID))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`""`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid category ID`),
			},
			{
				Config: config("jamfplatform_pro_category.clear.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("jamfplatform_pro_script.test", "category_id", "jamfplatform_pro_category.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "category_id", "-1"),
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
