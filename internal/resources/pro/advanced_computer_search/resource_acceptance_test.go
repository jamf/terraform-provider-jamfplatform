// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /advancedcomputersearches
// endpoint. Classic has known concurrency issues when multiple writes hit the
// same resource type — keep these tests serial with any other classic
// acceptance work in this package.
//
// Design notes the acc run verifies (each is load-bearing — see the build spike):
//   - omit=clear: criteria/display_fields are removed by OMITTING the block,
//     never `= []`. Flatten returns null for an empty server wrapper; a known
//     empty list ([]) would mismatch null and surface "inconsistent result
//     after apply". The clear step (step 4) drops both blocks and relies on the
//     framework's automatic post-apply empty-plan check to prove no perma-diff.
//   - GET-after-Update: every Update step implicitly verifies the GET-after path
//     (classic Update returns 201 + empty body).
//   - display_fields is a Set: the server reorders columns, so positional checks
//     are avoided and TestCheckTypeSetElemAttr is used.
//   - criteria-less create is unprobed; the happy path always creates with >=1
//     criterion.

package advanced_computer_search_test

import (
	"context"
	"fmt"
	"regexp"
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

const acsResource = "jamfplatform_pro_advanced_computer_search.test"

func testAccCheckACSDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_advanced_computer_search" {
				continue
			}
			_, err := c.GetAdvancedComputerSearchByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking advanced computer search %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("advanced computer search %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// Step 1: create with 1 criterion + 2 display columns + site NONE.
func acsConfigCreate(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_computer_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Computer Name"
					search_type = "like"
					value       = "lab"
				},
			]

			display_fields = ["Computer Name", "Username"]
		}
	`, name)
}

// Step 2: rename + grow criteria (1->2) + grow display (2->3).
func acsConfigGrow(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_computer_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Computer Name"
					search_type = "like"
					value       = "lab"
				},
				{
					name        = "Operating System Version"
					search_type = "like"
					value       = "15"
					and_or      = "and"
				},
			]

			display_fields = ["Computer Name", "Username", "Serial Number"]
		}
	`, name)
}

// Step 3: shrink criteria (2->1) and display (3->1) — within-collection replace.
func acsConfigShrink(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_computer_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Computer Name"
					search_type = "like"
					value       = "lab"
				},
			]

			display_fields = ["Computer Name"]
		}
	`, name)
}

// Step 4: clear by OMITTING criteria + display_fields (not `= []`). The implicit
// post-apply empty-plan check proves the empty-wrapper clear round-trips with no
// perma-diff.
func acsConfigCleared(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_computer_search" "test" {
			name = %q
		}
	`, name)
}

func TestAccResource_ProAdvancedComputerSearch_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-acs-" + suffix
	renamed := "tf-acc-pro-acs-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckACSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: acsConfigCreate(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(acsResource, "id"),
					resource.TestCheckResourceAttr(acsResource, "name", name),
					resource.TestCheckResourceAttr(acsResource, "site_id", "-1"),
					// No site: site_name is null (absent) on the "-1" sentinel — the
					// server echo of "NONE" is flaky, so DerivedRefName nulls it.
					resource.TestCheckNoResourceAttr(acsResource, "site_name"),
					resource.TestCheckResourceAttr(acsResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(acsResource, "criteria.0.name", "Computer Name"),
					resource.TestCheckResourceAttr(acsResource, "criteria.0.search_type", "like"),
					resource.TestCheckResourceAttr(acsResource, "display_fields.#", "2"),
					resource.TestCheckTypeSetElemAttr(acsResource, "display_fields.*", "Computer Name"),
					resource.TestCheckTypeSetElemAttr(acsResource, "display_fields.*", "Username"),
				),
			},
			{
				Config: acsConfigGrow(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(acsResource, "name", renamed),
					resource.TestCheckResourceAttr(acsResource, "criteria.#", "2"),
					resource.TestCheckResourceAttr(acsResource, "criteria.1.name", "Operating System Version"),
					resource.TestCheckResourceAttr(acsResource, "display_fields.#", "3"),
					resource.TestCheckTypeSetElemAttr(acsResource, "display_fields.*", "Serial Number"),
				),
			},
			{
				// Import the POPULATED resource — the highest-risk round-trip.
				// Verifies server-assigned criterion priority and the reordered
				// display_fields Set survive import (Set compares order-independently).
				ResourceName:            acsResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: acsConfigShrink(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(acsResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(acsResource, "display_fields.#", "1"),
				),
			},
			{
				Config: acsConfigCleared(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(acsResource, "name", renamed),
					// Cleared: omitted blocks flatten to null. The framework's
					// automatic post-apply empty-plan check is the real assertion.
					resource.TestCheckNoResourceAttr(acsResource, "criteria.0.name"),
				),
			},
			{
				ResourceName:            acsResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProAdvancedComputerSearch_EmptyNameRejected exercises the
// name LengthAtLeast(1) validator.
func TestAccResource_ProAdvancedComputerSearch_EmptyNameRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_advanced_computer_search" "test" {
						name = ""
					}
				`,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

func TestAccDataSource_ProAdvancedComputerSearch_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-acs-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckACSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_computer_search" "test" {
						name = %q

						criteria = [
							{
								name        = "Computer Name"
								search_type = "like"
								value       = "lab"
							},
						]

						display_fields = ["Computer Name"]
					}

					data "jamfplatform_pro_advanced_computer_search" "by_id" {
						id = jamfplatform_pro_advanced_computer_search.test.id
					}

					data "jamfplatform_pro_advanced_computer_search" "by_name" {
						name = jamfplatform_pro_advanced_computer_search.test.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_advanced_computer_search.by_id", "name", acsResource, "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_advanced_computer_search.by_id", "criteria.#", "1"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_advanced_computer_search.by_name", "id", acsResource, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_advanced_computer_search.by_name", "display_fields.#", "1"),
				),
			},
		},
	})
}

func TestAccListResource_ProAdvancedComputerSearch_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-acs-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckACSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_computer_search" "test" {
						name = %q

						criteria = [
							{
								name        = "Computer Name"
								search_type = "like"
								value       = "lab"
							},
						]
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(acsResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_advanced_computer_search" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_advanced_computer_search.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_advanced_computer_search.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							// criteria is NOT in the /advancedcomputersearches summary
							// row, which carries id and name only. Asserting name alone
							// is what let a list resource emitting null criteria pass:
							// `terraform query -generate-config-out` then wrote a search
							// with no criteria, and applying it back deleted the real
							// ones. This pins the per-item GET.
							{Path: tfjsonpath.New("criteria").AtSliceIndex(0).AtMapKey("name"), KnownValue: knownvalue.StringExact("Computer Name")},
							{Path: tfjsonpath.New("criteria").AtSliceIndex(0).AtMapKey("value"), KnownValue: knownvalue.StringExact("lab")},
						},
					),
				},
			},
		},
	})
}
