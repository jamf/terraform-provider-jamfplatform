// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ztna_grouped_gateway_test

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

// testAccCheckGroupedGatewayDestroy verifies grouped gateways created during the
// test were destroyed. It does not check the member gateways: those are separate
// resources in the same config and carry their own destroy check in the gateway
// package.
func testAccCheckGroupedGatewayDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_ztna_grouped_gateway" {
				continue
			}
			_, err := c.GetZtnaGroupedGatewayV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud ZTNA grouped gateway %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Security Cloud ZTNA grouped gateway %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_SecurityCloudZtnaGroupedGateway_Basic covers create, an in-place
// update of every writable field, and import.
//
// The update step reverses the member order as well as changing the strategy and
// the stability window, because order is the priority order for the
// first-available strategy: a patch that normalised or sorted the list would pass
// a test that only ever wrote it in one order.
func TestAccResource_SecurityCloudZtnaGroupedGateway_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gg-" + suffix
	nameUpdated := "tf-acc-jsc-gg-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupedGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: memberGatewaysConfig(suffix, tenantID) + fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
						name                   = %q
						routing_strategy           = "First available"
						required_gateway_stability = "30 minutes"

						gateway_ids = [
							jamfplatform_security_cloud_ztna_gateway.a.id,
							jamfplatform_security_cloud_ztna_gateway.b.id,
						]

						tenant_ids = [%q]
					}
				`, name, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_grouped_gateway.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "routing_strategy", "First available"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "required_gateway_stability", "30 minutes"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "gateway_ids.#", "2"),
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_grouped_gateway.test", "created_at"),
					resource.TestCheckResourceAttrPair(
						"jamfplatform_security_cloud_ztna_grouped_gateway.test", "gateway_ids.0",
						"jamfplatform_security_cloud_ztna_gateway.a", "id",
					),
				),
			},
			{
				Config: memberGatewaysConfig(suffix, tenantID) + fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
						name                   = %q
						routing_strategy           = "Nearest"
						required_gateway_stability = "1 hour"

						gateway_ids = [
							jamfplatform_security_cloud_ztna_gateway.b.id,
							jamfplatform_security_cloud_ztna_gateway.a.id,
						]

						tenant_ids = [%q]
					}
				`, nameUpdated, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "routing_strategy", "Nearest"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_ztna_grouped_gateway.test", "required_gateway_stability", "1 hour"),
					resource.TestCheckResourceAttrPair(
						"jamfplatform_security_cloud_ztna_grouped_gateway.test", "gateway_ids.0",
						"jamfplatform_security_cloud_ztna_gateway.b", "id",
					),
					resource.TestCheckResourceAttrPair(
						"jamfplatform_security_cloud_ztna_grouped_gateway.test", "gateway_ids.1",
						"jamfplatform_security_cloud_ztna_gateway.a", "id",
					),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_ztna_grouped_gateway.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGroupedGateway_MixedFormsRefused pins the
