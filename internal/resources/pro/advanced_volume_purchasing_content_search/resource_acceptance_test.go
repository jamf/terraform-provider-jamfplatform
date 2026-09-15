// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro /v1/advanced-user-content-searches
// endpoint (the admin UI calls these Advanced Volume Purchasing Content Searches).
//
// Design notes the acc run verifies (each is load-bearing — see the build spike):
//   - full-replace PUT, omit=preserve: criteria/display_fields are Optional+Computed
//     with UseStateForUnknown, so OMITTING the block PRESERVES the current value;
//     clearing requires an explicit `= []`. Flatten returns a known EMPTY collection
//     (not null) so an explicit `[]` round-trips with no "inconsistent result after
//     apply". criteria is a types.List (Computed nested collection cannot be a Go
//     slice — STYLE_GUIDE §Computed nested collections).
//   - GET-after-write: every Create/Update step implicitly verifies the GET-after
//     path (the Pro PUT response body echoes the submitted display fields and
//     silently drops invalid ones, so state must come from a fresh GET).
//   - display_fields is a Set: the server reorders columns, so positional checks
//     are avoided and TestCheckTypeSetElemAttr is used.
//   - criteria/display-field names use Jamf Pro's WIRE vocabulary, which differs
//     from the UI labels (UI "Content Name" -> wire "Name", "Price" -> "Cost",
//     "Total Content" -> "Total", "Username" -> "Username").

package advanced_volume_purchasing_content_search_test

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const avpcsResource = "jamfplatform_pro_advanced_volume_purchasing_content_search.test"

func testAccCheckAVPCSDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_advanced_volume_purchasing_content_search" {
				continue
			}
			_, err := c.GetAdvancedUserContentSearchV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking advanced volume purchasing content search %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("advanced volume purchasing content search %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// Step 1: create with 1 criterion + 2 display columns + site NONE.
func avpcsConfigCreate(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Name"
					search_type = "like"
					value       = "Office"
				},
			]

			display_fields = ["Name", "Cost"]
		}
	`, name)
}

// Step 2: rename + grow criteria (1->2) + grow display (2->3).
func avpcsConfigGrow(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Name"
					search_type = "like"
					value       = "Office"
				},
				{
					name        = "Username"
					search_type = "is"
					value       = "alice"
					and_or      = "and"
				},
			]

			display_fields = ["Name", "Cost", "Total"]
		}
	`, name)
}

// Step 3: shrink criteria (2->1) and display (3->1) — within-collection replace.
func avpcsConfigShrink(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Name"
					search_type = "like"
					value       = "Office"
				},
			]

			display_fields = ["Name"]
		}
	`, name)
}

// avpcsConfigOmitted omits criteria + display_fields entirely. Under
// Optional+Computed+UseStateForUnknown this PRESERVES the prior values (omit =
// preserve), not clears.
func avpcsConfigOmitted(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
			name = %q
		}
	`, name)
}

