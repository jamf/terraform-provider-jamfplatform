// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package advanced_volume_purchasing_content_search_test

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

func createExistingAdvancedVolumePurchasingContentSearch(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	criteria := []pro.SmartSearchCriterion{{AndOr: "and", Name: "Name", SearchType: "like", Value: "Office"}}
	fields := []string{"Name"}
	created, err := c.CreateAdvancedUserContentSearchV1(ctx, &pro.AdvancedUserContentSearch{Name: name, Criteria: &criteria, DisplayFields: &fields})
	if err != nil {
		t.Fatalf("creating advanced volume purchasing content search %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteAdvancedUserContentSearchV1(ctx, created.ID) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Name"
					search_type = "like"
					value       = "Office"
				},
			]

			display_fields = ["Name"]
		}
	`, name)
}

func TestAccResource_ProAdvancedVolumePurchasingContentSearch_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-advanced-volume-purchasing-content-search-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingAdvancedVolumePurchasingContentSearch(t, name) },
				Config:      collisionConfig(name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*advanced\s+volume\s+purchasing\s+content\s+search.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProAdvancedVolumePurchasingContentSearch_NameDifferingByCaseDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-advanced-volume-purchasing-content-search-acc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingAdvancedVolumePurchasingContentSearch(t, name) },
				Config:             collisionConfig(strings.ToUpper(name)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
