// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /removablemacaddresses endpoint.
// Classic has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any future classic acceptance work in this
// package.

package removable_mac_address_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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

// macFor builds a MAC-shaped, run-unique value. The server stores the value verbatim
// (no canonicalisation) and does not validate the format, but a real MAC keeps the
// fixtures realistic. The variant byte distinguishes records used by different tests
// (and steps) within the same run so they never collide. The 02: prefix is the
// locally-administered range, well clear of any real tenant hardware.
func macFor(suffix string, variant byte) string {
	n, _ := strconv.ParseInt(suffix, 10, 64)
	return fmt.Sprintf("02:00:%02X:%02X:%02X:%02X", byte(n>>16), byte(n>>8), byte(n), variant)
}

// testAccCheckRemovableMacAddressDestroy verifies records created during the test were destroyed.
func testAccCheckRemovableMacAddressDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_removable_mac_address" {
				continue
			}
			_, err := c.GetRemovableMacAddressByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro removable MAC address %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro removable MAC address %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// regexpDuplicate matches the server's 409 "Duplicate name" rejection. A single,
// whitespace-free token avoids the line-wrap brittleness Terraform introduces when it
// renders long error details (the §ExpectError regex line-wrap lesson).
func regexpDuplicate() *regexp.Regexp { return regexp.MustCompile("Duplicate") }

func removableMacAddressConfig(mac string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_removable_mac_address" "test" {
			mac_address = %q
		}
	`, mac)
}

// TestAccResource_ProRemovableMacAddress_Basic exercises create, in-place rename, and
// import for the classic /removablemacaddresses endpoint. The rename step also
// implicitly verifies the GET-after-Update path (the classic update returns 201 with
// an ID-only body) — the server renames the MAC in place rather than replacing it.
func TestAccResource_ProRemovableMacAddress_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := macFor(suffix, 0x01)
	renamed := macFor(suffix, 0x02)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRemovableMacAddressDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: removableMacAddressConfig(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_removable_mac_address.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_removable_mac_address.test", "mac_address", original),
				),
			},
			{
				Config: removableMacAddressConfig(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_removable_mac_address.test", "mac_address", renamed),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_removable_mac_address.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProRemovableMacAddress_DuplicateRejected verifies the server-enforced
// uniqueness constraint: creating a second record with an identical MAC fails with a
// 409 "Duplicate name". Uniqueness is exact-string (the server does not canonicalise).
//
// The duplicate check is read-then-write on the classic backend and is NOT race-safe:
// two records created in parallel (Terraform's default) can both slip through before
// either commits, yielding two rows and no error. The explicit depends_on serialises
// creation so "b" POSTs against an already-committed "a" — the sequential path that
// actually triggers the 409 (confirmed by wire-probe).
func TestAccResource_ProRemovableMacAddress_DuplicateRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mac := macFor(suffix, 0x06)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRemovableMacAddressDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_removable_mac_address" "a" {
						mac_address = %q
					}

					resource "jamfplatform_pro_removable_mac_address" "b" {
						mac_address = %q
						depends_on  = [jamfplatform_pro_removable_mac_address.a]
					}
				`, mac, mac),
				ExpectError: regexpDuplicate(),
			},
		},
	})
}

func TestAccDataSource_ProRemovableMacAddress_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mac := macFor(suffix, 0x03)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRemovableMacAddressDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_removable_mac_address" "src" {
						mac_address = %q
					}

					data "jamfplatform_pro_removable_mac_address" "lookup" {
						id = jamfplatform_pro_removable_mac_address.src.id
					}
				`, mac),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_removable_mac_address.lookup", "mac_address", "jamfplatform_pro_removable_mac_address.src", "mac_address"),
				),
			},
		},
	})
}

func TestAccDataSource_ProRemovableMacAddress_ByMacAddress(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mac := macFor(suffix, 0x04)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRemovableMacAddressDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_removable_mac_address" "src" {
						mac_address = %q
					}

					data "jamfplatform_pro_removable_mac_address" "lookup" {
						mac_address = jamfplatform_pro_removable_mac_address.src.mac_address
					}
				`, mac),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_removable_mac_address.lookup", "id", "jamfplatform_pro_removable_mac_address.src", "id"),
				),
			},
		},
	})
}

// TestAccListResource_ProRemovableMacAddress_Basic exercises the
// jamfplatform_pro_removable_mac_address list resource via the `terraform query` workflow.
func TestAccListResource_ProRemovableMacAddress_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mac := macFor(suffix, 0x05)

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRemovableMacAddressDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_removable_mac_address" "src" {
						mac_address = %q
					}
				`, mac),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_removable_mac_address.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_removable_mac_address" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, mac),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_removable_mac_address.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_removable_mac_address.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(mac)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("mac_address"), KnownValue: knownvalue.StringExact(mac)},
						},
					),
				},
			},
		},
	})
}
