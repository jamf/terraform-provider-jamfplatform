// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package smart_mobile_device_group_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_smart_mobile_device_group" "test" {
  name = %q
  criteria = [
    {
      name        = "Model"
      search_type = "like"
      value       = "iPad"
    },
  ]
}
`, name)
}

func TestAccResource_ProSmartMobileDeviceGroup_NameCollisionWithPlatformGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-smart-mdg-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandPlatformDeviceGroup(t, name, devicegroups.DeviceTypeV1Mobile) },
				Config:      collisionConfig(strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*static\s+mobile\s+device\s+group.*already\s+exists`),
			},
		},
	})
}

func twoGroupsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_smart_mobile_device_group" "one" {
  name = %[1]q
  criteria = [
    {
      name        = "Model"
      search_type = "like"
      value       = "iPad"
    },
  ]
}

resource "jamfplatform_pro_smart_mobile_device_group" "two" {
  depends_on = [jamfplatform_pro_smart_mobile_device_group.one]

  name = %[2]q
  criteria = [
    {
      name        = "Model"
      search_type = "like"
      value       = "iPad"
    },
  ]
}
`, nameOne, nameTwo)
}

func TestAccResource_ProSmartMobileDeviceGroup_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-smart-mdg-one-" + suffix
	two := "tf-acc-collide-smart-mdg-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoGroupsConfig(one, two)},
			{
				Config:      twoGroupsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProSmartMobileDeviceGroup_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-smart-mdg-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoGroupsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)already\s+exists.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
