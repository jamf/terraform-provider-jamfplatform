// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package inventory_preload_record_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingInventoryPreloadRecord(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	created, err := c.CreateInventoryPreloadRecordV2(ctx, &pro.InventoryPreloadRecordV2{SerialNumber: name, DeviceType: "Mobile Device"})
	if err != nil {
		t.Fatalf("creating inventory preload record %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteInventoryPreloadRecordV2(ctx, created.ID) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_inventory_preload_record" "test" {
			serial_number = %q
			device_type   = "Computer"
		}
	`, name)
}

func TestAccResource_ProInventoryPreloadRecord_SerialNumberCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "ZZCOLLIDE" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingInventoryPreloadRecord(t, name) },
				Config:      collisionConfig(strings.ToLower(name) + " "),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Serial\s+number\s+already\s+in\s+use.*inventory\s+preload\s+record.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProInventoryPreloadRecord_SerialNumberWithLeadingSpaceDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "ZZCOLLIDEacc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingInventoryPreloadRecord(t, name) },
				Config:             collisionConfig(" " + name),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
