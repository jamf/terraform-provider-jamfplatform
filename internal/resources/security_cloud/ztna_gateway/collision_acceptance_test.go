// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ztna_gateway_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_SecurityCloudZtnaGateway_NameCollisionIsStrictOnly(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-collide-gw-" + suffix

	config := func(providerBlock string) string {
		return providerBlock + fmt.Sprintf(`
			resource "jamfplatform_security_cloud_ztna_gateway" "test" {
				name          = %q
				egress_region = %q

				contact = {
					name  = "Terraform Acceptance"
					email = "tf-acc@example.com"
				}
			}
		`, strings.ToUpper(name), regionA)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testhelpers.CreateOutOfBandSecurityCloudDedicatedGateway(t, name) },
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