// membership rule that only the server can enforce. Grouping an IPsec gateway with
// an internet one is refused, and the diagnostic has to name the member list
// rather than the group.
func TestAccResource_SecurityCloudZtnaGroupedGateway_MixedFormsRefused(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupedGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mixedFormGatewaysConfig(suffix, tenantID) + fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
						name                   = "tf-acc-jsc-gg-mixed-%s"
						routing_strategy           = "Nearest"
						required_gateway_stability = "30 minutes"

						gateway_ids = [
							jamfplatform_security_cloud_ztna_gateway.internet.id,
							jamfplatform_security_cloud_ztna_gateway.ipsec.id,
						]

						tenant_ids = [%q]
					}
				`, suffix, tenantID),
				ExpectError: regexpMixedForms,
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaGroupedGateway_InvalidGatewayStabilityRejectedAtPlan
// pins the one real enum in this resource. The value is required even for the two
// strategies that ignore it, and the Go zero value — what an omitted integer would
// send — is refused.
func TestAccResource_SecurityCloudZtnaGroupedGateway_InvalidGatewayStabilityRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
						name                   = "tf-acc-jsc-gg-baddelay-%s"
						routing_strategy           = "Nearest"
						required_gateway_stability = "15 minutes"
						gateway_ids            = ["a1b2", "c3d4"]
						tenant_ids             = [%q]
					}
				`, suffix, tenantID),
				ExpectError: regexpInvalidAttributeValue,
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
						name                   = "tf-acc-jsc-gg-onemember-%s"
						routing_strategy           = "Nearest"
						required_gateway_stability = "30 minutes"
						gateway_ids            = ["a1b2"]
						tenant_ids             = [%q]
					}
				`, suffix, tenantID),
				ExpectError: regexpTooFewMembers,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGroupedGateway_ByIDAndName covers both lookup
// paths of the singular data source against one live group.
func TestAccDataSource_SecurityCloudZtnaGroupedGateway_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gg-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupedGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: memberGatewaysConfig(suffix, tenantID) + fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "src" {
						name                   = %q
						routing_strategy           = "Random"
						required_gateway_stability = "5 minutes"

						gateway_ids = [
							jamfplatform_security_cloud_ztna_gateway.a.id,
							jamfplatform_security_cloud_ztna_gateway.b.id,
						]

						tenant_ids = [%q]
					}

					data "jamfplatform_security_cloud_ztna_grouped_gateway" "by_id" {
						id = jamfplatform_security_cloud_ztna_grouped_gateway.src.id
					}

					data "jamfplatform_security_cloud_ztna_grouped_gateway" "by_name" {
						name       = jamfplatform_security_cloud_ztna_grouped_gateway.src.name
						depends_on = [jamfplatform_security_cloud_ztna_grouped_gateway.src]
					}
				`, name, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_grouped_gateway.by_id", "routing_strategy", "Random"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_grouped_gateway.by_id", "required_gateway_stability", "5 minutes"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_grouped_gateway.by_id", "gateway_ids.#", "2"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_ztna_grouped_gateway.by_name", "id", "jamfplatform_security_cloud_ztna_grouped_gateway.src", "id"),
				),
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGroupedGateways_ListsCreatedGroup checks the
// plural data source surfaces a group created in the same apply.
func TestAccDataSource_SecurityCloudZtnaGroupedGateways_ListsCreatedGroup(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-ggs-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupedGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: memberGatewaysConfig(suffix, tenantID) + fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "src" {
						name                   = %q
						routing_strategy           = "Nearest"
						required_gateway_stability = "30 minutes"

						gateway_ids = [
							jamfplatform_security_cloud_ztna_gateway.a.id,
							jamfplatform_security_cloud_ztna_gateway.b.id,
						]

						tenant_ids = [%q]
					}

					data "jamfplatform_security_cloud_ztna_grouped_gateways" "all" {
						depends_on = [jamfplatform_security_cloud_ztna_grouped_gateway.src]
					}
				`, name, tenantID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_ztna_grouped_gateways.all", "id", "ztna_grouped_gateways"),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_security_cloud_ztna_grouped_gateways.all", "grouped_gateways.*", map[string]string{
						"name": name,
					}),
				),
			},
		},
	})
}

// TestAccListResource_SecurityCloudZtnaGroupedGateway_Basic exercises the list
// resource via the `terraform query` workflow.
//
// Requires Terraform 1.14+ (list resources).
func TestAccListResource_SecurityCloudZtnaGroupedGateway_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	tenantID := testhelpers.RequireSecurityCloudTenantID(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-gg-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupedGatewayDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: memberGatewaysConfig(suffix, tenantID) + fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_grouped_gateway" "src" {
						name                   = %q
						routing_strategy           = "Nearest"
						required_gateway_stability = "30 minutes"

						gateway_ids = [
							jamfplatform_security_cloud_ztna_gateway.a.id,
							jamfplatform_security_cloud_ztna_gateway.b.id,
						]

						tenant_ids = [%q]
					}
				`, name, tenantID),
				Check: resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_ztna_grouped_gateway.src", "id"),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_security_cloud_ztna_grouped_gateway" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_security_cloud_ztna_grouped_gateway.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("routing_strategy"), KnownValue: knownvalue.StringExact("Nearest")},
						},
					),
				},
			},
		},
	})
}

