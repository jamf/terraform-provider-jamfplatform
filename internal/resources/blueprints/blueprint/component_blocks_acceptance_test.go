// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_Blueprint_ComponentBlocks exercises the ordered component_blocks authoring style:
// multiple named blocks, a per-block activation condition, plan stability on read-back, appending a
// block (including audio_accessory_settings — the appended-list-element path), and the same
// component type (passcode_policy) repeated across two blocks.
func TestAccResource_Blueprint_ComponentBlocks(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-bp-blocks-" + suffix
	res := "jamfplatform_blueprints_blueprint.blocks"

	threeBlocks := testBlueprintConfig(smartGroupHCL("blocks"), fmt.Sprintf(`
		resource "jamfplatform_blueprints_blueprint" "blocks" {
			name          = %q
			description   = "Acceptance test — safe to delete"
			deployed      = false
			device_groups = [jamfplatform_device_group.scope.id]

			component_blocks = [
				{
					name = "Passcode Policy"
					passcode_policy = {
						require_passcode = true
						minimum_length   = 6
					}
				},
				{
					name = "Software Update Settings"
					software_update_settings = {
						allow_standard_user_os_updates     = true
						automatic_download                 = "AlwaysOn"
						automatic_install_os_updates       = "AlwaysOn"
						automatic_install_security_updates = "AlwaysOn"
						recommended_cadence                = "Newest"
					}
				},
				{
					name                  = "Math Settings"
					activation_conditions = "ANY @property(jamf.device.groups) IN {'${jamfplatform_device_group.scope.id}'}"
					math_settings = {
						calculator_scientific_mode_enabled   = true
						system_behavior_keyboard_suggestions = true
						system_behavior_math_notes           = true
					}
				},
			]
		}
	`, name))

	fourBlocks := testBlueprintConfig(smartGroupHCL("blocks"), fmt.Sprintf(`
		resource "jamfplatform_blueprints_blueprint" "blocks" {
			name          = %q
			description   = "Acceptance test — safe to delete"
			deployed      = false
			device_groups = [jamfplatform_device_group.scope.id]

			component_blocks = [
				{
					name = "Passcode Policy"
					passcode_policy = {
						require_passcode = true
						minimum_length   = 8
					}
				},
				{
					name = "Software Update Settings"
					software_update_settings = {
						allow_standard_user_os_updates     = true
						automatic_download                 = "AlwaysOn"
						automatic_install_os_updates       = "AlwaysOn"
						automatic_install_security_updates = "AlwaysOn"
						recommended_cadence                = "Newest"
					}
				},
				{
					name = "Audio"
					audio_accessory_settings = {
						temporary_pairing_disabled = true
					}
				},
				{
					name = "Stricter Passcode"
					passcode_policy = {
						require_passcode = true
						minimum_length   = 10
					}
				},
			]
		}
	`, name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: threeBlocks,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(res, "id"),
					resource.TestCheckResourceAttr(res, "component_blocks.#", "3"),
					resource.TestCheckResourceAttr(res, "component_blocks.0.name", "Passcode Policy"),
					resource.TestCheckResourceAttr(res, "component_blocks.0.passcode_policy.minimum_length", "6"),
					resource.TestCheckResourceAttr(res, "component_blocks.1.name", "Software Update Settings"),
					resource.TestCheckResourceAttr(res, "component_blocks.2.name", "Math Settings"),
					resource.TestCheckResourceAttrSet(res, "component_blocks.2.activation_conditions"),
				),
			},
			{
				// Read-back of the three blocks must produce no diff.
				Config: threeBlocks,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Reorder-in-place edit + append an audio block (appended list element) and a
				// second passcode block (same component type in two blocks).
				Config: fourBlocks,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "component_blocks.#", "4"),
					resource.TestCheckResourceAttr(res, "component_blocks.0.passcode_policy.minimum_length", "8"),
					resource.TestCheckResourceAttr(res, "component_blocks.2.name", "Audio"),
					resource.TestCheckResourceAttr(res, "component_blocks.2.audio_accessory_settings.temporary_pairing_disabled", "true"),
					resource.TestCheckResourceAttr(res, "component_blocks.3.name", "Stricter Passcode"),
					resource.TestCheckResourceAttr(res, "component_blocks.3.passcode_policy.minimum_length", "10"),
				),
			},
			{
				// Read-back of the four blocks (including the defaulted audio fields) must be stable.
				Config: fourBlocks,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
