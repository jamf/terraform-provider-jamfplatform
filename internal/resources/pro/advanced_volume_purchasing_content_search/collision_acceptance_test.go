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

// twoAdvancedVolumePurchasingContentSearchesConfig declares two resources. The second depends on the first so
// the creates are sequential: Jamf's duplicate check is not atomic, and two
// concurrent creates of one name can both succeed, which would leave a test
// asserting on a race instead of on the refusal.
func twoAdvancedVolumePurchasingContentSearchesConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "one" {
  name = %[1]q

  criteria = [
    {
      name        = "Name"
      search_type = "like"
      value       = "Office"
    },
  ]

  display_fields = ["Name"]
}

resource "jamfplatform_pro_advanced_volume_purchasing_content_search" "two" {
  depends_on = [jamfplatform_pro_advanced_volume_purchasing_content_search.one]

  name = %[2]q

  criteria = [
    {
      name        = "Name"
      search_type = "like"
      value       = "Office"
    },
  ]

  display_fields = ["Name"]
}
`, nameOne, nameTwo)
}

func TestAccResource_ProAdvancedVolumePurchasingContentSearch_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoAdvancedVolumePurchasingContentSearchesConfig(one, two),
			},
			{
				Config:      twoAdvancedVolumePurchasingContentSearchesConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProAdvancedVolumePurchasingContentSearch_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoAdvancedVolumePurchasingContentSearchesConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
