// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const strictCollisionProvider = `
provider "jamfplatform" {
  name_collision_check = { include_unenforced = true }
}
`

func TestAccResource_Blueprint_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-bp-collide-" + suffix
	planned := testBlueprintConfig(smartGroupHCL("collide"), fmt.Sprintf(`
		resource "jamfplatform_blueprints_blueprint" "test" {
			name          = %q
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
			]
		}
	`, strings.ToUpper(name)))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					group := testhelpers.CreateOutOfBandPlatformDeviceGroup(t, "tf-acc-bp-collide-grp-"+suffix, "COMPUTER")
					testhelpers.CreateOutOfBandBlueprint(t, name, group)
				},
				Config:      strictCollisionProvider + planned,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use\s+\(duplicate\s+check\).*already\s+exists\s+in\s+Jamf\s+Blueprints.*accepts\s+duplicates`),
			},
			{
				Config:             planned,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
