// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /ibeacons endpoint. Classic
// has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any other classic acceptance work in
// this package.

package ibeacon_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const acceptanceTestUUID = "759b0599-64e0-416a-8d31-d8e93482a4d7"

// testAccCheckIbeaconDestroy verifies iBeacons created during the test were
// destroyed.
func testAccCheckIbeaconDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_ibeacon" {
				continue
			}
			_, err := c.GetIBeaconByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro iBeacon %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro iBeacon %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func ibeaconConfigConcrete(name string, major, minor int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ibeacon" "test" {
			name  = %q
			uuid  = %q
			major = %d
			minor = %d
		}
	`, name, acceptanceTestUUID, major, minor)
}

func ibeaconConfigAnyMajorAndMinor(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ibeacon" "test" {
			name                    = %q
			uuid                    = %q
			include_any_major_value = true
			include_any_minor_value = true
		}
	`, name, acceptanceTestUUID)
}

func ibeaconConfigAnyMajorConcreteMinor(name string, minor int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ibeacon" "test" {
			name                    = %q
			uuid                    = %q
			include_any_major_value = true
			minor                   = %d
		}
	`, name, acceptanceTestUUID, minor)
}

// TestAccResource_ProIbeacon_Basic exercises create with concrete major/minor,
// in-place rename + major/minor change, and import for the classic /ibeacons
// endpoint. The rename step implicitly verifies the GET-after-Update path
// (classic Update returns 201 + empty body) and the major/minor → state
// derivation in state_builders.go.
func TestAccResource_ProIbeacon_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-ibeacon-" + suffix
	renamed := "tf-acc-ibeacon-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckIbeaconDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ibeaconConfigConcrete(original, 1, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_ibeacon.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "name", original),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "uuid", acceptanceTestUUID),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "major", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "minor", "2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_major_value", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_minor_value", "false"),
				),
			},
			{
				Config: ibeaconConfigConcrete(renamed, 100, 200),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "name", renamed),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "major", "100"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "minor", "200"),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_ibeacon.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProIbeacon_AnyMajorAndMinor exercises the "match any value"
// path on both axes. Create with both include_any toggles true; verify the
// -1/-1 sentinel round-trips back to IncludeAny*=true + null major/minor.
func TestAccResource_ProIbeacon_AnyMajorAndMinor(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ibeacon-any-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckIbeaconDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ibeaconConfigAnyMajorAndMinor(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_major_value", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_minor_value", "true"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_ibeacon.test", "major"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_ibeacon.test", "minor"),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_ibeacon.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProIbeacon_MixedAxes exercises the mixed shape: any-major
// + concrete-minor, then toggle to concrete-major + any-minor. Confirms the
// two axes are independent both at the schema-validation layer and through
// the GET-after-Update refresh path.
func TestAccResource_ProIbeacon_MixedAxes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ibeacon-mixed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckIbeaconDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ibeaconConfigAnyMajorConcreteMinor(name, 7),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_major_value", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_minor_value", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "minor", "7"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_ibeacon.test", "major"),
				),
			},
			{
				// Flip the axes: concrete major, any minor.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ibeacon" "test" {
						name                    = %q
						uuid                    = %q
						major                   = 42
						include_any_minor_value = true
					}
				`, name, acceptanceTestUUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_major_value", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "include_any_minor_value", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_ibeacon.test", "major", "42"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_ibeacon.test", "minor"),
				),
			},
		},
	})
}
