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

// twoGatewaysConfig declares two IPsec gateways, the second depending on the first
// so the creates are sequential. The IPsec form is used because the dedicated form
// needs a paid add-on. The provider block turns on the strict-only name rule, which
// is the only collision rule gateways have.
func twoGatewaysConfig(nameOne, nameTwo string) string {
	gateway := func(label, dependsOn, name, sourceAddress, jamfSubnet, peerHost string) string {
		return fmt.Sprintf(`
resource "jamfplatform_security_cloud_ztna_gateway" %q {
  %s
  name                      = %q
  egress_region             = %q
  ipsec_source_ip_addresses = [%q]

  contact = {
    name  = "Terraform Acceptance"
    email = "tf-acc@example.com"
  }

  ipsec = {
    key_exchange_protocol = "IKEv2"

    phase_1 = {
      encryption           = "AES-256"
      integrity            = "SHA-512"
      diffie_hellman_group = "Group 14 (modp2048)"
      sa_lifetime_seconds  = 28800
    }

    phase_2 = {
      encryption           = "AES-256"
      integrity            = "SHA-512"
      diffie_hellman_group = "Group 14 (modp2048)"
      sa_lifetime_seconds  = 28800
    }

    jamf_side = {
      host                             = "%%any"
      ike_domain_id                    = "wpa.wandera.com"
      subnet                           = %q
      authentication_secret            = "tf-acc-secret-1"
      authentication_secret_wo_version = 1
    }

    customer_side = {
      host          = %q
      ike_domain_id = "peer.tf-acc.example.com"
      subnets       = ["0.0.0.0/0"]
      vendor        = "strongSwan"
    }
  }
}
`, label, dependsOn, name, regionB, sourceAddress, jamfSubnet, peerHost)
	}
	return `
provider "jamfplatform" {
  name_collision_check = { include_unenforced = true }
}
` + gateway("one", "", nameOne, "3.66.107.208", "10.20.0.0/16", peerHostA) +
		gateway("two", "depends_on = [jamfplatform_security_cloud_ztna_gateway.one]", nameTwo, "3.121.43.105", "10.21.0.0/16", peerHostB)
}

func TestAccResource_SecurityCloudZtnaGateway_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoGatewaysConfig(one, two)},
			{
				Config:      twoGatewaysConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
