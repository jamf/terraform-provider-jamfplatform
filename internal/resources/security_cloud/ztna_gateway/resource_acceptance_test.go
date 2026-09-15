// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ztna_gateway_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// Every IPsec gateway in this suite gets its own egress region and its own
// Jamf-side subnet. Creating a second IPsec gateway alongside one that was
// mid-reprovision in the same region answered `409 CONFLICT` during wire probing,
// so the tests keep well clear of each other rather than relying on that being
// coincidental.
const (
	regionA = "Europe - UK"
	regionB = "Europe - Germany"
	regionC = "Europe - Ireland"
)

// regionALabel is the same value under a name that reads correctly where a test
// asserts what the schema reports rather than what it was configured with.
const regionALabel = regionA

// Peer addresses come from RFC 5737's TEST-NET blocks, which are reserved for
// documentation and never routed.
const (
	peerHostA = "198.51.100.7"
	peerHostB = "198.51.100.8"
)

// testAccCheckGatewayDestroy verifies gateways created during the test were
// destroyed.
func testAccCheckGatewayDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_ztna_gateway" {
				continue
			}
			_, err := c.GetZtnaGatewayV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud ZTNA gateway %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Security Cloud ZTNA gateway %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_SecurityCloudZtnaGateway_InternetGateway covers the simpler of
