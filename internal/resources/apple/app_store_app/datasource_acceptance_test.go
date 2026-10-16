// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// These tests read the public App Store, so their lookup subjects are Apple-owned apps whose ids
// and bundle identifiers have been stable for years: Xcode (Mac only, 497799835) and Pages
// (universal, 361309726). Values that move with every release (version, description, icon) are
// asserted by shape only.
//
// The data source makes no Jamf call, but provider Configure validates the Jamf credentials
// before any construct runs, so every test still gates on testhelpers.AccPreCheck. The package is
// unclaimed in .github/acceptance-lanes.json and therefore runs in the default pro lane, which the
// two chain tests need anyway.

package app_store_app_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const dataSourceAddr = "data.jamfplatform_apple_app_store_app.test"

var (
	iconURLPattern = regexp.MustCompile(`^https://[a-z0-9-]+\.mzstatic\.com/.+/512x512bb\.png$`)
	versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
	nonEmpty       = regexp.MustCompile(`\S`)
)

func TestAccDataSource_AppleAppStoreApp_MacByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
				data "jamfplatform_apple_app_store_app" "test" {
				  id     = "497799835"
				  target = "mac"
				}
			`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("bundle_id"), knownvalue.StringExact("com.apple.dt.Xcode")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("name"), knownvalue.StringExact("Xcode")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("country_region"), knownvalue.StringExact("US")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("is_free"), knownvalue.Bool(true)),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("vpp_device_based_licensing"), knownvalue.Bool(true)),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("version"), knownvalue.StringRegexp(versionPattern)),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("url"), knownvalue.StringRegexp(regexp.MustCompile(`^https://apps\.apple\.com/us/app/xcode/id497799835`))),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("icon_url"), knownvalue.StringRegexp(iconURLPattern)),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("description"), knownvalue.StringRegexp(nonEmpty)),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("genres"), knownvalue.ListPartial(map[int]knownvalue.Check{0: knownvalue.StringExact("Developer Tools")})),
			},
		}},
	})
}

func TestAccDataSource_AppleAppStoreApp_MobileByBundleID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
				data "jamfplatform_apple_app_store_app" "test" {
				  bundle_id = "com.apple.Pages"
				  target    = "mobile_device"
				}
			`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("id"), knownvalue.StringExact("361309726")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("bundle_id"), knownvalue.StringExact("com.apple.Pages")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("icon_url"), knownvalue.StringRegexp(iconURLPattern)),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("is_free"), knownvalue.Bool(true)),
			},
		}},
	})
}

// TestAccDataSource_AppleAppStoreApp_StoreURLStorefront pins that a store link's country is used
// when country_region is unset, and that the URL output follows the storefront.
func TestAccDataSource_AppleAppStoreApp_StoreURLStorefront(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
				data "jamfplatform_apple_app_store_app" "test" {
				  store_url = "https://apps.apple.com/gb/app/pages-create-documents/id361309726"
				  target    = "mac"
				}
			`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("id"), knownvalue.StringExact("361309726")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("country_region"), knownvalue.StringExact("GB")),
				statecheck.ExpectKnownValue(dataSourceAddr, tfjsonpath.New("url"), knownvalue.StringRegexp(regexp.MustCompile(`^https://apps\.apple\.com/gb/`))),
			},
		}},
	})
}

// TestAccDataSource_AppleAppStoreApp_Errors covers every refusal the data source raises itself.
// The retired Mac Pages id is the documented merge case: it answers empty on every storefront.
func TestAccDataSource_AppleAppStoreApp_Errors(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `id = "409201541"` + "\n" + `target = "mac"` + "\n}",
				ExpectError: regexp.MustCompile(`App not found on the App Store`),
			},
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `id = "497799835"` + "\n" + `target = "mac"` + "\n" + `country_region = "XX"` + "\n}",
				ExpectError: regexp.MustCompile(`Unrecognised App Store country or region`),
			},
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `bundle_id = "COM.APPLE.PAGES"` + "\n" + `target = "mobile_device"` + "\n}",
				ExpectError: regexp.MustCompile(`Bundle identifier spelling differs`),
			},
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `store_url = "https://apps.apple.com/gb/app/x/id361309726"` + "\n" + `country_region = "US"` + "\n" + `target = "mac"` + "\n}",
				ExpectError: regexp.MustCompile(`Country or region conflicts`),
			},
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `store_url = "https://example.com/app/id1"` + "\n" + `target = "mac"` + "\n}",
				ExpectError: regexp.MustCompile(`Invalid App Store link`),
			},
		},
	})
}

// TestAccDataSource_AppleAppStoreApp_ExactlyOneSelector matches the shared detail line, because
// ExactlyOneOf emits a different summary for "both" and for "neither".
func TestAccDataSource_AppleAppStoreApp_ExactlyOneSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)
	shared := regexp.MustCompile(`Exactly one of these attributes must be configured`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `id = "497799835"` + "\n" + `bundle_id = "com.apple.dt.Xcode"` + "\n" + `target = "mac"` + "\n}",
				ExpectError: shared,
			},
			{
				Config:      `data "jamfplatform_apple_app_store_app" "test" {` + "\n" + `target = "mac"` + "\n}",
				ExpectError: shared,
			},
		},
	})
}

