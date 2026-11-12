// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

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

// TestAccResource_ProPolicy_CategoryAndSiteClear proves "-1" is the clear value
// for general.category_id and general.site_id (#573): "" and "0" are refused at
// plan time, "-1" clears both on the server, and the cleared state plans empty.
// The input builder drops "", and Jamf Pro keeps the old value when the element
// is omitted, so before the plan-time refusal "" failed the post-apply
// consistency check; "0" draws a 409.
func TestAccResource_ProPolicy_CategoryAndSiteClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-refclear-" + suffix
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	const addr = "jamfplatform_pro_policy.test"

	config := func(categoryID, siteID string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_category" "clear" {
				name     = "tf-acc-policy-catclear-%[1]s"
				priority = 9
			}
			resource "jamfplatform_pro_site" "clear" {
				name = "tf-acc-policy-siteclear-%[1]s"
			}
			resource "jamfplatform_pro_policy" "test" {
				general = {
					name        = %[2]q
					category_id = %[3]s
					site_id     = %[4]s
				}
			}
		`, suffix, name, categoryID, siteID)
	}
	live := func(wantCategory, wantSite int) resource.TestCheckFunc {
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.Policy, error) { return c.GetPolicyByID(ctx, id) },
			func(p *proclassic.Policy) error {
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
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{Config: config(`""`, `"-1"`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid category ID`)},
			{Config: config(`"0"`, `"-1"`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid category ID`)},
			{Config: config(`"-1"`, `""`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid site ID`)},
			{Config: config(`"-1"`, `"0"`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid site ID`)},
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

// TestAccResource_ProPolicy_UnclearableReferencesRefused proves the two
// references Jamf Pro cannot clear are refused at plan time (#573): the Self
// Service icon (every body returns 201 or 409 and the icon stays) and the disk
// encryption configurations (cleared only by changing the action or key type).
func TestAccResource_ProPolicy_UnclearableReferencesRefused(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-unclearable-" + suffix

	icon := func(id string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_policy" "test" {
				general      = { name = %q }
				self_service = { self_service_icon = { id = %s } }
			}
		`, name, id)
	}
	diskEncryption := func(id string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_policy" "test" {
				general         = { name = %q }
				disk_encryption = { action = "apply", disk_encryption_configuration_id = %s }
			}
		`, name, id)
	}

	remediate := func(id string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_policy" "test" {
				general         = { name = %q }
				disk_encryption = { action = "remediate", remediate_key_type = "Institutional", remediate_disk_encryption_configuration_id = %s }
			}
		`, name, id)
	}

	steps := []resource.TestStep{}
	for _, id := range []string{`""`, `"-1"`, `"0"`} {
		steps = append(steps, resource.TestStep{Config: icon(id), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid icon ID`)})
	}
	for _, id := range []string{"-1", "0"} {
		steps = append(steps, resource.TestStep{Config: diskEncryption(id), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid disk encryption configuration ID`)})
	}
	for _, id := range []string{"-1", "0"} {
		steps = append(steps, resource.TestStep{Config: remediate(id), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid disk encryption configuration ID`)})
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    steps,
	})
}
