// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package building_test

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckBuildingDestroy verifies buildings created during the test were destroyed.
func testAccCheckBuildingDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_building" {
				continue
			}
			_, err := c.GetBuildingV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro building %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro building %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// buildingConfigMinimal returns a config with only the required Name attribute set.
func buildingConfigMinimal(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "test" {
			name = %q
		}
	`, name)
}

// buildingConfigAllFields returns a config with every optional field populated.
func buildingConfigAllFields(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "test" {
			name             = %q
			city             = "Minneapolis"
			country          = "USA"
			state_province   = "MN"
			street_address_1 = "100 Washington Ave S"
			street_address_2 = "Suite 1100"
			zip_postal_code  = "55401"
		}
	`, name)
}

// buildingConfigPartialFields keeps city, country, and street_address_1 set while
// dropping state_province, street_address_2, and zip_postal_code. The buildings PUT
// is full-replace, but every address field is Optional+Computed with
// UseStateForUnknown, so dropping a field from config PRESERVES its prior server
// value (omit = preserve) rather than clearing it. Setting a field to "" is what
// clears it (demonstrated inline in TestAccResource_ProBuilding_SplitOwnership).
func buildingConfigPartialFields(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "test" {
			name             = %q
			city             = "Minneapolis"
			country          = "USA"
			street_address_1 = "100 Washington Ave S"
		}
	`, name)
}

// TestAccResource_ProBuilding_Basic walks the Optional+Computed full-replace contract:
// minimal create, populate all optional fields, then drop a subset from config. Because
// every address field is Optional+Computed with UseStateForUnknown on a full-replace
// endpoint, the dropped fields are PRESERVED at their prior server value (omit =
// preserve), not cleared. The framework's implicit post-apply plan check fails any step
// that round-trips dirty.
func TestAccResource_ProBuilding_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	nameMinimal := "tf-acc-pro-building-" + suffix
	nameAll := "tf-acc-pro-building-all-" + suffix
	namePartial := "tf-acc-pro-building-partial-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBuildingDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: buildingConfigMinimal(nameMinimal),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_building.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "name", nameMinimal),
				),
			},
			{
				Config: buildingConfigAllFields(nameAll),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "name", nameAll),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "city", "Minneapolis"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "country", "USA"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "state_province", "MN"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "street_address_1", "100 Washington Ave S"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "street_address_2", "Suite 1100"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "zip_postal_code", "55401"),
				),
			},
			{
				Config: buildingConfigPartialFields(namePartial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "name", namePartial),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "city", "Minneapolis"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "country", "USA"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "street_address_1", "100 Washington Ave S"),
					// Dropped from config but PRESERVED at their prior values (omit =
					// preserve via Optional+Computed + UseStateForUnknown), not cleared.
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "state_province", "MN"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "street_address_2", "Suite 1100"),
					resource.TestCheckResourceAttr("jamfplatform_pro_building.test", "zip_postal_code", "55401"),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_building.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProBuilding_SplitOwnership proves the omit=preserve contract for an
// Optional+Computed address field (`city`) on the full-replace buildings endpoint: when
// `city` is omitted from HCL, an out-of-band edit (simulating the Jamf Pro UI) survives
// an unrelated Terraform change (a name update) rather than being wiped — and an
// explicit "" still clears it. Without Optional+Computed + UseStateForUnknown this
// regresses: the name-change PUT drops `city` and full-replace wipes it to null.
func TestAccResource_ProBuilding_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-building-split-" + suffix
	nameUpdated := "tf-acc-pro-building-split-upd-" + suffix
	const addr = "jamfplatform_pro_building.test"
	const tfCity = "TF City"        // initial TF-declared value
	const uiCity = "UI Edited City" // later set out-of-band (UI)

	var buildingID string

	// setCityOutOfBand simulates a UI edit: GET the building, set city, PUT it back
	// (a full-object write, like the admin console does).
	setCityOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetBuildingV1(ctx, buildingID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		v := uiCity
		got.City = &v
		if _, err := c.UpdateBuildingV1(ctx, buildingID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerCity := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetBuildingV1(context.Background(), buildingID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if helpers.DerefString(got.City) != want {
				return fmt.Errorf("city = %q, want %q", helpers.DerefString(got.City), want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBuildingDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with a TF-declared city, so the next step proves the UI value
				// is preserved AND not reverted to this prior TF-owned value.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_building" "test" {
						name = %q
						city = %q
					}
				`, name, tfCity),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "city", tfCity),
					func(s *terraform.State) error {
						buildingID = s.RootModule().Resources[addr].Primary.ID
						return nil
					},
				),
			},
			{
				// Admin overwrites city in the UI to a DIFFERENT value; config now
				// REMOVES city and changes only the name. The UI value must survive —
				// neither wiped by the full-replace PUT nor reverted to the prior
				// TF-owned value.
				PreConfig: setCityOutOfBand,
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_building" "test" {
						name = %q
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "name", nameUpdated),
					// State adopts the out-of-band value (Computed) and preserves it.
					resource.TestCheckResourceAttr(addr, "city", uiCity),
					checkServerCity(uiCity),
				),
			},
			{
				// Explicit "" clears it (full-replace), proving TF can still take over.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_building" "test" {
						name = %q
						city = ""
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "city", ""),
					checkServerCity(""),
				),
			},
		},
	})
}

func TestAccDataSource_ProBuilding_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-building-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBuildingDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_building" "src" {
						name = %q
						city = "Eau Claire"
					}

					data "jamfplatform_pro_building" "lookup" {
						id = jamfplatform_pro_building.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_building.lookup", "name", "jamfplatform_pro_building.src", "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_building.lookup", "city", "jamfplatform_pro_building.src", "city"),
				),
			},
		},
	})
}

func TestAccDataSource_ProBuildings_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-buildings-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBuildingDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_building" "src" {
						name = %q
					}

					data "jamfplatform_pro_buildings" "lookup" {
						filter = [
							{
								selector = "name"
								argument = jamfplatform_pro_building.src.name
							}
						]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_buildings.lookup", "buildings.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_buildings.lookup", "buildings.0.name", name),
				),
			},
		},
	})
}

// TestAccListResource_ProBuilding_Basic exercises the jamfplatform_pro_building list
// resource via the `terraform query` workflow.
func TestAccListResource_ProBuilding_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-building-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBuildingDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_building" "src" {
						name = %q
						city = "Minneapolis"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_building.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_building" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = [
								{
									selector = "name"
									argument = %q
								}
							]
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_building.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_building.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("city"), KnownValue: knownvalue.StringExact("Minneapolis")},
						},
					),
				},
			},
		},
	})
}
