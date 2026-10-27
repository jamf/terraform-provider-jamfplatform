// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package advanced_user_search_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingAdvancedUserSearch(t *testing.T, name string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateAdvancedUserSearchByID(ctx, "0", &proclassic.AdvancedUserSearch{Name: &name})
	if err != nil || created.ID == nil {
		t.Fatalf("creating advanced user search %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteAdvancedUserSearchByID(ctx, id) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_user_search" "test" {
			name = %q

			criteria = [
				{
					name        = "Full Name"
					search_type = "like"
					value       = "a"
				},
			]

			display_fields = ["Full Name"]
		}
	`, name)
}

func TestAccResource_ProAdvancedUserSearch_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-aus-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingAdvancedUserSearch(t, name) },
				Config:      collisionConfig(strings.ToUpper(name) + " "),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*advanced\s+user\s+search.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProAdvancedUserSearch_NameDifferingByAccentDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-aus-acc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingAdvancedUserSearch(t, name) },
				Config:             collisionConfig(strings.Replace(name, "e", "é", 1)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// twoSearchesConfig declares two advanced user searches. The second depends on
// the first so the creates are sequential: Jamf's duplicate check is not atomic,
// and two concurrent creates of one name can both succeed, which would leave a
// test asserting on a race instead of on the refusal.
func twoSearchesConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_advanced_user_search" "one" {
			name = %q

			criteria = [
				{
					name        = "Full Name"
					search_type = "like"
					value       = "a"
				},
			]

			display_fields = ["Full Name"]
		}

		resource "jamfplatform_pro_advanced_user_search" "two" {
			depends_on = [jamfplatform_pro_advanced_user_search.one]

			name = %q

			criteria = [
				{
					name        = "Full Name"
					search_type = "like"
					value       = "a"
				},
			]

			display_fields = ["Full Name"]
		}
	`, nameOne, nameTwo)
}

func TestAccResource_ProAdvancedUserSearch_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoSearchesConfig(one, two),
			},
			{
				Config:      twoSearchesConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProAdvancedUserSearch_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoSearchesConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating\s+Jamf\s+Pro\s+advanced\s+user\s+search.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