// chainConfig wires a listing into jamfplatform_pro_icon and into the app record for target. The
// app name is prefixed so the record is recognisable and cannot collide with a real one.
func chainConfig(target, appBlock string) string {
	return fmt.Sprintf(`
		data "jamfplatform_apple_app_store_app" "test" {
		  id     = "361309726"
		  target = %q
		}

		resource "jamfplatform_pro_icon" "test" {
		  icon_file_source = data.jamfplatform_apple_app_store_app.test.icon_url
		}

		%s
	`, target, appBlock)
}

const macChainApp = `
		resource "jamfplatform_pro_mac_app_store_app" "test" {
		  general = {
		    name            = "tf-acc-${data.jamfplatform_apple_app_store_app.test.name}"
		    version         = data.jamfplatform_apple_app_store_app.test.version
		    bundle_id       = data.jamfplatform_apple_app_store_app.test.bundle_id
		    url             = data.jamfplatform_apple_app_store_app.test.url
		    is_free         = data.jamfplatform_apple_app_store_app.test.is_free
		    deployment_type = "Make Available in Self Service"
		  }
		  self_service = {
		    self_service_description = data.jamfplatform_apple_app_store_app.test.description
		    self_service_icon = {
		      id = jamfplatform_pro_icon.test.id
		    }
		  }
		}
`

const mobileChainApp = `
		resource "jamfplatform_pro_mobile_device_app" "test" {
		  general = {
		    name                  = "tf-acc-${data.jamfplatform_apple_app_store_app.test.name}"
		    version               = data.jamfplatform_apple_app_store_app.test.version
		    bundle_id             = data.jamfplatform_apple_app_store_app.test.bundle_id
		    itunes_store_url      = data.jamfplatform_apple_app_store_app.test.url
		    itunes_country_region = data.jamfplatform_apple_app_store_app.test.country_region
		    is_free               = data.jamfplatform_apple_app_store_app.test.is_free
		  }
		  self_service = {
		    self_service_description = data.jamfplatform_apple_app_store_app.test.description
		    self_service_icon = {
		      id = jamfplatform_pro_icon.test.id
		    }
		  }
		}
`

// TestAccDataSource_AppleAppStoreApp_ChainMac builds the Mac flavour of Pages into a Mac App Store
// app with its icon. The second step re-plans the same configuration and must be empty: Apple's
// description (which carries U+2028 and bullets) has to round-trip through Jamf Pro's Self Service
// description, which no unit test can show.
func TestAccDataSource_AppleAppStoreApp_ChainMac(t *testing.T) {
	testhelpers.AccPreCheck(t)
	cfg := chainConfig("mac", macChainApp)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkAppsDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("jamfplatform_pro_mac_app_store_app.test", tfjsonpath.New("general").AtMapKey("bundle_id"), knownvalue.StringExact("com.apple.Pages")),
					statecheck.CompareValuePairs("jamfplatform_pro_mac_app_store_app.test", tfjsonpath.New("self_service").AtMapKey("self_service_icon").AtMapKey("id"),
						"jamfplatform_pro_icon.test", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("jamfplatform_pro_mac_app_store_app.test", tfjsonpath.New("general").AtMapKey("version"),
						dataSourceAddr, tfjsonpath.New("version"), compare.ValuesSame()),
				},
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}

// TestAccDataSource_AppleAppStoreApp_ChainMobile is the mobile device counterpart, and also wires
// country_region into itunes_country_region.
func TestAccDataSource_AppleAppStoreApp_ChainMobile(t *testing.T) {
	testhelpers.AccPreCheck(t)
	cfg := chainConfig("mobile_device", mobileChainApp)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkAppsDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("jamfplatform_pro_mobile_device_app.test", tfjsonpath.New("general").AtMapKey("bundle_id"), knownvalue.StringExact("com.apple.Pages")),
					statecheck.ExpectKnownValue("jamfplatform_pro_mobile_device_app.test", tfjsonpath.New("general").AtMapKey("itunes_country_region"), knownvalue.StringExact("US")),
					statecheck.CompareValuePairs("jamfplatform_pro_mobile_device_app.test", tfjsonpath.New("self_service").AtMapKey("self_service_icon").AtMapKey("id"),
						"jamfplatform_pro_icon.test", tfjsonpath.New("id"), compare.ValuesSame()),
				},
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}

// checkAppsDestroyed verifies the app records a chain test created are gone. Icons are not
// checked: Jamf Pro offers no icon delete, so jamfplatform_pro_icon's destroy is state-only.
func checkAppsDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			var err error
			switch rs.Type {
			case "jamfplatform_pro_mac_app_store_app":
				_, err = c.GetMacApplicationByID(ctx, rs.Primary.ID)
			case "jamfplatform_pro_mobile_device_app":
				_, err = c.GetMobileDeviceApplicationByID(ctx, rs.Primary.ID)
			default:
				continue
			}
			if err == nil {
				return fmt.Errorf("%s %s still exists", rs.Type, rs.Primary.ID)
			}
			if !helpers.IsNotFoundError(err) {
				return fmt.Errorf("checking %s %s: %w", rs.Type, rs.Primary.ID, err)
			}
		}
		return nil
	}
}
