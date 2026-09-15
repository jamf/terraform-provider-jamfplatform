// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package dns_zone_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// Acceptance name servers use TEST-NET addresses from RFC 5737, which are
// reserved for documentation and never routed. Jamf Security Cloud accepts them
// — it refuses private and loopback ranges, not documentation ranges — so the
// tests exercise the real write path without naming a resolver anyone operates.
const (
	testNameServerIPOne   = "203.0.113.53"
	testNameServerIPTwo   = "198.51.100.53"
	testNameServerIPThree = "192.0.2.53"
)

// testAccCheckDNSZoneDestroy verifies DNS zones created during the test were
// destroyed.
func testAccCheckDNSZoneDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_dns_zone" {
				continue
			}
			_, err := c.GetDnsZoneV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud DNS zone %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Security Cloud DNS zone %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_SecurityCloudDNSZone_Basic covers create, in-place update of
// every writable field, and import.
//
// The update step deliberately changes all three at once — rename, swap the
// domain set, and replace the name server list — because the provider sends the
// whole object on every update. A step that changed one field would pass even if
// the patch dropped the other two.
func TestAccResource_SecurityCloudDNSZone_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 2)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-dns-zone-" + suffix
	nameUpdated := "tf-acc-jsc-dns-zone-updated-" + suffix
	domain := "tf-acc-" + suffix + ".example.com"
	domainUpdated := "tf-acc-updated-" + suffix + ".example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "test" {
						name    = %q
						domains = [%q, %q]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}
				`, name, domain, "*."+domain, testNameServerIPOne, gateways[0]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_dns_zone.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "domains.#", "2"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_security_cloud_dns_zone.test", "domains.*", domain),
					resource.TestCheckTypeSetElemAttr("jamfplatform_security_cloud_dns_zone.test", "domains.*", "*."+domain),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "authoritative_name_servers.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_zone.test", "authoritative_name_servers.*", map[string]string{
						"ip_address": testNameServerIPOne,
						"gateway_id": gateways[0],
					}),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "test" {
						name    = %q
						domains = [%q]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}
				`, nameUpdated, domainUpdated, testNameServerIPTwo, gateways[0], testNameServerIPThree, gateways[1]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "domains.#", "1"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_security_cloud_dns_zone.test", "domains.*", domainUpdated),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "authoritative_name_servers.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_zone.test", "authoritative_name_servers.*", map[string]string{
						"ip_address": testNameServerIPTwo,
						"gateway_id": gateways[0],
					}),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_zone.test", "authoritative_name_servers.*", map[string]string{
						"ip_address": testNameServerIPThree,
						"gateway_id": gateways[1],
					}),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_dns_zone.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSZone_ServerSortedDomainsProduceNoDiff is the