// the two forms: create, an in-place update of every mutable field, and import.
//
// It also pins the derived discriminator from the outside. The config never
// mentions dedicated egress IPs, yet the gateway has to come back as one — if the
// provider stopped sending the flag the create would be refused outright as
// configured as neither form.
func TestAccResource_SecurityCloudZtnaGateway_InternetGateway(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gw-inet-" + suffix
	nameUpdated := "tf-acc-jsc-gw-inet-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "test" {
						name       = %q
						egress_region = %q
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}
					}
				`, name, regionA, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_gateway.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "egress_region", regionA),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "enabled", "true"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "contact.name", "Terraform Acceptance"),
					resource.TestCheckNoResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.key_exchange_protocol"),
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_gateway.test", "status.state"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "test" {
						name       = %q
						egress_region = %q
						enabled    = false
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance Updated"
							email = "tf-acc-updated@example.com"
						}
					}
				`, nameUpdated, regionA, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "enabled", "false"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "contact.email", "tf-acc-updated@example.com"),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_ztna_gateway.test",
				ImportState:       true,
				ImportStateVerify: true,
				// status.state is excluded because it is not a property of the
				// configuration but a server-side deployment state that advances on its
				// own. The step above sets enabled = false, and Jamf Security Cloud then
				// moves the gateway from PENDING to DISABLED asynchronously — so the state
				// stored by that step and the fresh read this import performs can
				// legitimately disagree, and did: roughly one run in four failed with
				// `- "status.state": "PENDING"` / `+ "status.state": "DISABLED"`. Comparing
				// the two is a race, not a check. Same reason STYLE_GUIDE keeps
				// status.updatedAt out of the schema entirely.
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"status.state",
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGateway_IPSecGateway covers the dense form:
// create with a full tunnel, then change the cipher suites, the customer subnets
// and the pre-shared key in one update, then import.
//
// Import deliberately ignores the two fields the wire cannot supply. The
// pre-shared key is WriteOnly and Jamf Security Cloud never returns it, so an
// imported gateway has neither it nor its rotation trigger — which is exactly the
// contract, and worth asserting rather than papering over.
func TestAccResource_SecurityCloudZtnaGateway_IPSecGateway(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gw-ipsec-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ipsecGatewayConfig(name, regionB, tenantID, "10.20.0.0/16", peerHostA, "AES-256", "SHA-512", "Group 14 (modp2048)", 28800, `["0.0.0.0/0"]`, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_gateway.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.key_exchange_protocol", "IKEv2"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.phase_1.encryption", "AES-256"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.phase_1.diffie_hellman_group", "Group 14 (modp2048)"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.jamf_side.subnet", "10.20.0.0/16"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.jamf_side.auth_method", "psk"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.customer_side.vendor", "strongSwan"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.customer_side.subnets.#", "1"),
					resource.TestCheckNoResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.jamf_side.authentication_secret"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.jamf_side.authentication_secret_wo_version", "1"),
				),
			},
			{
				Config: ipsecGatewayConfig(name, regionB, tenantID, "10.20.0.0/16", peerHostB, "AES-128", "SHA-256", "Group 19 (ecp256)", 3600, `["10.30.0.0/16", "192.168.50.0/24"]`, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.phase_1.encryption", "AES-128"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.phase_1.integrity", "SHA-256"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.phase_1.diffie_hellman_group", "Group 19 (ecp256)"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.phase_1.sa_lifetime_seconds", "3600"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.customer_side.host", peerHostB),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.customer_side.subnets.#", "2"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.jamf_side.authentication_secret_wo_version", "2"),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_ztna_gateway.test",
				ImportState:       true,
				ImportStateVerify: true,
				// status.state: see the internet-gateway import step above — a
				// server-side deployment state, not a configuration property, and racy to
				// compare across two reads.
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"status.state",
					"ipsec.jamf_side.authentication_secret",
					"ipsec.jamf_side.authentication_secret_wo_version",
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGateway_FormChangeForcesReplace pins the
// immutability of the gateway's form. Jamf Security Cloud refuses to add an IPsec
// tunnel to an existing internet gateway, and silently ignores an attempt to flip
// the dedicated-egress flag — so the provider must plan a replacement rather than
// an update that would either fail or, worse, appear to succeed and change nothing.
func TestAccResource_SecurityCloudZtnaGateway_FormChangeForcesReplace(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gw-form-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "test" {
						name       = %q
						egress_region = %q
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}
					}
				`, name, regionC, tenantID),
				Check: resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_gateway.test", "id"),
			},
			{
				Config: ipsecGatewayConfig(name, regionC, tenantID, "192.168.60.0/24", peerHostA, "AES-256", "SHA-512", "Group 14 (modp2048)", 28800, `["0.0.0.0/0"]`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jamfplatform_security_cloud_ztna_gateway.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_gateway.test", "ipsec.key_exchange_protocol", "IKEv2"),
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGateway_AvailabilityZonesRequireIPSec pins the
// cross-field rule at plan time. The server refuses the combination with a message
// about a field the provider does not even expose, so catching it here is the only
// way the user hears about the actual conflict.
func TestAccResource_SecurityCloudZtnaGateway_SourceAddressesRequireIPSec(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "test" {
						name               = "tf-acc-jsc-gw-az-%s"
						egress_region      = %q
						tenant_ids         = [%q]
						ipsec_source_ip_addresses = ["3.9.67.90"]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}
					}
				`, suffix, regionA, tenantID),
				ExpectError: regexpSourceAddressesNeedIPSec,
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGateway_InvalidJamfSubnetRejectedAtPlan pins the
// private-range check. The server answers with the whole `ipsec` block named and
// nothing about which address, so the plan-time check is what makes the mistake
// findable.
func TestAccResource_SecurityCloudZtnaGateway_InvalidJamfSubnetRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ipsecGatewayConfig("tf-acc-jsc-gw-badsubnet-"+suffix, regionA, tenantID, "8.8.8.0/24", peerHostA, "AES-256", "SHA-512", "Group 14 (modp2048)", 28800, `["0.0.0.0/0"]`, 1),
				ExpectError: regexpSubnetNotPrivate,
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGateway_InvalidCipherRejectedAtPlan pins the
// enum checks. An unknown cipher reaches the user as "Request body is missing or
// malformed" with no field and no value, which is why every enum is validated
// before the request is built.
func TestAccResource_SecurityCloudZtnaGateway_InvalidCipherRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ipsecGatewayConfig("tf-acc-jsc-gw-badcipher-"+suffix, regionA, tenantID, "10.40.0.0/16", peerHostA, "AES-999", "SHA-512", "Group 14 (modp2048)", 28800, `["0.0.0.0/0"]`, 1),
				ExpectError: regexpInvalidAttributeValueMatch,
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGateway_LowercaseVendorRejectedAtPlan covers the
// case-sensitivity trap specifically. `strongswan` is the spelling a reader of the
// admin UI would guess, and the server's reply to it says nothing at all.
func TestAccResource_SecurityCloudZtnaGateway_LowercaseVendorRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "test" {
						name       = "tf-acc-jsc-gw-vendor-%s"
						egress_region = %q
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}

						ipsec = {
							key_exchange_protocol = "IKEv2"
							phase_1 = { encryption = "AES-256", integrity = "SHA-512", diffie_hellman_group = "Group 14 (modp2048)", sa_lifetime_seconds = 28800 }
							phase_2 = { encryption = "AES-256", integrity = "SHA-512", diffie_hellman_group = "Group 14 (modp2048)", sa_lifetime_seconds = 28800 }
							jamf_side = {
								host                             = "%%any"
								ike_domain_id                    = "wpa.wandera.com"
								subnet                           = "10.50.0.0/16"
								authentication_secret            = "tf-acc-secret-0001"
								authentication_secret_wo_version = 1
							}
							customer_side = {
								host          = %q
								ike_domain_id = "peer.tf-acc.example.com"
								subnets       = ["0.0.0.0/0"]
								vendor        = "strongswan"
							}
						}
					}
				`, suffix, regionA, tenantID, peerHostA),
				ExpectError: regexpInvalidAttributeValueMatch,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGateway_ByIDAndName covers both lookup paths
// of the singular data source against one live gateway.
func TestAccDataSource_SecurityCloudZtnaGateway_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gw-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "src" {
						name       = %q
						egress_region = %q
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}
					}

					data "jamfplatform_security_cloud_ztna_gateway" "by_id" {
						id = jamfplatform_security_cloud_ztna_gateway.src.id
					}

					data "jamfplatform_security_cloud_ztna_gateway" "by_name" {
						name       = jamfplatform_security_cloud_ztna_gateway.src.name
						depends_on = [jamfplatform_security_cloud_ztna_gateway.src]
					}
				`, name, regionA, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_ztna_gateway.by_id", "name", "jamfplatform_security_cloud_ztna_gateway.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_gateway.by_id", "egress_region", regionA),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_gateway.by_id", "dedicated_egress_ips_enabled", "true"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_ztna_gateway.by_name", "id", "jamfplatform_security_cloud_ztna_gateway.src", "id"),
				),
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGateway_RequiresExactlyOneSelector pins the
// config validator on the singular data source, from both directions.
func TestAccDataSource_SecurityCloudZtnaGateway_RequiresExactlyOneSelector(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "jamfplatform_security_cloud_ztna_gateway" "neither" {}`,
				ExpectError: regexpExactlyOneSelector,
			},
			{
				Config: `
					data "jamfplatform_security_cloud_ztna_gateway" "both" {
						id   = "a1b2"
						name = "Some Gateway"
					}
				`,
				ExpectError: regexpExactlyOneSelector,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGateways_ListsCreatedGateway checks the plural
// data source surfaces a gateway created in the same apply. It asserts presence
// rather than a total, because the tenant may hold gateways this test did not
// create.
func TestAccDataSource_SecurityCloudZtnaGateways_ListsCreatedGateway(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gws-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "src" {
						name       = %q
						egress_region = %q
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}
					}

					data "jamfplatform_security_cloud_ztna_gateways" "all" {
						depends_on = [jamfplatform_security_cloud_ztna_gateway.src]
					}
				`, name, regionA, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_gateways.all", "id", "ztna_gateways"),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_security_cloud_ztna_gateways.all", "gateways.*", map[string]string{
						"name": name,
					}),
				),
			},
		},
	})
}

