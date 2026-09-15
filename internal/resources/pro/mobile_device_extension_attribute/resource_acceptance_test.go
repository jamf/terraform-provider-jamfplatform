// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro /v1/mobile-device-extension-attributes
// endpoint. Mobile EAs cannot run scripts; the discriminator covers POPUP and
// DIRECTORY_SERVICE_ATTRIBUTE_MAPPING only.

package mobile_device_extension_attribute_test

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

const mdeaResource = "jamfplatform_pro_mobile_device_extension_attribute.test"

func testAccCheckMDEADestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_mobile_device_extension_attribute" {
				continue
			}
			_, err := c.GetMobileDeviceExtensionAttributeV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking mobile EA %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("mobile EA %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func mdeaText(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
			name              = %q
			description       = "probe"
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = "GENERAL"
		}
	`, name)
}

func mdeaPopup(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
			name               = %q
			data_type          = "STRING"
			input_type         = "POPUP"
			inventory_display  = "EXTENSION_ATTRIBUTES"
			popup_menu_choices = ["One", "Two"]
		}
	`, name)
}

// Mobile device EAs support DIRECTORY_SERVICE_ATTRIBUTE_MAPPING, but the tenant
// precondition ("LDAP configured") requires inventory collection to pull
// user/location from the directory service — and unlike computers, mobile devices
// expose no equivalent inventory-collection settings resource (the setting is not
// surfaced in the API/UI), so we cannot reliably stand up that precondition. DSAM
// is therefore not exercised here; the computer EA test covers that input type.
func TestAccResource_ProMobileDeviceExtensionAttribute_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mdea-" + suffix
	renamed := "tf-acc-pro-mdea-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMDEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mdeaText(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mdeaResource, "id"),
					resource.TestCheckResourceAttr(mdeaResource, "input_type", "TEXT"),
					resource.TestCheckNoResourceAttr(mdeaResource, "popup_menu_choices.0"),
				),
			},
			{
				Config: mdeaPopup(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mdeaResource, "name", renamed),
					resource.TestCheckResourceAttr(mdeaResource, "input_type", "POPUP"),
					resource.TestCheckResourceAttr(mdeaResource, "popup_menu_choices.#", "2"),
				),
			},
			{
				ResourceName:            mdeaResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// mdeaSplitOwn renders a TEXT EA that OMITS description, varying inventory_display.
func mdeaSplitOwn(name, inventoryDisplay string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
			name              = %q
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = %q
		}
	`, name, inventoryDisplay)
}

// mdeaPopupNoChoices renders a POPUP EA that OMITS popup_menu_choices.
func mdeaPopupNoChoices(name, inventoryDisplay string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
			name              = %q
			data_type         = "STRING"
			input_type        = "POPUP"
			inventory_display = %q
		}
	`, name, inventoryDisplay)
}

// TestAccResource_ProMobileDeviceExtensionAttribute_SplitOwnership proves the
// omit=preserve contract for the Optional+Computed `description` on this
// full-replace endpoint: with description omitted, an out-of-band edit survives an
// unrelated change (inventory_display) and an explicit "" clears it.
func TestAccResource_ProMobileDeviceExtensionAttribute_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdea-split-" + suffix
	const uiDesc = "UI edited description"

	var eaID string

	setDescriptionOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetMobileDeviceExtensionAttributeV1(ctx, eaID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		v := uiDesc
		got.Description = &v
		if _, err := c.UpdateMobileDeviceExtensionAttributeV1(ctx, eaID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerDescription := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetMobileDeviceExtensionAttributeV1(context.Background(), eaID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if helpers.DerefString(got.Description) != want {
				return fmt.Errorf("description = %q, want %q", helpers.DerefString(got.Description), want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMDEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mdeaSplitOwn(name, "GENERAL"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mdeaResource, "id"),
					func(s *terraform.State) error {
						eaID = s.RootModule().Resources[mdeaResource].Primary.ID
						return nil
					},
				),
			},
			{
				PreConfig: setDescriptionOutOfBand,
				Config:    mdeaSplitOwn(name, "HARDWARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mdeaResource, "inventory_display", "HARDWARE"),
					resource.TestCheckResourceAttr(mdeaResource, "description", uiDesc),
					checkServerDescription(uiDesc),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
						name              = %q
						description       = ""
						data_type         = "STRING"
						input_type        = "TEXT"
						inventory_display = "HARDWARE"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mdeaResource, "description", ""),
					checkServerDescription(""),
				),
			},
		},
	})
}

