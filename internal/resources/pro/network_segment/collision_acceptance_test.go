// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package network_segment_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProNetworkSegment_NameCollision proves the plan-time check fires: a second resource
// whose name differs from the first only in case is refused at plan, before any write.
func TestAccResource_ProNetworkSegment_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-network-segment-" + testhelpers.RunSuffix()
	existing := fmt.Sprintf(`
					resource "jamfplatform_pro_network_segment" "existing" {
						name             = %q
						starting_address = "10.10.10.0"
						ending_address   = "10.10.10.255"
					}
				`, name)
	duplicate := fmt.Sprintf(`
					resource "jamfplatform_pro_network_segment" "duplicate" {
						name             = %q
						starting_address = "10.10.11.0"
						ending_address   = "10.10.11.255"
					}
				`, strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + duplicate,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
		},
	})
}

// twoNetworkSegmentsConfig declares two resources. The second depends on the first so
// the creates are sequential: Jamf's duplicate check is not atomic, and two
// concurrent creates of one name can both succeed, which would leave a test
// asserting on a race instead of on the refusal.
func twoNetworkSegmentsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_network_segment" "one" {
  name             = %[1]q
  starting_address = "10.10.11.0"
  ending_address   = "10.10.11.255"
}

resource "jamfplatform_pro_network_segment" "two" {
  depends_on = [jamfplatform_pro_network_segment.one]

  name             = %[2]q
  starting_address = "10.10.12.0"
  ending_address   = "10.10.12.255"
}
`, nameOne, nameTwo)
}

func TestAccResource_ProNetworkSegment_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoNetworkSegmentsConfig(one, two),
			},
			{
				Config:      twoNetworkSegmentsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProNetworkSegment_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoNetworkSegmentsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
