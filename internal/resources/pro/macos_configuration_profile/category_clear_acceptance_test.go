// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package macos_configuration_profile_test

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

// TestAccResource_MacOSConfigurationProfile_CategoryAndSiteClear proves "-1"
// is the clear value for general.category_id and general.site_id (#567): ""
// and "0" are refused at plan time, "-1" clears both on the server, and the
// cleared state plans empty. Before the plan-time refusal, "" was dropped from
// the request, Jamf Pro kept the old value, and the apply failed the post-apply
// consistency check; "0" draws a 409.
func TestAccResource_MacOSConfigurationProfile_CategoryAndSiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-clear-" + suffix
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")
	const addr = "jamfplatform_pro_macos_configuration_profile.test"
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))

	config := func(categoryID, siteID string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_category" "clear" {
  name     = "tf-acc-mcp-clear-cat-%[1]s"
  priority = 9
}

resource "jamfplatform_pro_site" "clear" {
  name = "tf-acc-mcp-clear-site-%[1]s"
}

resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name        = %[2]q
    category_id = %[3]s
    site_id     = %[4]s
    payloads    = <<EOF
%[5]sEOF
  }
}
`, suffix, name, categoryID, siteID, payload)
	}
	live := func(wantCategory, wantSite int) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.OsXConfigurationProfile, error) {
				return c.GetOSXConfigurationProfileByID(ctx, id)
			},
			func(p *proclassic.OsXConfigurationProfile) error {
				g := p.General
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
		CheckDestroy:             checkDestroy(t),
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
					resource.TestCheckResourceAttrPair(addr, "general.category_id", "jamfplatform_pro_category.clear", "id"),
					resource.TestCheckResourceAttrPair(addr, "general.site_id", "jamfplatform_pro_site.clear", "id"),
				),
			},
			{
				Config: config(`"-1"`, `"-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "general.category_id", "-1"),
					resource.TestCheckResourceAttr(addr, "general.site_id", "-1"),
					resource.TestCheckNoResourceAttr(addr, "general.category_name"),
					resource.TestCheckNoResourceAttr(addr, "general.site_name"),
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