// regression guard for the Set choice on `domains`. Jamf Security Cloud stores
// the domain list sorted byte-wise ascending, so a config authored in any other
// order reads back reordered. As a Set that is no diff; as a List it would fail
// "provider produced inconsistent result after apply". The config authors the
// domains in deliberately reverse-sorted order and the step asserts the follow-up
// plan is empty.
func TestAccResource_SecurityCloudDNSZone_ServerSortedDomainsProduceNoDiff(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-dns-zone-order-" + suffix
	domain := "tf-acc-order-" + suffix + ".example.com"

	config := fmt.Sprintf(`
		resource "jamfplatform_security_cloud_dns_zone" "test" {
			name    = %q
			domains = [%q, %q, %q]

			authoritative_name_servers = [
				{
					ip_address = %q
					gateway_id = %q
				},
			]
		}
	`, name, "zzz."+domain, "aaa."+domain, "mmm."+domain, testNameServerIPOne, gateways[0])

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_zone.test", "domains.#", "3"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSZone_Disappears covers the drift path: a zone
// deleted out from under Terraform must be dropped from state and planned for
// re-creation rather than failing the refresh.
func TestAccResource_SecurityCloudDNSZone_Disappears(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-dns-zone-gone-" + suffix
	domain := "tf-acc-gone-" + suffix + ".example.com"

	config := fmt.Sprintf(`
		resource "jamfplatform_security_cloud_dns_zone" "test" {
			name    = %q
			domains = [%q]

			authoritative_name_servers = [
				{
					ip_address = %q
					gateway_id = %q
				},
			]
		}
	`, name, domain, testNameServerIPOne, gateways[0])

	var zoneID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_dns_zone.test", "id"),
					captureDNSZoneID("jamfplatform_security_cloud_dns_zone.test", &zoneID),
				),
			},
			{
				PreConfig: func() {
					c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
					if err := c.DeleteDnsZoneV1(context.Background(), zoneID); err != nil {
						t.Fatalf("drift preconfig: deleting DNS zone %s: %v", zoneID, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSZone_DuplicateNameServerIPRejectedAtPlan pins
// the plan-time uniqueness check. Jamf Security Cloud refuses a repeated name
// server IP even when the two entries name different gateways, so the config
// below must never reach apply.
func TestAccResource_SecurityCloudDNSZone_DuplicateNameServerIPRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 2)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "test" {
						name    = "tf-acc-jsc-dns-zone-dupe-%s"
						domains = ["tf-acc-dupe-%s.example.com"]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}
				`, suffix, suffix, testNameServerIPOne, gateways[0], testNameServerIPOne, gateways[1]),
				ExpectError: regexpDuplicateIP,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSZone_InvalidNameServerIPRejectedAtPlan pins the
// IPv4 check. Jamf Security Cloud collapses several distinct mistakes into one
// opaque INVALID_FIELD message, so the provider catches the shape at plan time.
func TestAccResource_SecurityCloudDNSZone_InvalidNameServerIPRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "test" {
						name    = "tf-acc-jsc-dns-zone-badip-%s"
						domains = ["tf-acc-badip-%s.example.com"]

						authoritative_name_servers = [
							{
								ip_address = "2001:db8::53"
								gateway_id = %q
							},
						]
					}
				`, suffix, suffix, gateways[0]),
				ExpectError: regexpInvalidIP,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSZone_UnknownGatewayPointsAtNameServers checks
// that a server-enforced dependency failure lands on the attribute that caused
// it. The gateway ID below is well-formed but does not exist, so the write fails
// with GATEWAY_NOT_FOUND — and the diagnostic must name the name servers rather
// than the zone.
func TestAccResource_SecurityCloudDNSZone_UnknownGatewayPointsAtNameServers(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "test" {
						name    = "tf-acc-jsc-dns-zone-nogw-%s"
						domains = ["tf-acc-nogw-%s.example.com"]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = "zzzz"
							},
						]
					}
				`, suffix, suffix, testNameServerIPOne),
				ExpectError: regexpGatewayNotFound,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDNSZone_ByIDAndName covers both lookup paths of
// the singular data source against one live zone.
func TestAccDataSource_SecurityCloudDNSZone_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-dns-zone-ds-" + suffix
	domain := "tf-acc-ds-" + suffix + ".example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "src" {
						name    = %q
						domains = [%q]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}

					data "jamfplatform_security_cloud_dns_zone" "by_id" {
						id = jamfplatform_security_cloud_dns_zone.src.id
					}

					data "jamfplatform_security_cloud_dns_zone" "by_name" {
						name       = jamfplatform_security_cloud_dns_zone.src.name
						depends_on = [jamfplatform_security_cloud_dns_zone.src]
					}
				`, name, domain, testNameServerIPOne, gateways[0]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_dns_zone.by_id", "name", "jamfplatform_security_cloud_dns_zone.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_dns_zone.by_id", "domains.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_dns_zone.by_id", "domains.0", domain),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_dns_zone.by_id", "authoritative_name_servers.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_dns_zone.by_id", "authoritative_name_servers.0.ip_address", testNameServerIPOne),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_dns_zone.by_id", "authoritative_name_servers.0.gateway_id", gateways[0]),
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_dns_zone.by_name", "id", "jamfplatform_security_cloud_dns_zone.src", "id"),
				),
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDNSZone_RequiresExactlyOneSelector pins the
// config validator on the singular data source, from both directions.
func TestAccDataSource_SecurityCloudDNSZone_RequiresExactlyOneSelector(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_dns_zone" "neither" {}
				`,
				ExpectError: regexpExactlyOneSelector,
			},
			{
				Config: `
					data "jamfplatform_security_cloud_dns_zone" "both" {
						id   = "f5734162-26d4-4d0f-a3ae-62f952b3688f"
						name = "Test Zone"
					}
				`,
				ExpectError: regexpExactlyOneSelector,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDNSZones_ListsCreatedZone checks the plural data
// source surfaces a zone created in the same apply. It asserts the created zone
// is present rather than asserting a total, because the tenant may hold zones
// this test did not create.
func TestAccDataSource_SecurityCloudDNSZones_ListsCreatedZone(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-dns-zones-" + suffix
	domain := "tf-acc-plural-" + suffix + ".example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "src" {
						name    = %q
						domains = [%q]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}

					data "jamfplatform_security_cloud_dns_zones" "all" {
						depends_on = [jamfplatform_security_cloud_dns_zone.src]
					}
				`, name, domain, testNameServerIPOne, gateways[0]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_dns_zones.all", "id", "dns_zones"),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_security_cloud_dns_zones.all", "dns_zones.*", map[string]string{
						"name": name,
					}),
				),
			},
		},
	})
}