// TestAccListResource_SecurityCloudZtnaGateway_Basic exercises the list resource
// via the `terraform query` workflow. The endpoint takes no filter, so step 2
// asserts the created gateway appears among the results rather than pinning a
// total.
//
// Requires Terraform 1.14+ (list resources).
func TestAccListResource_SecurityCloudZtnaGateway_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gw-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_gateway" "src" {
						name       = %q
						egress_region = %q
						tenant_ids = [%q]

						contact = {
							name  = "Terraform Acceptance"
							email = "tf-acc@example.com"
						}
					}
				`, name, regionA, tenantID),
				Check: resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_gateway.src", "id"),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_security_cloud_ztna_gateway" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_security_cloud_ztna_gateway.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("egress_region"), KnownValue: knownvalue.StringExact(regionALabel)},
						},
					),
				},
			},
		},
	})
}

// ipsecGatewayConfig renders a full IPsec gateway config. The pre-shared key is
// literal rather than parameterised because it is WriteOnly — it never reaches
// state, so there is nothing for a test to correlate it against.
func ipsecGatewayConfig(name, region, tenantID, jamfSubnet, peerHost, encryption, integrity, dhGroup string, lifetime int, customerSubnets string, woVersion int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_security_cloud_ztna_gateway" "test" {
			name       = %q
			egress_region = %q
			tenant_ids = [%q]

			contact = {
				name  = "Terraform Acceptance"
				email = "tf-acc@example.com"
			}

			ipsec = {
				key_exchange_protocol = "IKEv2"

				phase_1 = {
					encryption           = %q
					integrity            = %q
					diffie_hellman_group = %q
					sa_lifetime_seconds  = %d
				}

				phase_2 = {
					encryption           = %q
					integrity            = %q
					diffie_hellman_group = %q
					sa_lifetime_seconds  = %d
				}

				jamf_side = {
					host                             = "%%any"
					ike_domain_id                    = "wpa.wandera.com"
					subnet                           = %q
					authentication_secret            = "tf-acc-secret-%d"
					authentication_secret_wo_version = %d
				}

				customer_side = {
					host          = %q
					ike_domain_id = "peer.tf-acc.example.com"
					subnets       = %s
					vendor        = "strongSwan"
				}
			}
		}
	`, name, region, tenantID,
		encryption, integrity, dhGroup, lifetime,
		encryption, integrity, dhGroup, lifetime,
		jamfSubnet, woVersion, woVersion,
		peerHost, customerSubnets)
}

