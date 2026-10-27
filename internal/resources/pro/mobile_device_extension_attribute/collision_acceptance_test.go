// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_extension_attribute_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingMobileDeviceExtensionAttribute(t *testing.T, name string) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateMobileDeviceExtensionAttributeV1(ctx, &pro.MobileDeviceExtensionAttributes{
		Name:                 name,
		DataType:             "STRING",
		InputType:            "TEXT",
		InventoryDisplayType: "GENERAL",
	})
	if err != nil {
		t.Fatalf("creating mobile device extension attribute %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteMobileDeviceExtensionAttributeV1(ctx, created.ID) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_extension_attribute" "test" {
			name              = %q
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = "GENERAL"
		}
	`, name)
}

func TestAccResource_ProMobileDeviceExtensionAttribute_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdea-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingMobileDeviceExtensionAttribute(t, name) },
				Config:      collisionConfig(name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
			{
				Config:      collisionConfig(" " + name + " "),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceExtensionAttribute_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mine := "tf-acc-collide-mdea-mine-" + suffix
	taken := "tf-acc-collide-mdea-taken-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: collisionConfig(mine)},
			{
				PreConfig:   func() { createExistingMobileDeviceExtensionAttribute(t, taken) },
				Config:      collisionConfig(taken),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}

func twoConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_extension_attribute" "one" {
			name              = %q
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = "GENERAL"
		}

		resource "jamfplatform_pro_mobile_device_extension_attribute" "two" {
			depends_on = [jamfplatform_pro_mobile_device_extension_attribute.one]

			name              = %q
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = "GENERAL"
		}
	`, nameOne, nameTwo)
}

func TestAccResource_ProMobileDeviceExtensionAttribute_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-mdea-one-" + suffix
	two := "tf-acc-collide-mdea-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoConfig(one, two)},
			{
				Config:      twoConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceExtensionAttribute_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdea-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