// memberGatewaysConfig renders two dedicated internet gateways to group. They are
// the internet form rather than IPsec because a group needs its members to share a
// form and internet gateways need no tunnel configuration, no distinct private
// subnet per gateway, and no pre-shared key.
func memberGatewaysConfig(suffix, tenantID string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_security_cloud_ztna_gateway" "a" {
			name       = "tf-acc-jsc-gg-member-a-%s"
			egress_region = "Europe - UK"
			tenant_ids = [%q]

			contact = {
				name  = "Terraform Acceptance"
				email = "tf-acc@example.com"
			}
		}

		resource "jamfplatform_security_cloud_ztna_gateway" "b" {
			name       = "tf-acc-jsc-gg-member-b-%s"
			egress_region = "Europe - Germany"
			tenant_ids = [%q]

			contact = {
				name  = "Terraform Acceptance"
				email = "tf-acc@example.com"
			}
		}
	`, suffix, tenantID, suffix, tenantID)
}

// mixedFormGatewaysConfig renders one gateway of each form, so a group over both is
// refused for the reason the test is about.
func mixedFormGatewaysConfig(suffix, tenantID string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_security_cloud_ztna_gateway" "internet" {
			name       = "tf-acc-jsc-gg-inet-%s"
			egress_region = "Europe - UK"
			tenant_ids = [%q]

			contact = {
				name  = "Terraform Acceptance"
				email = "tf-acc@example.com"
			}
		}

		resource "jamfplatform_security_cloud_ztna_gateway" "ipsec" {
			name       = "tf-acc-jsc-gg-ipsec-%s"
			egress_region = "Europe - Germany"
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
					subnet                           = "10.60.0.0/16"
					authentication_secret            = "tf-acc-secret-mixed"
					authentication_secret_wo_version = 1
				}

				customer_side = {
					host          = "198.51.100.9"
					ike_domain_id = "peer.tf-acc.example.com"
					subnets       = ["0.0.0.0/0"]
					vendor        = "strongSwan"
				}
			}
		}
	`, suffix, tenantID, suffix, tenantID)
}

// Expected-error patterns for the plan- and apply-time refusals. Terraform wraps
// diagnostic text at roughly 80 columns, so each pattern matches a short phrase
// that cannot be split across a line break.
// TestAccDataSource_SecurityCloudZtnaGroupedGateway_EmptyIDRefused pins the guard for the
// selector mistake ExactlyOneOf cannot catch.
//
// A set-but-empty `id` counts as configured, so `id = var.x` with an unset variable
// satisfies the config validator and then falls through to whichever lookup is
// left. Without the guard the name branch runs with a null name, and the operator
// gets an SDK-internal string about an attribute they never set.
func TestAccDataSource_SecurityCloudZtnaGroupedGateway_EmptyIDRefused(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_ztna_grouped_gateway" "empty_id" {
						id = ""
					}
				`,
				ExpectError: regexpEmptyID,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaGroupedGateway_NameNotFoundIsWritten pins that a name
// matching nothing produces a written diagnostic rather than the resolver's
// synthetic 404 body, which reads as an internal error and names no remedy.
func TestAccDataSource_SecurityCloudZtnaGroupedGateway_NameNotFoundIsWritten(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_ztna_grouped_gateway" "missing" {
						name = "tf-acc-no-such-ztna_grouped_gateway-zzz"
					}
				`,
				ExpectError: regexpNameNotFound,
			},
		},
	})
}

var (
	regexpMixedForms            = regexp.MustCompile(`Member gateways have different forms`)
	regexpInvalidAttributeValue = regexp.MustCompile(`Invalid Attribute Value Match`)
	regexpTooFewMembers         = regexp.MustCompile(`Invalid Attribute Value`)
	regexpEmptyID               = regexp.MustCompile(`ID is empty`)
	regexpNameNotFound          = regexp.MustCompile(`on this tenant is named`)
)
