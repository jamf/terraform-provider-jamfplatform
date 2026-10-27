// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package static_computer_group_test

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
resource "jamfplatform_pro_static_computer_group" "test" {
  name = %q
}
`, name)
}

func TestAccResource_ProStaticComputerGroup_NameCollisionWithPlatformGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-scg-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandPlatformDeviceGroup(t, name, devicegroups.DeviceTypeV1Computer) },
				Config:      collisionConfig(strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProStaticComputerGroup_NameDifferingByTrailingSpaceDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-scg-ts-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { testhelpers.CreateOutOfBandProStaticComputerGroup(t, name) },
				Config:             collisionConfig(name + " "),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func twoGroupsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "one" {
  name = %q
}

resource "jamfplatform_pro_static_computer_group" "two" {
  depends_on = [jamfplatform_pro_static_computer_group.one]

  name = %q
}
`, nameOne, nameTwo)
}

func TestAccResource_ProStaticComputerGroup_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoGroupsConfig(one, two),
			},
			{
				Config:      twoGroupsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProStaticComputerGroup_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

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
