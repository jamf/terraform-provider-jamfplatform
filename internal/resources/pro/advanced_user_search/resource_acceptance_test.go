// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /advancedusersearches endpoint.
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any other classic acceptance work
// in this package.
//
// Design notes the acc run verifies (see the build spike):
//   - omit=clear: criteria/display_fields are removed by OMITTING the block,
//     never `= []` (flatten returns null for an empty server wrapper).
//   - GET-after-Update: every Update step implicitly verifies the GET-after path.
//   - display_fields is a Set (server reorders columns).
//   - operator subset: TestAcc...DateWindowOperatorRejected locks the decision to
//     drop the two date-window operators on user searches. NOTE: the live wire
//     *accepts* this operator at the API (a 201, stored verbatim) — the rejection
//     is a deliberate provider-side validator, so this test guards a choice the
//     server does not enforce.
//   - criteria-less create is unprobed; the happy path always creates with >=1
//     criterion.

package advanced_user_search_test

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

const ausResource = "jamfplatform_pro_advanced_user_search.test"

func testAccCheckAUSDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_advanced_user_search" {
				continue
			}
			_, err := c.GetAdvancedUserSearchByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking advanced user search %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("advanced user search %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func ausConfigCreate(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_user_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Full Name"
					search_type = "like"
					value       = "a"
				},
			]

			display_fields = ["Full Name", "Email Address"]
		}
	`, name)
}

func ausConfigGrow(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_user_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Full Name"
					search_type = "like"
					value       = "a"
				},
				{
					name        = "Email Address"
					search_type = "like"
					value       = "@example.com"
					and_or      = "and"
				},
			]

			display_fields = ["Full Name", "Email Address", "Username"]
		}
	`, name)
}

func ausConfigShrink(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_user_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Full Name"
					search_type = "like"
					value       = "a"
				},
			]

			display_fields = ["Full Name"]
		}
	`, name)
}

func ausConfigCleared(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_user_search" "test" {
			name = %q
		}
	`, name)
}

func TestAccResource_ProAdvancedUserSearch_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-aus-" + suffix
	renamed := "tf-acc-pro-aus-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAUSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ausConfigCreate(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ausResource, "id"),
					resource.TestCheckResourceAttr(ausResource, "name", name),
					resource.TestCheckResourceAttr(ausResource, "site_id", "-1"),
					// No site: site_name is null (absent) on the "-1" sentinel — the
					// server echo of "NONE" is flaky, so DerivedRefName nulls it.
					resource.TestCheckNoResourceAttr(ausResource, "site_name"),
					resource.TestCheckResourceAttr(ausResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(ausResource, "criteria.0.name", "Full Name"),
					resource.TestCheckResourceAttr(ausResource, "display_fields.#", "2"),
					resource.TestCheckTypeSetElemAttr(ausResource, "display_fields.*", "Email Address"),
				),
			},
			{
				Config: ausConfigGrow(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ausResource, "name", renamed),
					resource.TestCheckResourceAttr(ausResource, "criteria.#", "2"),
					resource.TestCheckResourceAttr(ausResource, "criteria.1.name", "Email Address"),
					resource.TestCheckResourceAttr(ausResource, "display_fields.#", "3"),
					resource.TestCheckTypeSetElemAttr(ausResource, "display_fields.*", "Username"),
				),
			},
			{
				// Import the POPULATED resource — the highest-risk round-trip.
				// Verifies server-assigned criterion priority and the reordered
				// display_fields Set survive import (Set compares order-independently).
				ResourceName:            ausResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: ausConfigShrink(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ausResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(ausResource, "display_fields.#", "1"),
				),
			},
			{
				Config: ausConfigCleared(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ausResource, "name", renamed),
					resource.TestCheckNoResourceAttr(ausResource, "criteria.0.name"),
				),
			},
			{
				ResourceName:            ausResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProAdvancedUserSearch_DateWindowOperatorRejected locks the Q1
// decision: user searches use the user-group operator subset, so the two
// date-window operators are rejected by the provider's OneOf validator at plan
// time — even though the live API accepts them. The regex uses \s+ to survive
// Terraform's ~80-column line wrapping of the validator detail.
func TestAccResource_ProAdvancedUserSearch_DateWindowOperatorRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_advanced_user_search" "test" {
						name = "tf-acc-aus-bad-op"

						criteria = [
							{
								name        = "Email Address"
								search_type = "in more than x days"
								value       = "7"
							},
						]
					}
				`,
				ExpectError: regexp.MustCompile(`value\s+must\s+be\s+one\s+of`),
			},
		},
	})
}

// TestAccResource_ProAdvancedUserSearch_EmptyNameRejected exercises the name
// LengthAtLeast(1) validator.
func TestAccResource_ProAdvancedUserSearch_EmptyNameRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_advanced_user_search" "test" {
						name = ""
					}
				`,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

func TestAccDataSource_ProAdvancedUserSearch_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-aus-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAUSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_user_search" "test" {
						name = %q

						criteria = [
							{
								name        = "Full Name"
								search_type = "like"
								value       = "a"
							},
						]

						display_fields = ["Full Name"]
					}

					data "jamfplatform_pro_advanced_user_search" "by_id" {
						id = jamfplatform_pro_advanced_user_search.test.id
					}

					data "jamfplatform_pro_advanced_user_search" "by_name" {
						name = jamfplatform_pro_advanced_user_search.test.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_advanced_user_search.by_id", "name", ausResource, "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_advanced_user_search.by_id", "criteria.#", "1"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_advanced_user_search.by_name", "id", ausResource, "id"),
				),
			},
		},
	})
}

func TestAccListResource_ProAdvancedUserSearch_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-aus-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAUSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_user_search" "test" {
						name = %q

						criteria = [
							{
								name        = "Full Name"
								search_type = "like"
								value       = "a"
							},
						]
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(ausResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_advanced_user_search" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_advanced_user_search.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_advanced_user_search.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							// criteria is NOT in the /advancedusersearches summary row,
							// which carries id and name only. Asserting name alone is
							// what let a list resource emitting null criteria pass:
							// `terraform query -generate-config-out` then wrote a search
							// with no criteria, and applying it back deleted the real
							// ones. This pins the per-item GET.
							{Path: tfjsonpath.New("criteria").AtSliceIndex(0).AtMapKey("name"), KnownValue: knownvalue.StringExact("Full Name")},
							{Path: tfjsonpath.New("criteria").AtSliceIndex(0).AtMapKey("value"), KnownValue: knownvalue.StringExact("a")},
						},
					),
				},
			},
		},
	})
}
