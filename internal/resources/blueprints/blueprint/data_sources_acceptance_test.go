// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The three blueprint data sources (`jamfplatform_blueprints`, `_blueprints_component`,
// `_blueprints_components`) live in their own packages with no external test package, so their
// acceptance tests sit beside the resource's, as TestAccDataSource_Blueprint does. These cover what
// that one does not: the singular component lookup, the plural search filter, and the `timeouts`
// attribute every read now honours.

func TestAccDataSource_BlueprintComponent_ByIdentifier(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_blueprints_components" "all" {}

					data "jamfplatform_blueprints_component" "first" {
						id = data.jamfplatform_blueprints_components.all.components[0].identifier
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_blueprints_component.first", "identifier",
						"data.jamfplatform_blueprints_components.all", "components.0.identifier",
					),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_blueprints_component.first", "name",
						"data.jamfplatform_blueprints_components.all", "components.0.name",
					),
					resource.TestCheckResourceAttrSet("data.jamfplatform_blueprints_component.first", "supported_os.%"),
				),
			},
		},
	})
}

func TestAccDataSource_Blueprints_SearchFilter(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ds-blueprints-search-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testBlueprintConfig(smartGroupHCL("dssearch"), fmt.Sprintf(`
					resource "jamfplatform_blueprints_blueprint" "source" {
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
						]
					}

					data "jamfplatform_blueprints" "matching" {
						search = upper(jamfplatform_blueprints_blueprint.source.name)
					}

					data "jamfplatform_blueprints" "nothing" {
						search     = "tf-acc-no-such-blueprint-%s"
						depends_on = [jamfplatform_blueprints_blueprint.source]
					}
				`, name, suffix)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_blueprints.matching", "blueprints.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_blueprints.matching", "blueprints.0.name", name),
					resource.TestCheckResourceAttrSet("data.jamfplatform_blueprints.matching", "blueprints.0.id"),
					resource.TestCheckResourceAttr("data.jamfplatform_blueprints.nothing", "blueprints.#", "0"),
				),
			},
		},
	})
}

func TestAccDataSource_Blueprints_ConfiguredTimeouts(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_blueprints" "all" {
						timeouts = {
							read = "5m"
						}
					}

					data "jamfplatform_blueprints_components" "all" {
						timeouts = {
							read = "5m"
						}
					}

					data "jamfplatform_blueprints_component" "first" {
						id = data.jamfplatform_blueprints_components.all.components[0].identifier
						timeouts = {
							read = "5m"
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_blueprints.all", "timeouts.read", "5m"),
					resource.TestCheckResourceAttr("data.jamfplatform_blueprints_components.all", "timeouts.read", "5m"),
					resource.TestCheckResourceAttr("data.jamfplatform_blueprints_component.first", "timeouts.read", "5m"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_blueprints.all", "blueprints.#"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_blueprints_component.first", "name"),
				),
			},
		},
	})
}

func TestAccDataSource_Blueprints_MalformedTimeout(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_blueprints" "all" {
						timeouts = {
							read = "soon"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`(?is)(invalid|duration)`),
			},
		},
	})
}
