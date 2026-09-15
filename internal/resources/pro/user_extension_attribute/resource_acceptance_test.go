// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro Classic /userextensionattributes
// endpoint. User EAs support Text Field and Pop-up Menu only.

package user_extension_attribute_test

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

const ueaResource = "jamfplatform_pro_user_extension_attribute.test"

func testAccCheckUEADestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_user_extension_attribute" {
				continue
			}
			_, err := c.GetUserExtensionAttributeByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking user EA %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("user EA %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func ueaText(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_extension_attribute" "test" {
			name        = %q
			description = "probe"
			data_type   = "String"
			input_type  = "Text Field"
		}
	`, name)
}

func ueaPopup(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_extension_attribute" "test" {
			name               = %q
			data_type          = "String"
			input_type         = "Pop-up Menu"
			popup_menu_choices = ["Red", "Green", "Blue"]
		}
	`, name)
}

func TestAccResource_ProUserExtensionAttribute_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-uea-" + suffix
	renamed := "tf-acc-pro-uea-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ueaText(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ueaResource, "id"),
					resource.TestCheckResourceAttr(ueaResource, "input_type", "Text Field"),
					resource.TestCheckResourceAttr(ueaResource, "data_type", "String"),
					resource.TestCheckNoResourceAttr(ueaResource, "popup_menu_choices.0"),
				),
			},
			{
				// Text Field → Pop-up Menu: server auto-clears nothing extra; the
				// flatten round-trips the ordered choices.
				Config: ueaPopup(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ueaResource, "name", renamed),
					resource.TestCheckResourceAttr(ueaResource, "input_type", "Pop-up Menu"),
					resource.TestCheckResourceAttr(ueaResource, "popup_menu_choices.#", "3"),
					resource.TestCheckResourceAttr(ueaResource, "popup_menu_choices.0", "Red"),
				),
			},
			{
				ResourceName:            ueaResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccResource_ProUserExtensionAttribute_ValidatorErrors(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// popup_menu_choices forbidden on Text Field.
				Config: `
					resource "jamfplatform_pro_user_extension_attribute" "test" {
						name               = "tf-acc-uea-bad"
						data_type          = "String"
						input_type         = "Text Field"
						popup_menu_choices = ["a"]
					}
				`,
				ExpectError: regexp.MustCompile(`Pop-up`),
			},
		},
	})
}

func TestAccDataSource_ProUserExtensionAttribute_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-uea-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ueaText(name) + `
					data "jamfplatform_pro_user_extension_attribute" "by_name" {
						name = jamfplatform_pro_user_extension_attribute.test.name
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_user_extension_attribute.by_name", "id", ueaResource, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_user_extension_attribute.by_name", "data_type", "String"),
				),
			},
		},
	})
}

// The Classic list endpoint returns id + name only, so include_resource hydrates
// just those; data_type/input_type are null on list results by design.
func TestAccListResource_ProUserExtensionAttribute_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-uea-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ueaText(name),
				Check:  resource.TestCheckResourceAttrSet(ueaResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_user_extension_attribute" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_user_extension_attribute.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_user_extension_attribute.test",
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
