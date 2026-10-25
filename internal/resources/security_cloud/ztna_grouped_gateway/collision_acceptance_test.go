// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ztna_grouped_gateway_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_SecurityCloudZtnaGroupedGateway_NameCollisionIsStrictOnly(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-collide-gg-" + suffix

	config := func(providerBlock string) string {
		return providerBlock + fmt.Sprintf(`
			resource "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
				name                       = %q
				routing_strategy           = "Nearest"
				required_gateway_stability = "1 hour"
				gateway_ids                = ["aaaa", "bbbb"]
			}
		`, strings.ToUpper(name))
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					members := []string{
						testhelpers.CreateOutOfBandSecurityCloudDedicatedGateway(t, name+"-a"),
						testhelpers.CreateOutOfBandSecurityCloudDedicatedGateway(t, name+"-b"),
					}
					testhelpers.CreateOutOfBandSecurityCloudGroupedGateway(t, name, members)
				},
				Config: config(`
					provider "jamfplatform" {
						name_collision_check = { include_unenforced = true }
					}
				`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Security\s+Cloud.*accepts\s+duplicates`),
			},
			{
				Config:             config(""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
