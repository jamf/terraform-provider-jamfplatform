//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func skipItemsConfig(name, skip string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false
  scope_serial_numbers                  = []
  skip_setup_items                      = %s
}
`, name, adeFixtureRef, skip)
}

// TestAccResource_ProComputerPrestageEnrollment_SkipSetupItems_RenamedPanes walks a configuration from the
// deprecated attribute names to the admin UI names. Each step plans empty on
// the next refresh: the replacement tracks the deprecated name while only that is
// configured, and the deprecated attribute follows the replacement once switched.
func TestAccResource_ProComputerPrestageEnrollment_SkipSetupItems_RenamedPanes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-skip-rename-" + suffix
	ade := adeFixtureBlock(suffix, token)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ade + skipItemsConfig(name, "{ tos = true }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.tos", "true"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.terms_and_conditions", "true"),
				),
			},
			{
				Config:   ade + skipItemsConfig(name, "{ tos = true }"),
				PlanOnly: true,
			},
			{
				Config: ade + skipItemsConfig(name, "{ terms_and_conditions = false }"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.terms_and_conditions", "false"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.tos", "false"),
				),
			},
			{
				Config:   ade + skipItemsConfig(name, "{ terms_and_conditions = false }"),
				PlanOnly: true,
			},
		},
	})
}

// TestAccResource_ProComputerPrestageEnrollment_SkipSetupItems_BothNamesConflict pins that setting a
// pane under both its deprecated and its current name fails at plan time.
func TestAccResource_ProComputerPrestageEnrollment_SkipSetupItems_BothNamesConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-skip-conflict-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      adeFixtureBlock(suffix, token) + skipItemsConfig(name, "{ tos = true, terms_and_conditions = true }"),
				ExpectError: regexp.MustCompile(`(?s)tos.*terms_and_conditions|terms_and_conditions.*tos`),
			},
		},
	})
}

// TestAccResource_ProComputerPrestageEnrollment_SkipSetupItems_LiquidGlass covers the Liquid Glass pane, which
// the Jamf Pro admin UI lists and the provider did not expose before.
func TestAccResource_ProComputerPrestageEnrollment_SkipSetupItems_LiquidGlass(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-skip-liquid-" + suffix
	ade := adeFixtureBlock(suffix, token)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ade + skipItemsConfig(name, "{ liquid_glass = true }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.liquid_glass", "true"),
				),
			},
			{
				Config:   ade + skipItemsConfig(name, "{ liquid_glass = true }"),
				PlanOnly: true,
			},
		},
	})
}