// avpcsConfigCleared clears criteria + display_fields with explicit `= []`. The
// known-empty collections must round-trip (no "inconsistent result after apply").
func avpcsConfigCleared(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
			name           = %q
			criteria       = []
			display_fields = []
		}
	`, name)
}

func TestAccResource_ProAdvancedVolumePurchasingContentSearch_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-avpcs-" + suffix
	renamed := "tf-acc-pro-avpcs-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAVPCSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: avpcsConfigCreate(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(avpcsResource, "id"),
					resource.TestCheckResourceAttr(avpcsResource, "name", name),
					resource.TestCheckResourceAttr(avpcsResource, "site_id", "-1"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.0.name", "Name"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.0.search_type", "like"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "2"),
					resource.TestCheckTypeSetElemAttr(avpcsResource, "display_fields.*", "Name"),
					resource.TestCheckTypeSetElemAttr(avpcsResource, "display_fields.*", "Cost"),
				),
			},
			{
				Config: avpcsConfigGrow(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(avpcsResource, "name", renamed),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "2"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.1.name", "Username"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "3"),
					resource.TestCheckTypeSetElemAttr(avpcsResource, "display_fields.*", "Total"),
				),
			},
			{
				// Import the POPULATED resource — the highest-risk round-trip.
				// Verifies server-assigned criterion priority and the reordered
				// display_fields Set survive import (Set compares order-independently).
				ResourceName:            avpcsResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: avpcsConfigShrink(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "1"),
				),
			},
			{
				// Omit both collections: omit=preserve carries the prior values forward
				// (criteria 1, display 1), NOT cleared.
				Config: avpcsConfigOmitted(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.0.name", "Name"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "1"),
				),
			},
			{
				// Explicit `= []` clears both; the known-empty collections round-trip.
				Config: avpcsConfigCleared(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(avpcsResource, "name", renamed),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "0"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "0"),
				),
			},
			{
				ResourceName:            avpcsResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProAdvancedVolumePurchasingContentSearch_EmptyNameRejected
// exercises the name LengthAtLeast(1) validator.
// TestAccResource_ProAdvancedVolumePurchasingContentSearch_SplitOwnership proves the
// omit=preserve contract for criteria/display_fields (Optional+Computed on the
// full-replace endpoint), plus the §Computed-collection risk paths: create with
// both collections OMITTED (Unknown → known empty, no decode error); an out-of-band
// criterion survives an unrelated apply; explicit `= []` clears and round-trips.
func TestAccResource_ProAdvancedVolumePurchasingContentSearch_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-avpcs-split-" + suffix

	var searchID string

	addCriterionOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetAdvancedUserContentSearchV1(ctx, searchID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		pri := 0
		opening, closing := false, false
		got.Criteria = &[]pro.SmartSearchCriterion{
			{Name: "Name", SearchType: "like", Value: "Office", AndOr: "and", Priority: &pri, OpeningParen: &opening, ClosingParen: &closing},
		}
		if _, err := c.UpdateAdvancedUserContentSearchV1(ctx, searchID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerCriteriaLen := func(want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetAdvancedUserContentSearchV1(context.Background(), searchID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			n := 0
			if got.Criteria != nil {
				n = len(*got.Criteria)
			}
			if n != want {
				return fmt.Errorf("server criteria len = %d, want %d", n, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAVPCSDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with criteria + display_fields OMITTED — the create-omit path.
				Config: avpcsConfigOmitted(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(avpcsResource, "id"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "0"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "0"),
					func(s *terraform.State) error {
						searchID = s.RootModule().Resources[avpcsResource].Primary.ID
						return nil
					},
				),
			},
			{
				// A criterion is added out of band; config still omits criteria and
				// changes only display_fields. The out-of-band criterion must survive.
				PreConfig: addCriterionOutOfBand,
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
						name           = %q
						display_fields = ["Name"]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "1"),
					resource.TestCheckResourceAttr(avpcsResource, "criteria.0.name", "Name"),
					resource.TestCheckResourceAttr(avpcsResource, "display_fields.#", "1"),
					checkServerCriteriaLen(1),
				),
			},
			{
				// Explicit `= []` clears criteria; display_fields kept by declaration.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
						name           = %q
						criteria       = []
						display_fields = ["Name"]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(avpcsResource, "criteria.#", "0"),
					checkServerCriteriaLen(0),
				),
			},
		},
	})
}

func TestAccResource_ProAdvancedVolumePurchasingContentSearch_EmptyNameRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
						name = ""
					}
				`,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

// TestAccResource_ProAdvancedVolumePurchasingContentSearch_SubsetOperatorRejected
// exercises the content operator subset: `member of` is a valid canonical operator
// but is excluded from the Volume-Purchasing-Content vocabulary, so the provider
// must reject it at plan time (proving the criteria.Without subset is wired up).
func TestAccResource_ProAdvancedVolumePurchasingContentSearch_SubsetOperatorRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
						name = "tf-acc-bad-op"
						criteria = [
							{
								name        = "Name"
								search_type = "member of"
								value       = "x"
							},
						]
					}
				`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

func TestAccDataSource_ProAdvancedVolumePurchasingContentSearch_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-avpcs-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAVPCSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
						name = %q

						criteria = [
							{
								name        = "Name"
								search_type = "like"
								value       = "Office"
							},
						]

						display_fields = ["Name"]
					}

					data "jamfplatform_pro_advanced_volume_purchasing_content_search" "by_id" {
						id = jamfplatform_pro_advanced_volume_purchasing_content_search.test.id
					}

					data "jamfplatform_pro_advanced_volume_purchasing_content_search" "by_name" {
						name = jamfplatform_pro_advanced_volume_purchasing_content_search.test.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_advanced_volume_purchasing_content_search.by_id", "name", avpcsResource, "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_advanced_volume_purchasing_content_search.by_id", "criteria.#", "1"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_advanced_volume_purchasing_content_search.by_name", "id", avpcsResource, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_advanced_volume_purchasing_content_search.by_name", "display_fields.#", "1"),
				),
			},
		},
	})
}

func TestAccListResource_ProAdvancedVolumePurchasingContentSearch_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-avpcs-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAVPCSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
						name = %q

						criteria = [
							{
								name        = "Name"
								search_type = "like"
								value       = "Office"
							},
						]
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(avpcsResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_advanced_volume_purchasing_content_search.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_advanced_volume_purchasing_content_search.test",
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