// TestAccListResource_SecurityCloudDNSZone_Basic exercises the list resource via
// the `terraform query` workflow. The endpoint takes no filter, so step 2 asserts
// the created zone appears among the results rather than pinning a total.
//
// Requires Terraform 1.14+ (list resources).
func TestAccListResource_SecurityCloudDNSZone_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-dns-zone-list-" + suffix
	domain := "tf-acc-list-" + suffix + ".example.com"

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSZoneDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "src" {
						name    = %q
						domains = [%q]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}
				`, name, domain, testNameServerIPOne, gateways[0]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_dns_zone.src", "id"),
				),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_security_cloud_dns_zone" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_security_cloud_dns_zone.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("domains"), KnownValue: knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact(domain),
							})},
						},
					),
				},
			},
		},
	})
}

// captureDNSZoneID records the applied zone ID so a later step can act on the
// zone directly. PreConfig runs without access to state, so the ID has to be
// carried out of the apply step this way.
func captureDNSZoneID(address string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// Expected-error patterns for the plan- and apply-time refusals. Terraform wraps
// diagnostic text at roughly 80 columns, so each pattern matches a short phrase
// that cannot be split across a line break.
// TestAccDataSource_SecurityCloudDNSZone_EmptyIDRefused pins the guard for the
// selector mistake ExactlyOneOf cannot catch.
//
// A set-but-empty `id` counts as configured, so `id = var.x` with an unset variable
// satisfies the config validator and then falls through to whichever lookup is
// left. Without the guard the name branch runs with a null name, and the operator
// gets an SDK-internal string about an attribute they never set.
func TestAccDataSource_SecurityCloudDNSZone_EmptyIDRefused(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_dns_zone" "empty_id" {
						id = ""
					}
				`,
				ExpectError: regexpEmptyID,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDNSZone_NameNotFoundIsWritten pins that a name
// matching nothing produces a written diagnostic rather than the resolver's
// synthetic 404 body, which reads as an internal error and names no remedy.
func TestAccDataSource_SecurityCloudDNSZone_NameNotFoundIsWritten(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_dns_zone" "missing" {
						name = "tf-acc-no-such-dns_zone-zzz"
					}
				`,
				ExpectError: regexpNameNotFound,
			},
		},
	})
}

var (
	regexpDuplicateIP        = regexp.MustCompile(`Duplicate ip_address within set`)
	regexpInvalidIP          = regexp.MustCompile(`Invalid IPv4 address`)
	regexpGatewayNotFound    = regexp.MustCompile(`Referenced gateway not found`)
	regexpExactlyOneSelector = regexp.MustCompile(`Exactly one of these attributes must be configured`)
	regexpEmptyID            = regexp.MustCompile(`ID is empty`)
	regexpNameNotFound       = regexp.MustCompile(`on this tenant is named`)
)
