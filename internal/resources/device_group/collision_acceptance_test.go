// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package device_group_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionConfig(name, deviceType string) string {
	return fmt.Sprintf(`
resource "jamfplatform_device_group" "test" {
  name        = %q
  group_type  = "static"
  device_type = %q
}
`, name, deviceType)
}

func TestAccResource_DeviceGroup_ComputerNameCollisionWithJamfProGroupImportsByPlatformID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-dg-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandProStaticComputerGroup(t, name) },
				Config:      collisionConfig(strings.ToUpper(name), "computer"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*static\s+computer\s+group.*id\s+=\s+"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"`),
			},
		},
	})
}

func TestAccResource_DeviceGroup_MobileNameCollisionWithPlatformGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-dg-mob-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandPlatformDeviceGroup(t, name, devicegroups.DeviceTypeV1Mobile) },
				Config:      collisionConfig(strings.ToUpper(name), "mobile"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*mobile\s+device\s+group.*already\s+exists`),
			},
		},
	})
}

func TestAccResource_DeviceGroup_NameDoesNotCollideAcrossDeviceTypes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-dg-sep-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { testhelpers.CreateOutOfBandPlatformDeviceGroup(t, name, devicegroups.DeviceTypeV1Computer) },
				Config:             collisionConfig(name, "mobile"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
