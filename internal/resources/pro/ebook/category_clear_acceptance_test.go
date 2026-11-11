// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ebook_test

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

// TestAccResource_ProEbook_CategoryClear proves "-1" is the clear value for
// general.category_id (#567): "" is refused at plan time, "-1" clears the
// category on the server, and the cleared state plans empty. Before the
// plan-time refusal, "" was dropped from the request, Jamf Pro kept the old
// category, and the apply failed the post-apply consistency check.
func TestAccResource_ProEbook_CategoryClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-catclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-ebook-catclear-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_ebook" "test" {
				general = {
					name            = %[2]q
					url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
					file_type       = "PDF"
					version         = "1.0"
					deployment_type = "Make Available in Self Service"
					category_id     = %[3]s
				}
			}
		`, suffix, name, categoryID)
	}
	liveCategory := func(want int) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(ebookResourceAddr,
			func(ctx context.Context, id string) (*proclassic.Ebook, error) { return c.GetEbookByID(ctx, id) },
			func(e *proclassic.Ebook) error {
				if e.General == nil || e.General.Category == nil {
					return fmt.Errorf("general.category: absent")
				}
				return testhelpers.RequireEqual("general.category.id", want, testhelpers.Deref(e.General.Category.ID))
			})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`""`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid category ID`),
			},
			{
				Config: config("jamfplatform_pro_category.clear.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(ebookResourceAddr, "general.category_id", "jamfplatform_pro_category.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.category_id", "-1"),
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "general.category_name"),
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
