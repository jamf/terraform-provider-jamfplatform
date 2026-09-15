// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /sites endpoint. Classic has known
// concurrency issues when multiple writes hit the same resource type — keep these
// tests serial with any future classic acceptance work in this package.

package site_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckSiteDestroy verifies sites created during the test were destroyed.
func testAccCheckSiteDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_site" {
				continue
			}
			_, err := c.GetSiteByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro site %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro site %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func siteConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_site" "test" {
			name = %q
		}
	`, name)
}

// TestAccResource_ProSite_Basic exercises create, in-place rename, and import for
// the classic /sites endpoint. The rename step also implicitly verifies the
// GET-after-Update path (classic UpdateSiteByID returns 201 with empty body).
func TestAccResource_ProSite_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-site-" + suffix
	renamed := "tf-acc-pro-site-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: siteConfig(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_site.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_site.test", "name", original),
				),
			},
			{
				Config: siteConfig(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_site.test", "name", renamed),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_site.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccDataSource_ProSite_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-site-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_site" "src" {
						name = %q
					}

					data "jamfplatform_pro_site" "lookup" {
						id = jamfplatform_pro_site.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_site.lookup", "name", "jamfplatform_pro_site.src", "name"),
				),
			},
		},
	})
}

func TestAccDataSource_ProSite_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-site-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_site" "src" {
						name = %q
					}

					data "jamfplatform_pro_site" "lookup" {
						name = jamfplatform_pro_site.src.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_site.lookup", "id", "jamfplatform_pro_site.src", "id"),
				),
			},
		},
	})
}

func TestAccDataSource_ProSites_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-sites-filter-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_site" "src" {
						name = %q
					}

					data "jamfplatform_pro_sites" "lookup" {
						filter = {
							name_substring = jamfplatform_pro_site.src.name
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sites.lookup", "sites.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sites.lookup", "sites.0.name", name),
				),
			},
		},
	})
}

// TestAccListResource_ProSite_Basic exercises the jamfplatform_pro_site list
// resource via the `terraform query` workflow.
func TestAccListResource_ProSite_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-site-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSiteDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_site" "src" {
						name = %q
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_site.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_site" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_site.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_site.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
						},
					),
				},
			},
		},
	})
}