// TestAccResource_ProMobileDeviceExtensionAttribute_PopupSplitOwnership proves the
// omit=preserve contract for popup_menu_choices (Optional+Computed Set, gated by
// input_type = POPUP): out-of-band choices survive an unrelated change, an explicit
// [] clears them and round-trips, and a POPUP→TEXT transition clears cleanly (the
// input_type-aware plan modifier predicts the cleared result, no consistency error).
func TestAccResource_ProMobileDeviceExtensionAttribute_PopupSplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdea-popup-split-" + suffix
	uiChoices := []string{"One", "Two"}

	var eaID string

	setChoicesOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetMobileDeviceExtensionAttributeV1(ctx, eaID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		cs := append([]string(nil), uiChoices...)
		got.PopupMenuChoices = &cs
		if _, err := c.UpdateMobileDeviceExtensionAttributeV1(ctx, eaID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerChoices := func(wantLen int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetMobileDeviceExtensionAttributeV1(context.Background(), eaID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			n := 0
			if got.PopupMenuChoices != nil {
				n = len(*got.PopupMenuChoices)
			}
			if n != wantLen {
				return fmt.Errorf("server popup_menu_choices len = %d, want %d", n, wantLen)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMDEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mdeaPopupNoChoices(name, "GENERAL"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mdeaResource, "id"),
					func(s *terraform.State) error {
						eaID = s.RootModule().Resources[mdeaResource].Primary.ID
						return nil
					},
				),
			},
			{
				PreConfig: setChoicesOutOfBand,
				Config:    mdeaPopupNoChoices(name, "EXTENSION_ATTRIBUTES"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mdeaResource, "inventory_display", "EXTENSION_ATTRIBUTES"),
					resource.TestCheckResourceAttr(mdeaResource, "popup_menu_choices.#", "2"),
					resource.TestCheckTypeSetElemAttr(mdeaResource, "popup_menu_choices.*", "One"),
					checkServerChoices(2),
				),
			},
			{
				// Explicit [] while staying POPUP clears and round-trips as an empty set.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
						name               = %q
						data_type          = "STRING"
						input_type         = "POPUP"
						inventory_display  = "EXTENSION_ATTRIBUTES"
						popup_menu_choices = []
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mdeaResource, "popup_menu_choices.#", "0"),
					checkServerChoices(0),
				),
			},
			{
				// POPUP→TEXT clears cleanly (no "inconsistent result after apply").
				Config: mdeaSplitOwn(name, "EXTENSION_ATTRIBUTES"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mdeaResource, "input_type", "TEXT"),
					resource.TestCheckNoResourceAttr(mdeaResource, "popup_menu_choices.0"),
					checkServerChoices(0),
				),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceExtensionAttribute_ValidatorErrors(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// popup_menu_choices forbidden on TEXT.
				Config: `
					resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
						name               = "tf-acc-mdea-bad"
						data_type          = "STRING"
						input_type         = "TEXT"
						inventory_display  = "GENERAL"
						popup_menu_choices = ["a"]
					}
				`,
				ExpectError: regexp.MustCompile(`POPUP`),
			},
		},
	})
}

func TestAccDataSource_ProMobileDeviceExtensionAttribute_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mdea-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMDEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mdeaText(name) + `
					data "jamfplatform_pro_mobile_device_extension_attribute" "by_name" {
						name = jamfplatform_pro_mobile_device_extension_attribute.test.name
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mobile_device_extension_attribute.by_name", "id", mdeaResource, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_extension_attribute.by_name", "input_type", "TEXT"),
				),
			},
		},
	})
}

func TestAccListResource_ProMobileDeviceExtensionAttribute_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mdea-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMDEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mdeaText(name),
				Check:  resource.TestCheckResourceAttrSet(mdeaResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_mobile_device_extension_attribute" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_mobile_device_extension_attribute.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_mobile_device_extension_attribute.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("input_type"), KnownValue: knownvalue.StringExact("TEXT")},
						},
					),
				},
			},
		},
	})
}