// Expected-error patterns for the plan- and apply-time refusals. Terraform wraps
// diagnostic text at roughly 80 columns, so each pattern matches a short phrase
// that cannot be split across a line break.
// TestAccDataSource_SecurityCloudZtnaGateway_EmptyIDRefused pins the guard for the
// selector mistake ExactlyOneOf cannot catch.
//
// A set-but-empty `id` counts as configured, so `id = var.x` with an unset variable
// satisfies the config validator and then falls through to whichever lookup is
// left. Without the guard the name branch runs with a null name, and the operator
// gets an SDK-internal string about an attribute they never set.
func TestAccDataSource_SecurityCloudZtnaGateway_EmptyIDRefused(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_ztna_gateway" "empty_id" {
						id = ""
					}
				`,
				ExpectError: regexpEmptyID,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGateway_NameNotFoundIsWritten pins that a name
// matching nothing produces a written diagnostic rather than the resolver's
// synthetic 404 body, which reads as an internal error and names no remedy.
func TestAccDataSource_SecurityCloudZtnaGateway_NameNotFoundIsWritten(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_ztna_gateway" "missing" {
						name = "tf-acc-no-such-ztna_gateway-zzz"
					}
				`,
				ExpectError: regexpNameNotFound,
			},
		},
	})
}

var (
	regexpSourceAddressesNeedIPSec   = regexp.MustCompile(`IPsec source addresses require an IPsec gateway`)
	regexpSubnetNotPrivate           = regexp.MustCompile(`is not a private range`)
	regexpInvalidAttributeValueMatch = regexp.MustCompile(`Invalid Attribute Value Match`)
	regexpExactlyOneSelector         = regexp.MustCompile(`Exactly one of these attributes must be configured`)
	regexpEmptyID                    = regexp.MustCompile(`ID is empty`)
	regexpNameNotFound               = regexp.MustCompile(`on this tenant is named`)
)
