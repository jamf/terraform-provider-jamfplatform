// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package advanced_mobile_device_search_test

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

func createExistingAdvancedMobileDeviceSearch(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	criteria := []pro.SmartSearchCriterion{{AndOr: "and", Name: "Managed", SearchType: "is", Value: "Unmanaged"}}
	fields := []string{"Display Name"}
	created, err := c.CreateAdvancedMobileDeviceSearchV1(ctx, &pro.AdvancedSearch{Name: name, Criteria: &criteria, DisplayFields: &fields})
	if err != nil {
		t.Fatalf("creating advanced mobile device search %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteAdvancedMobileDeviceSearchV1(ctx, created.ID) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_mobile_device_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Managed"
					search_type = "is"
					value       = "Unmanaged"
				},
			]

			display_fields = ["Display Name"]
		}
	`, name)
}

func TestAccResource_ProAdvancedMobileDeviceSearch_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-advanced-mobile-device-search-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingAdvancedMobileDeviceSearch(t, name) },
				Config:      collisionConfig(strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*advanced\s+mobile\s+device\s+search.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProAdvancedMobileDeviceSearch_NameDifferingByAccentDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-advanced-mobile-device-search-acc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingAdvancedMobileDeviceSearch(t, name) },
				Config:             collisionConfig(strings.Replace(name, "e", "é", 1)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
