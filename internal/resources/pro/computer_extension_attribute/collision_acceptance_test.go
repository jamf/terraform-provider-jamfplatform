// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package computer_extension_attribute_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingComputerExtensionAttribute(t *testing.T, name string) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateComputerExtensionAttributeV1(ctx, &pro.ComputerExtensionAttributes{
		Name:                 name,
		DataType:             "STRING",
		InputType:            "TEXT",
		InventoryDisplayType: "GENERAL",
	})
	if err != nil {
		t.Fatalf("creating computer extension attribute %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteComputerExtensionAttributeV1(ctx, created.ID) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name              = %q
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = "GENERAL"
		}
	`, name)
}

func TestAccResource_ProComputerExtensionAttribute_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-cea-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingComputerExtensionAttribute(t, name) },
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

func TestAccResource_ProComputerExtensionAttribute_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mine := "tf-acc-collide-cea-mine-" + suffix
	taken := "tf-acc-collide-cea-taken-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: collisionConfig(mine)},
			{
				PreConfig:   func() { createExistingComputerExtensionAttribute(t, taken) },
				Config:      collisionConfig(taken),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}
