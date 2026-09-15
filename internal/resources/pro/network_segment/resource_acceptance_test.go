// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /networksegments endpoint. Classic has
// known concurrency issues when multiple writes hit the same resource type — keep these
// tests serial with any other classic acceptance work in this package.

package network_segment_test

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

// testAccCheckNetworkSegmentDestroy verifies network segments created during the test
// were destroyed.
func testAccCheckNetworkSegmentDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_network_segment" {
				continue
			}
			_, err := c.GetNetworkSegmentByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro network segment %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro network segment %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func networkSegmentConfigMinimal(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_network_segment" "test" {
			name             = %q
			starting_address = "10.10.10.0"
			ending_address   = "10.10.10.255"
		}
	`, name)
}

func networkSegmentConfigRenamedAndRanged(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_network_segment" "test" {
			name             = %q
			starting_address = "10.10.11.0"
			ending_address   = "10.10.11.255"
		}
	`, name)
}

// networkSegmentConfigAllFields sets every optional writable field so the override
// toggles and their companion strings are exercised end-to-end. The classic API
// resolves `building` / `department` to existing Building / Department records by
// name and silently drops references it cannot resolve — so the building and
// department must be pre-created in the same config.
func networkSegmentConfigAllFields(name, building, department string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "ns_fixture" {
			name = %q
		}

		resource "jamfplatform_pro_department" "ns_fixture" {
			name = %q
		}

		resource "jamfplatform_pro_network_segment" "test" {
			name             = %q
			starting_address = "10.10.12.0"
			ending_address   = "10.10.12.255"

			building             = jamfplatform_pro_building.ns_fixture.name
			department           = jamfplatform_pro_department.ns_fixture.name
			override_buildings   = true
			override_departments = true
		}
	`, building, department, name)
}

const networkSegmentResourceAddr = "jamfplatform_pro_network_segment.test"

// networkSegmentLive fetches the server's copy of the segment under test and
// hands it to assert, so the drop step can prove on the wire that the
// always-emitted empty elements cleared building/department and turned the
// override flags off — a null in state cannot tell a cleared value from one
// the classic merge retained and the state builder reconciled away (#384).
func networkSegmentLive(t *testing.T, assert func(*proclassic.NetworkSegment) error) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(networkSegmentResourceAddr, c.GetNetworkSegmentByID, assert)
}

// TestAccResource_ProNetworkSegment_Basic exercises create, in-place rename + IP range
// change, populate-all-then-drop on the optional building/department + override fields,
// and import for the classic /networksegments endpoint. The rename and clear-on-omit
// steps implicitly verify the GET-after-Update path (classic Update returns 201 + empty
// body) and the Optional drift reconciliation in state_builders.go; the drop step also
// asserts the clear on the wire (#384).
func TestAccResource_ProNetworkSegment_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-ns-" + suffix
	renamed := "tf-acc-pro-ns-renamed-" + suffix
	allFields := "tf-acc-pro-ns-all-" + suffix
	buildingName := "tf-acc-ns-building-" + suffix
	departmentName := "tf-acc-ns-department-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkSegmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: networkSegmentConfigMinimal(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_network_segment.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "name", original),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "starting_address", "10.10.10.0"),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "ending_address", "10.10.10.255"),
				),
			},
			{
				Config: networkSegmentConfigAllFields(allFields, buildingName, departmentName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "name", allFields),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "building", buildingName),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "department", departmentName),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "override_buildings", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "override_departments", "true"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_building.ns_fixture", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_department.ns_fixture", "id"),
				),
			},
			{
				Config: networkSegmentConfigRenamedAndRanged(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "name", renamed),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "starting_address", "10.10.11.0"),
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "ending_address", "10.10.11.255"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_network_segment.test", "building"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_network_segment.test", "department"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_network_segment.test", "override_buildings"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_network_segment.test", "override_departments"),
					networkSegmentLive(t, func(s *proclassic.NetworkSegment) error {
						if err := testhelpers.RequireEqual("building", "", testhelpers.Deref(s.Building)); err != nil {
							return err
						}
						if err := testhelpers.RequireEqual("department", "", testhelpers.Deref(s.Department)); err != nil {
							return err
						}
						if err := testhelpers.RequireEqual("override_buildings", false, testhelpers.Deref(s.OverrideBuildings)); err != nil {
							return err
						}
						return testhelpers.RequireEqual("override_departments", false, testhelpers.Deref(s.OverrideDepartments))
					}),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_network_segment.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccDataSource_ProNetworkSegment_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ns-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkSegmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_network_segment" "src" {
						name             = %q
						starting_address = "10.20.0.0"
						ending_address   = "10.20.0.255"
					}

					data "jamfplatform_pro_network_segment" "lookup" {
						id = jamfplatform_pro_network_segment.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_network_segment.lookup", "name", "jamfplatform_pro_network_segment.src", "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_network_segment.lookup", "starting_address", "jamfplatform_pro_network_segment.src", "starting_address"),
				),
			},
		},
	})
}

func TestAccDataSource_ProNetworkSegment_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ns-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkSegmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_network_segment" "src" {
						name             = %q
						starting_address = "10.30.0.0"
						ending_address   = "10.30.0.255"
					}

					data "jamfplatform_pro_network_segment" "lookup" {
						name = jamfplatform_pro_network_segment.src.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_network_segment.lookup", "id", "jamfplatform_pro_network_segment.src", "id"),
				),
			},
		},
	})
}

func TestAccDataSource_ProNetworkSegments_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ns-filter-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkSegmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_network_segment" "src" {
						name             = %q
						starting_address = "10.40.0.0"
						ending_address   = "10.40.0.255"
					}

					data "jamfplatform_pro_network_segments" "lookup" {
						filter = {
							name_substring = jamfplatform_pro_network_segment.src.name
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_network_segments.lookup", "network_segments.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_network_segments.lookup", "network_segments.0.name", name),
				),
			},
		},
	})
}

// TestAccListResource_ProNetworkSegment_Basic exercises the jamfplatform_pro_network_segment
// list resource via the `terraform query` workflow.
func TestAccListResource_ProNetworkSegment_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ns-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkSegmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_network_segment" "src" {
						name             = %q
						starting_address = "10.50.0.0"
						ending_address   = "10.50.0.255"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_network_segment.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_network_segment" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_network_segment.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_network_segment.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("starting_address"), KnownValue: knownvalue.StringExact("10.50.0.0")},
						},
					),
				},
			},
		},
	})
}

// TestAccListResource_ProNetworkSegment_HydratesBeyondTheSummaryRow pins that a
// list result carries the attributes the /networksegments summary row does not.
//
// The summary carries id, name, starting_address and ending_address only, and
// the list resource used to emit nulls for everything else. That is invisible to
// a query test asserting only summary fields — which is what
// TestAccListResource_ProNetworkSegment_Basic does, and why it passed
// throughout — but it is not invisible to `terraform query
// -generate-config-out`: the generated configuration carried no building,
// department or override flags, and applying it back would have cleared them.
//
// So this asserts a field that can only have come from the per-item GET. It
// belongs here rather than on testhelpers.GenerateConfigStep, which adopts
// through the singular read and never touches the list resource at all.
func TestAccListResource_ProNetworkSegment_HydratesBeyondTheSummaryRow(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ns-hydrate-" + suffix
	buildingName := "tf-acc-ns-hydrate-building-" + suffix
	departmentName := "tf-acc-ns-hydrate-department-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkSegmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: networkSegmentConfigAllFields(name, buildingName, departmentName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_network_segment.test", "building", buildingName),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_network_segment" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_network_segment.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_network_segment.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							// Not in the summary row: these are the fix.
							{Path: tfjsonpath.New("building"), KnownValue: knownvalue.StringExact(buildingName)},
							{Path: tfjsonpath.New("department"), KnownValue: knownvalue.StringExact(departmentName)},
							{Path: tfjsonpath.New("override_buildings"), KnownValue: knownvalue.Bool(true)},
							{Path: tfjsonpath.New("override_departments"), KnownValue: knownvalue.Bool(true)},
						},
					),
				},
			},
		},
	})
}
