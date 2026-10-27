// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package category_test

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

// createExistingCategory creates a category outside Terraform and removes it when
// the test ends, standing in for an object someone made in the web interface.
func createExistingCategory(t *testing.T, name string) string {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateCategoryV1(ctx, &pro.Category{Name: name, Priority: 9})
	if err != nil {
		t.Fatalf("creating category %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteCategoryV1(ctx, created.ID) })
	return created.ID
}

func TestAccResource_ProCategory_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-" + testhelpers.RunSuffix()
	var existingID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { existingID = createExistingCategory(t, name) },
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "test" {
						name     = %q
						priority = 7
					}
				`, strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
			{
				PreConfig: func() { t.Logf("existing category ID %s", existingID) },
				Config: fmt.Sprintf(`
					provider "jamfplatform" {
						name_collision_check = { mode = "off" }
					}
					resource "jamfplatform_pro_category" "test" {
						name     = %q
						priority = 7
					}
				`, strings.ToUpper(name)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccResource_ProCategory_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mine := "tf-acc-collide-mine-" + suffix
	taken := "tf-acc-collide-taken-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCategoryDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "test" {
						name     = %q
						priority = 7
					}
				`, mine),
			},
			{
				PreConfig: func() { createExistingCategory(t, taken) },
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "test" {
						name     = %q
						priority = 7
					}
				`, taken),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}

func TestAccResource_ProCategory_NameCollisionWhenNameIsUnknownAtPlan(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-unknown-" + testhelpers.RunSuffix()
	config := fmt.Sprintf(`
		resource "terraform_data" "name" {
			input = %q
		}
		resource "jamfplatform_pro_category" "test" {
			name     = "${terraform_data.name.output}-aa"
			priority = 7
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingCategory(t, name+"-aa") },
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}

// twoCategoriesConfig declares two categories, the second depending on the first
// so the creates are sequential: Jamf's duplicate check is not atomic, and two
// concurrent creates of one name can both succeed.
func twoCategoriesConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_category" "one" {
  name     = %q
  priority = 9
}

resource "jamfplatform_pro_category" "two" {
  depends_on = [jamfplatform_pro_category.one]

  name     = %q
  priority = 9
}
`, nameOne, nameTwo)
}

func TestAccResource_ProCategory_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoCategoriesConfig(one, two)},
			{
				Config:      twoCategoriesConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProCategory_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoCategoriesConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
