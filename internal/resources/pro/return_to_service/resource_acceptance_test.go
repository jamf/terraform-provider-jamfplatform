// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro /v1/return-to-service endpoint.
//
// Design notes the acc run verifies (each is load-bearing — see the build spike):
//   - both display_name and wifi_profile_id are server-required on every write, so
//     the schema marks both Required (no Optional+Computed); update sends both.
//   - wifi_profile_id must reference a MOBILE-DEVICE configuration profile that
//     carries a Wi-Fi payload — wire-proven: a macOS profile and a non-Wi-Fi
//     mobile profile are both rejected. The fixture therefore creates a
//     jamfplatform_pro_mobile_device_configuration_profile with a com.apple.wifi.managed
//     payload and feeds its id (the classic profile id this endpoint resolves
//     against) into wifi_profile_id.
//   - GET-after-write: Create reads back via GET (POST returns only id + href);
//     Update reads back via GET for the canonical representation.
//   - import round-trips id + display_name + wifi_profile_id (timeouts ignored).

package return_to_service_test

import (
	"context"
	"crypto/rand"
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

const rtsResource = "jamfplatform_pro_return_to_service.test"

func testAccCheckRTSDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_return_to_service" {
				continue
			}
			_, err := c.GetReturnToServiceConfigurationV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Return to Service configuration %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Return to Service configuration %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// newUUID generates a random v4 UUID string for fresh payload identifiers.
func newUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating UUID: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// wifiPayload returns a minimal mobile-device Wi-Fi .mobileconfig with fresh
// payload identifiers so repeat runs don't collide.
func wifiPayload(t *testing.T, ssid string) string {
	t.Helper()
	inner, outer := newUUID(t), newUUID(t)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>PayloadContent</key>
  <array><dict>
    <key>PayloadType</key><string>com.apple.wifi.managed</string>
    <key>PayloadVersion</key><integer>1</integer>
    <key>PayloadIdentifier</key><string>com.jamf.tf.acc.wifi.%[1]s</string>
    <key>PayloadUUID</key><string>%[1]s</string>
    <key>PayloadDisplayName</key><string>Wi-Fi</string>
    <key>SSID_STR</key><string>%[3]s</string>
    <key>EncryptionType</key><string>None</string>
    <key>AutoJoin</key><true/>
  </dict></array>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadVersion</key><integer>1</integer>
  <key>PayloadIdentifier</key><string>com.jamf.tf.acc.rts.%[2]s</string>
  <key>PayloadUUID</key><string>%[2]s</string>
  <key>PayloadDisplayName</key><string>tf-acc-rts-wifi</string>
</dict></plist>
`, inner, outer, ssid)
}

// rtsConfigSingle creates one Wi-Fi mobile profile and an RTS config referencing it.
func rtsConfigSingle(displayName, profileName, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "wifi_a" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
}

resource "jamfplatform_pro_return_to_service" "test" {
  display_name    = %q
  wifi_profile_id = jamfplatform_pro_mobile_device_configuration_profile.wifi_a.id
}
`, profileName, payload, displayName)
}

// rtsConfigSwap adds a second Wi-Fi profile and points the RTS config at it,
// exercising both a display_name rename and a wifi_profile_id change.
func rtsConfigSwap(displayName, profileNameA, payloadA, profileNameB, payloadB string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "wifi_a" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
}

resource "jamfplatform_pro_mobile_device_configuration_profile" "wifi_b" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
}

resource "jamfplatform_pro_return_to_service" "test" {
  display_name    = %q
  wifi_profile_id = jamfplatform_pro_mobile_device_configuration_profile.wifi_b.id
}
`, profileNameA, payloadA, profileNameB, payloadB, displayName)
}

func TestAccResource_ProReturnToService_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-rts-" + suffix
	renamed := "tf-acc-pro-rts-renamed-" + suffix
	profileA := "tf-acc-rts-wifi-a-" + suffix
	profileB := "tf-acc-rts-wifi-b-" + suffix
	payloadA := wifiPayload(t, "tf-acc-rts-a")
	payloadB := wifiPayload(t, "tf-acc-rts-b")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRTSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: rtsConfigSingle(name, profileA, payloadA),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(rtsResource, "id"),
					resource.TestCheckResourceAttr(rtsResource, "display_name", name),
					resource.TestCheckResourceAttrPair(rtsResource, "wifi_profile_id", "jamfplatform_pro_mobile_device_configuration_profile.wifi_a", "id"),
				),
			},
			{
				// Rename display_name and swap to a different Wi-Fi profile.
				Config: rtsConfigSwap(renamed, profileA, payloadA, profileB, payloadB),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rtsResource, "display_name", renamed),
					resource.TestCheckResourceAttrPair(rtsResource, "wifi_profile_id", "jamfplatform_pro_mobile_device_configuration_profile.wifi_b", "id"),
				),
			},
			{
				ResourceName:            rtsResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProReturnToService_EmptyDisplayNameRejected exercises the
// display_name LengthAtLeast(1) validator.
func TestAccResource_ProReturnToService_EmptyDisplayNameRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_return_to_service" "test" {
						display_name    = ""
						wifi_profile_id = "1"
					}
				`,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

// TestAccResource_ProReturnToService_NonNumericWifiProfileIDRejected exercises
// the wifi_profile_id whole-number validator at plan time.
func TestAccResource_ProReturnToService_NonNumericWifiProfileIDRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_return_to_service" "test" {
						display_name    = "tf-acc-bad-wifi"
						wifi_profile_id = "not-a-number"
					}
				`,
				ExpectError: regexp.MustCompile(`whole number greater than 0`),
			},
		},
	})
}

func TestAccDataSource_ProReturnToService_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-rts-ds-" + suffix
	profileA := "tf-acc-rts-ds-wifi-" + suffix
	payloadA := wifiPayload(t, "tf-acc-rts-ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRTSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: rtsConfigSingle(name, profileA, payloadA) + `
					data "jamfplatform_pro_return_to_service" "by_id" {
						id = jamfplatform_pro_return_to_service.test.id
					}

					data "jamfplatform_pro_return_to_service" "by_name" {
						display_name = jamfplatform_pro_return_to_service.test.display_name
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_return_to_service.by_id", "display_name", rtsResource, "display_name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_return_to_service.by_id", "wifi_profile_id", rtsResource, "wifi_profile_id"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_return_to_service.by_name", "id", rtsResource, "id"),
				),
			},
		},
	})
}

func TestAccListResource_ProReturnToService_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-rts-list-" + suffix
	profileA := "tf-acc-rts-list-wifi-" + suffix
	payloadA := wifiPayload(t, "tf-acc-rts-list")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRTSDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: rtsConfigSingle(name, profileA, payloadA),
				Check:  resource.TestCheckResourceAttrSet(rtsResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_return_to_service" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_return_to_service.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_return_to_service.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("display_name"), KnownValue: knownvalue.StringExact(name)},
						},
					),
				},
			},
		},
	})
}
