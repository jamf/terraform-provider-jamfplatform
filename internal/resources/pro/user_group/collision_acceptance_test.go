// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package user_group_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_user_group" "test" {
  name       = %q
  group_type = "static"
}
`, name)
}

func TestAccResource_ProUserGroup_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ug-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandClassicUserGroup(t, name) },
				Config:      collisionConfig(name + " "),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*user\s+group.*leading\s+and\s+trailing\s+spaces.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProUserGroup_NameDifferingByCaseDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ug-case-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { testhelpers.CreateOutOfBandClassicUserGroup(t, name) },
				Config:             collisionConfig(strings.ToUpper(name)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func twoUserGroupsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_user_group" "one" {
  name       = %q
  group_type = "static"
}

resource "jamfplatform_pro_user_group" "two" {
  depends_on = [jamfplatform_pro_user_group.one]

  name       = %q
  group_type = "static"
}
`, nameOne, nameTwo)
}

func TestAccResource_ProUserGroup_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoUserGroupsConfig(one, two),
			},
			{
				Config:      twoUserGroupsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProUserGroup_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoUserGroupsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
