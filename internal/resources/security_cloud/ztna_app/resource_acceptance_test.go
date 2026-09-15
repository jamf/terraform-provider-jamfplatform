// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ztna_app_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// Acceptance host names sit under example.com and example.net, reserved by RFC 2606
// and never resolvable, so the tests exercise the real write path without claiming a
// name anyone operates. That matters more here than elsewhere: a host name belongs to
// only one application across the whole tenant, so a careless name would lock out
// every other test and every real application on the acceptance tenant.
//
// Address ranges come from RFC 5737's TEST-NET-1 for the same reason.
const (
	testHostnameSuffix    = ".tf-acc.example.com"
	testHostnameAltSuffix = ".tf-acc.example.net"
	testSubnet            = "192.0.2.0/24"
)

const resourceName = "jamfplatform_security_cloud_ztna_app.test"

// testAccCheckZtnaAppDestroy verifies applications created during the test were
// destroyed.
func testAccCheckZtnaAppDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_ztna_app" {
				continue
			}
			_, err := c.GetZtnaAppV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud access policy application %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Security Cloud access policy application %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_SecurityCloudZtnaApp_Custom covers the custom form end to end:
// create, an in-place update of every non-RequiresReplace attribute, and import.
//
// The update step changes everything at once — rename, re-categorise, swap both
// traffic-matching collections, switch from all device groups to two selected ones,
// switch routing from direct to via a gateway, add a per-group override, and adopt
// all three security cards. That is deliberate: the provider sends the whole object
// on every update, so a step changing one field would pass even if the patch dropped
// the rest.
func TestAccResource_SecurityCloudZtnaApp_Custom(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 2)
	categories := testhelpers.RequireSecurityCloudContentCategories(t)

	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-ztna-app-" + suffix
	nameUpdated := "tf-acc-jsc-ztna-app-updated-" + suffix
	host := suffix + testHostnameSuffix
	hostUpdated := suffix + testHostnameAltSuffix
	category := categories[0].DisplayName
	categoryUpdated := categories[1].DisplayName

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = %q
						category          = %q
						hostnames         = [%q]
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, name, category, host),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("app_type"), knownvalue.StringExact("Custom")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("category"), knownvalue.StringExact(category)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("predefined_app_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("all_device_groups"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("hostnames"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("traffic_routing"), knownvalue.StringExact("Direct device routing")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("gateway_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("routing_mode"), knownvalue.Null()),
					// An omitted collection reads back null rather than empty, which is the
					// reconciliation that keeps an application with no address ranges from
					// diffing against its own refresh.
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("direct_ips_and_subnets"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("device_group_ids"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing_overrides"), knownvalue.Null()),
					// The server holds all three security cards regardless; state must stay
					// absent until the configuration adopts them.
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security"), knownvalue.Null()),
				},
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "one" {
						name = "tf-acc-jsc-ztna-app-group-1-%[1]s"
					}

					resource "jamfplatform_security_cloud_device_group" "two" {
						name = "tf-acc-jsc-ztna-app-group-2-%[1]s"
					}

					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name                   = %[2]q
						category               = %[3]q
						hostnames              = [%[4]q]
						direct_ips_and_subnets = [%[5]q]
						all_device_groups      = false

						device_group_ids = [
							jamfplatform_security_cloud_device_group.one.id,
							jamfplatform_security_cloud_device_group.two.id,
						]

						routing = {
							traffic_routing = "Encrypt and route via ZTNA"
							gateway_id      = %[6]q
							routing_mode    = "Standard"
						}

						routing_overrides = [
							{
								device_group_ids = [jamfplatform_security_cloud_device_group.one.id]
								routing = {
									traffic_routing = "Direct device routing"
								}
							},
						]

						security = {
							managed_device = {
								enabled                   = true
								device_push_notifications = false
							}
							device_risk = {
								enabled                   = true
								deny_at_risk_level        = "Medium"
								device_push_notifications = true
							}
							jamf_trust = {
								enabled = true
							}
						}
					}
				`, suffix, nameUpdated, categoryUpdated, hostUpdated, testSubnet, gateways[0]),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(nameUpdated)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("category"), knownvalue.StringExact(categoryUpdated)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("all_device_groups"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("device_group_ids"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("direct_ips_and_subnets"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact(testSubnet),
					})),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("hostnames"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact(hostUpdated),
					})),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("traffic_routing"), knownvalue.StringExact("Encrypt and route via ZTNA")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("gateway_id"), knownvalue.StringExact(gateways[0])),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("routing_mode"), knownvalue.StringExact("Standard")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing_overrides"), knownvalue.ListSizeExact(1)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing_overrides").AtSliceIndex(0).AtMapKey("device_group_ids"), knownvalue.SetSizeExact(1)),
					// The override names one of the two groups this same config creates, so the
					// id it must hold is only knowable at apply time. Comparing it against the
					// group resource pins it anyway — a read that mangles or swaps the override
					// group ids passes a size-only assertion.
					statecheck.CompareValuePairs(
						"jamfplatform_security_cloud_device_group.one", tfjsonpath.New("id"),
						resourceName, tfjsonpath.New("routing_overrides").AtSliceIndex(0).AtMapKey("device_group_ids").AtSliceIndex(0),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing_overrides").AtSliceIndex(0).AtMapKey("routing").AtMapKey("traffic_routing"), knownvalue.StringExact("Direct device routing")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security").AtMapKey("managed_device").AtMapKey("enabled"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security").AtMapKey("managed_device").AtMapKey("device_push_notifications"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security").AtMapKey("device_risk").AtMapKey("deny_at_risk_level"), knownvalue.StringExact("Medium")),
					// jamf_trust declared only `enabled`, so the notification leaf must come
					// from its schema default rather than reading back unknown.
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security").AtMapKey("jamf_trust").AtMapKey("enabled"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security").AtMapKey("jamf_trust").AtMapKey("device_push_notifications"), knownvalue.Bool(true)),
					// Adopting only two of three cards must leave the third absent.
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security").AtMapKey("device_risk").AtMapKey("enabled"), knownvalue.Bool(true)),
				},
			},
			{
				// Back to direct routing and all device groups, dropping every collection.
				// This is the step that proves the routing transition works in the harder
				// direction: merge patch has to null the gateway and routing mode the previous
				// step left behind, and an emptied collection has to clear rather than linger.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = %q
						category          = %q
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, nameUpdated, categoryUpdated),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("gateway_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing").AtMapKey("routing_mode"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("hostnames"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("direct_ips_and_subnets"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("device_group_ids"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("routing_overrides"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("security"), knownvalue.Null()),
				},
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				// timeouts is provider-side configuration with no server representation.
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// requireUnusedPredefinedAppID returns a predefined application definition the tenant
// does not already have an application for.
//
// Taking the first entry of the catalogue is not safe: Jamf Security Cloud allows only
// one application per definition, and a tenant that already has one for the first entry
// would fail every predefined test with `409 CONFLICT "Resource already exists."` —
// which is a fixture collision, not a provider bug. The acceptance tenant this was
// written against already had an application for the catalogue's first entry, so this
// is an observed hazard rather than a hypothetical one.
//
// A tenant with an application for every definition SKIPS rather than fails: absent
// fixture headroom is a property of the environment, not a defect in the resource.
func requireUnusedPredefinedAppID(t *testing.T) string {
	t.Helper()

	catalogue := testhelpers.RequireSecurityCloudPredefinedApps(t)
	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))

	apps, err := c.ListZtnaAppsV1(context.Background())
	if err != nil {
		t.Fatalf("listing access policy applications to find an unused predefined definition: %s", err)
	}

	used := map[string]struct{}{}
	for _, app := range apps {
		if app.PredefinedAppID != nil {
			used[*app.PredefinedAppID] = struct{}{}
		}
	}
	for _, definition := range catalogue {
		if _, taken := used[definition.ID]; !taken {
			return definition.ID
		}
	}

	t.Skipf("Skipping: every one of the %d predefined application definitions already has an application on this tenant, so there is none free to create", len(catalogue))
	return ""
}

// TestAccResource_SecurityCloudZtnaApp_Predefined covers the other mutually exclusive
// shape. A predefined application's name is owned by the Jamf-maintained definition
// and reads back null, and the definition's own host names never appear in
// `hostnames` — only the additions do.
func TestAccResource_SecurityCloudZtnaApp_Predefined(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	predefinedID := requireUnusedPredefinedAppID(t)

	suffix := testhelpers.RunSuffix()
	host := "extra-" + suffix + testHostnameSuffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						predefined_app_id = %q
						category          = "Uncategorized"
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, predefinedID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("app_type"), knownvalue.StringExact("Predefined")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("predefined_app_id"), knownvalue.StringExact(predefinedID)),
					// The definition contributes host names of its own, and they must not
					// appear here — otherwise the configuration would diff against them.
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("hostnames"), knownvalue.Null()),
				},
			},
			{
				// Host names added to a predefined application extend the definition's set
				// rather than replacing it, so exactly one entry must read back.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						predefined_app_id = %q
						category          = "Uncategorized"
						hostnames         = [%q]
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, predefinedID, host),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("hostnames"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact(host),
					})),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.Null()),
				},
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaApp_FormIsImmutable pins that changing the form
// replaces rather than updates. The patch body carries no predefinedAppId field at
// all, so an in-place attempt would be a write that silently does nothing.
func TestAccResource_SecurityCloudZtnaApp_FormIsImmutable(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	predefinedID := requireUnusedPredefinedAppID(t)

	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-ztna-app-form-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = %q
						category          = "Uncategorized"
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, name),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						predefined_app_id = %q
						category          = "Uncategorized"
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, predefinedID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("app_type"), knownvalue.StringExact("Predefined")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.Null()),
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaApp_Drift covers recovery from an application
// deleted outside Terraform. Read has to remove the resource from state rather than
// erroring, or a deleted application would wedge every subsequent plan.
//
// The refresh step carries no Config of its own: a TestStep cannot set both Config
// and RefreshState, so it reuses the preceding step's.
func TestAccResource_SecurityCloudZtnaApp_Drift(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-ztna-app-drift-" + suffix
	var appID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = %q
						category          = "Uncategorized"
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, name),
				Check: captureZtnaAppID(resourceName, &appID),
			},
			{
				PreConfig: func() {
					c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
					if err := c.DeleteZtnaAppV1(context.Background(), appID); err != nil {
						t.Fatalf("deleting access policy application out of band: %s", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudZtnaApp_ValidatorErrors covers every declared
// cross-field validator. Each of these is refused by Jamf Security Cloud with a
// message that names too little to act on — or, for the predefined-name case, is not
// refused at all — which is why they are enforced at plan time.
//
// The regexes anchor on tokens that carry no internal whitespace, because Terraform
// wraps error output at around 80 columns and a match spanning the wrap point would
// be flaky.
func TestAccResource_SecurityCloudZtnaApp_ValidatorErrors(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	predefinedID := requireUnusedPredefinedAppID(t)

	suffix := testhelpers.RunSuffix()

	cases := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "name alongside a predefined app",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-both-%s"
					predefined_app_id = %q
					category          = "Uncategorized"
					all_device_groups = true
					routing = { traffic_routing = "Direct device routing" }
				}
			`, suffix, predefinedID),
			expectError: regexp.MustCompile(`cannot be renamed`),
		},
		{
			name: "neither name nor predefined app",
			config: `
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					category          = "Uncategorized"
					all_device_groups = true
					routing = { traffic_routing = "Direct device routing" }
				}
			`,
			expectError: regexp.MustCompile(`needs\s+a\s+name`),
		},
		{
			name: "routing via ztna without a gateway",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-nogw-%s"
					category          = "Uncategorized"
					all_device_groups = true
					routing = {
						traffic_routing = "Encrypt and route via ZTNA"
						routing_mode    = "Standard"
					}
				}
			`, suffix),
			expectError: regexp.MustCompile(`access\s+gateway`),
		},
		{
			name: "routing via ztna without a routing mode",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-nomode-%s"
					category          = "Uncategorized"
					all_device_groups = true
					routing = {
						traffic_routing = "Encrypt and route via ZTNA"
						gateway_id      = %q
					}
				}
			`, suffix, gateways[0]),
			expectError: regexp.MustCompile(`routing\s+mode`),
		},
		{
			name: "direct routing with a gateway",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-directgw-%s"
					category          = "Uncategorized"
					all_device_groups = true
					routing = {
						traffic_routing = "Direct device routing"
						gateway_id      = %q
					}
				}
			`, suffix, gateways[0]),
			expectError: regexp.MustCompile(`does\s+not\s+use\s+an\s+access\s+gateway`),
		},
		{
			name: "device groups alongside all device groups",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-bothgroups-%s"
					category          = "Uncategorized"
					all_device_groups = true
					device_group_ids  = ["00000000-0000-0000-0000-000000000000"]
					routing = { traffic_routing = "Direct device routing" }
				}
			`, suffix),
			expectError: regexp.MustCompile(`conflict\s+with\s+all\s+device\s+groups`),
		},
		{
			name: "override on an unassigned device group",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-unassigned-%s"
					category          = "Uncategorized"
					all_device_groups = false
					device_group_ids  = ["aaaaaaaa-0000-0000-0000-000000000000"]
					routing = { traffic_routing = "Direct device routing" }
					routing_overrides = [
						{
							device_group_ids = ["bbbbbbbb-0000-0000-0000-000000000000"]
							routing = { traffic_routing = "Direct device routing" }
						},
					]
				}
			`, suffix),
			expectError: regexp.MustCompile(`unassigned\s+device\s+group`),
		},
		{
			name: "one device group in two overrides",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-dupoverride-%s"
					category          = "Uncategorized"
					all_device_groups = true
					routing = { traffic_routing = "Direct device routing" }
					routing_overrides = [
						{
							device_group_ids = ["aaaaaaaa-0000-0000-0000-000000000000"]
							routing = { traffic_routing = "Direct device routing" }
						},
						{
							device_group_ids = ["aaaaaaaa-0000-0000-0000-000000000000"]
							routing = { traffic_routing = "Direct device routing" }
						},
					]
				}
			`, suffix),
			expectError: regexp.MustCompile(`more\s+than\s+one\s+routing\s+override`),
		},
		{
			name: "overlapping host names",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-overlap-%s"
					category          = "Uncategorized"
					all_device_groups = true
					hostnames         = ["*.tf-acc.example.com", "sub.tf-acc.example.com"]
					routing = { traffic_routing = "Direct device routing" }
				}
			`, suffix),
			expectError: regexp.MustCompile(`Host\s+names\s+overlap`),
		},
		{
			name: "host name with upper case",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-upper-%s"
					category          = "Uncategorized"
					all_device_groups = true
					hostnames         = ["UPPER.tf-acc.example.com"]
					routing = { traffic_routing = "Direct device routing" }
				}
			`, suffix),
			expectError: regexp.MustCompile(`lower-case`),
		},
		{
			name: "host name with a trailing dot",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-dot-%s"
					category          = "Uncategorized"
					all_device_groups = true
					hostnames         = ["trailing.tf-acc.example.com."]
					routing = { traffic_routing = "Direct device routing" }
				}
			`, suffix),
			expectError: regexp.MustCompile(`trailing\s+dot`),
		},
		{
			name: "bare address instead of a CIDR range",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name                   = "tf-acc-bareip-%s"
					category               = "Uncategorized"
					all_device_groups      = true
					direct_ips_and_subnets = ["192.0.2.10"]
					routing = { traffic_routing = "Direct device routing" }
				}
			`, suffix),
			expectError: regexp.MustCompile(`prefix\s+length`),
		},
		{
			name: "invalid routing mode value",
			config: fmt.Sprintf(`
				resource "jamfplatform_security_cloud_ztna_app" "test" {
					name              = "tf-acc-badmode-%s"
					category          = "Uncategorized"
					all_device_groups = true
					routing = { traffic_routing = "CUSTOM" }
				}
			`, suffix),
			expectError: regexp.MustCompile(`Invalid\s+Attribute\s+Value\s+Match`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						ExpectError: tc.expectError,
					},
				},
			})
		})
	}
}

// TestAccResource_SecurityCloudZtnaApp_ServerConflicts covers the tenant-wide
// constraints Terraform cannot see in the plan, so each one is a translated
// diagnostic rather than a plan-time check.
func TestAccResource_SecurityCloudZtnaApp_ServerConflicts(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	predefinedID := requireUnusedPredefinedAppID(t)

	suffix := testhelpers.RunSuffix()
	host := "conflict-" + suffix + testHostnameSuffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = "tf-acc-jsc-ztna-conflict-a-%[1]s"
						category          = "Uncategorized"
						hostnames         = [%[2]q]
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}
				`, suffix, host),
			},
			{
				// A second application claiming the same host name is refused tenant-wide.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = "tf-acc-jsc-ztna-conflict-a-%[1]s"
						category          = "Uncategorized"
						hostnames         = [%[2]q]
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}

					resource "jamfplatform_security_cloud_ztna_app" "clash" {
						name              = "tf-acc-jsc-ztna-conflict-b-%[1]s"
						category          = "Uncategorized"
						hostnames         = [%[2]q]
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}
				`, suffix, host),
				ExpectError: regexp.MustCompile(`already\s+claimed\s+by\s+another\s+access\s+policy`),
			},
			{
				// An unknown category is a state conflict rather than a malformed request,
				// because the category list is server-owned and can change.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = "tf-acc-jsc-ztna-conflict-a-%[1]s"
						category          = "tf-acc-not-a-category"
						hostnames         = [%[2]q]
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}
				`, suffix, host),
				ExpectError: regexp.MustCompile(`Unknown\s+application\s+category`),
			},
			{
				// A device group that does not exist is likewise only detectable server-side.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = "tf-acc-jsc-ztna-conflict-a-%[1]s"
						category          = "Uncategorized"
						hostnames         = [%[2]q]
						all_device_groups = false
						device_group_ids  = ["00000000-0000-0000-0000-000000000000"]
						routing = { traffic_routing = "Direct device routing" }
					}
				`, suffix, host),
				ExpectError: regexp.MustCompile(`Referenced\s+device\s+group\s+not\s+found`),
			},
			{
				// An unknown definition ID is refused as a conflict, not a bad request.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = "tf-acc-jsc-ztna-conflict-a-%[1]s"
						category          = "Uncategorized"
						hostnames         = [%[2]q]
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}

					resource "jamfplatform_security_cloud_ztna_app" "bogus" {
						predefined_app_id = "00000000-0000-0000-0000-000000000000"
						category          = "Uncategorized"
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}
				`, suffix, host),
				ExpectError: regexp.MustCompile(`Predefined\s+application\s+not\s+found`),
			},
			{
				// Only one application per definition is allowed on a tenant, and the server
				// reports that with a bare generic conflict naming nothing.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = "tf-acc-jsc-ztna-conflict-a-%[1]s"
						category          = "Uncategorized"
						hostnames         = [%[2]q]
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}

					resource "jamfplatform_security_cloud_ztna_app" "first" {
						predefined_app_id = %[3]q
						category          = "Uncategorized"
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}

					resource "jamfplatform_security_cloud_ztna_app" "second" {
						predefined_app_id = %[3]q
						category          = "Uncategorized"
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
						depends_on        = [jamfplatform_security_cloud_ztna_app.first]
					}
				`, suffix, host, predefinedID),
				ExpectError: regexp.MustCompile(`Predefined\s+application\s+already\s+in\s+use`),
			},
		},
	})
}

// listContainsObjectWithString is a knownvalue.Check that passes when a list of
// objects holds at least one element whose string attribute key equals want.
//
// It exists because every list check terraform-plugin-testing v1.16.0 ships is
// positional — ListExact, ListPartial and ListSizeExact all address elements by
// index — and the plural application list arrives in whatever order Jamf Security
// Cloud returns it in, which the schema explicitly says not to rely on. An empty or
// truncated list therefore fails here rather than passing on a lucky index.
//
// A missing or null attribute compares as the empty string, so want must be
// non-empty for the check to mean anything; the callers pass literals.
type listContainsObjectWithString struct {
	key  string
	want string
}

var _ knownvalue.Check = listContainsObjectWithString{}

// CheckValue reports whether any element carries the wanted attribute value.
func (c listContainsObjectWithString) CheckValue(other any) error {
	elements, ok := other.([]any)
	if !ok {
		return fmt.Errorf("expected []any value for listContainsObjectWithString check, got: %T", other)
	}

	got := make([]string, 0, len(elements))
	for i, element := range elements {
		object, ok := element.(map[string]any)
		if !ok {
			return fmt.Errorf("expected map[string]any for element %d of listContainsObjectWithString check, got: %T", i, element)
		}
		value, _ := object[c.key].(string)
		if value == c.want {
			return nil
		}
		got = append(got, value)
	}

	return fmt.Errorf("no element with %s %q among the %d elements checked (saw: %v)", c.key, c.want, len(elements), got)
}

// String returns the string representation of the check.
func (c listContainsObjectWithString) String() string {
	return fmt.Sprintf("an element with %s %q", c.key, c.want)
}

// TestAccDataSource_SecurityCloudZtnaApp covers all three lookup keys on the singular
// data source, plus the plural one.
//
// The predefined_app_id key exists because a predefined application has no name at
// all, so `name` cannot address one — a fact worth an assertion rather than a comment.
func TestAccDataSource_SecurityCloudZtnaApp(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	predefinedID := requireUnusedPredefinedAppID(t)

	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-ztna-app-ds-" + suffix
	host := "ds-" + suffix + testHostnameSuffix

	config := fmt.Sprintf(`
		resource "jamfplatform_security_cloud_ztna_app" "test" {
			name              = %[1]q
			category          = "Uncategorized"
			hostnames         = [%[2]q]
			all_device_groups = true
			routing = { traffic_routing = "Direct device routing" }
		}

		resource "jamfplatform_security_cloud_ztna_app" "predefined" {
			predefined_app_id = %[3]q
			category          = "Uncategorized"
			all_device_groups = true
			routing = { traffic_routing = "Direct device routing" }
		}

		data "jamfplatform_security_cloud_ztna_app" "by_id" {
			id = jamfplatform_security_cloud_ztna_app.test.id
		}

		data "jamfplatform_security_cloud_ztna_app" "by_name" {
			name       = %[1]q
			depends_on = [jamfplatform_security_cloud_ztna_app.test]
		}

		data "jamfplatform_security_cloud_ztna_app" "by_predefined" {
			predefined_app_id = %[3]q
			depends_on        = [jamfplatform_security_cloud_ztna_app.predefined]
		}

		data "jamfplatform_security_cloud_ztna_apps" "all" {
			depends_on = [
				jamfplatform_security_cloud_ztna_app.test,
				jamfplatform_security_cloud_ztna_app.predefined,
			]
		}
	`, name, host, predefinedID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_app.by_id", tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_app.by_id", tfjsonpath.New("app_type"), knownvalue.StringExact("Custom")),
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_app.by_name", tfjsonpath.New("hostnames"), knownvalue.ListExact([]knownvalue.Check{
						knownvalue.StringExact(host),
					})),
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_app.by_predefined", tfjsonpath.New("app_type"), knownvalue.StringExact("Predefined")),
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_app.by_predefined", tfjsonpath.New("name"), knownvalue.Null()),
					// The data source always reports all three security cards, unlike the
					// resource, because it has no configuration to gate on.
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_app.by_id", tfjsonpath.New("security").AtMapKey("jamf_trust").AtMapKey("enabled"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_apps.all", tfjsonpath.New("id"), knownvalue.StringExact("ztna_apps")),
					// The plural id is a compile-time constant, so on its own it asserts nothing
					// about the read. These three cover the result list itself.
					//
					// The size floor is two — the custom and predefined applications this config
					// creates — and a floor rather than an exact count because the tenant holds
					// whatever else other tests and real configuration have left behind. There is
					// no ListSizeAtLeast in terraform-plugin-testing v1.16.0; ListPartial keyed on
					// the last required index is the same assertion, since it fails with "missing
					// element index" on a shorter list.
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_apps.all", tfjsonpath.New("ztna_apps"), knownvalue.ListPartial(map[int]knownvalue.Check{
						1: knownvalue.NotNull(),
					})),
					// Both created applications must actually appear. Matching by field rather
					// than by index is deliberate: the endpoint takes no sort parameter and the
					// schema says the order is the server's, so an index would be a coin toss.
					// The predefined one is matched on predefined_app_id because its name is null.
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_apps.all", tfjsonpath.New("ztna_apps"), listContainsObjectWithString{key: "name", want: name}),
					statecheck.ExpectKnownValue("data.jamfplatform_security_cloud_ztna_apps.all", tfjsonpath.New("ztna_apps"), listContainsObjectWithString{key: "predefined_app_id", want: predefinedID}),
				},
			},
		},
	})
}

// TestAccDataSource_SecurityCloudZtnaApp_LookupErrors covers the lookup failures.
//
// The ExactlyOneOf cases anchor on the shared detail rather than the summary, because
// the framework emits "Invalid Attribute Combination" when more than one key is set
// and "Missing Attribute Configuration" when none is — a regex matching one summary
// would pass on the other for the wrong reason.
func TestAccDataSource_SecurityCloudZtnaApp_LookupErrors(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	cases := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "no lookup key",
			config: `
				data "jamfplatform_security_cloud_ztna_app" "test" {}
			`,
			expectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
		},
		{
			name: "two lookup keys",
			config: `
				data "jamfplatform_security_cloud_ztna_app" "test" {
					id   = "00000000-0000-0000-0000-000000000000"
					name = "anything"
				}
			`,
			expectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
		},
		{
			name: "unknown id",
			config: `
				data "jamfplatform_security_cloud_ztna_app" "test" {
					id = "00000000-0000-0000-0000-000000000000"
				}
			`,
			expectError: regexp.MustCompile(`Unable\s+to\s+find`),
		},
		{
			name: "unknown name",
			config: `
				data "jamfplatform_security_cloud_ztna_app" "test" {
					name = "tf-acc-no-such-application"
				}
			`,
			expectError: regexp.MustCompile(`Unable\s+to\s+find`),
		},
		{
			name: "empty lookup key",
			config: `
				data "jamfplatform_security_cloud_ztna_app" "test" {
					id = ""
				}
			`,
			expectError: regexp.MustCompile(`Lookup\s+key\s+is\s+empty`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						ExpectError: tc.expectError,
					},
				},
			})
		})
	}
}

// TestAccListResource_SecurityCloudZtnaApp covers the list resource, which streams
// identities for `terraform query`.
func TestAccListResource_SecurityCloudZtnaApp(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-ztna-app-list-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckZtnaAppDestroy(t),
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = %q
						category          = "Uncategorized"
						all_device_groups = true
						routing = { traffic_routing = "Direct device routing" }
					}
				`, name),
			},
			{
				// A query step's config is a .tfquery.hcl file, which accepts `provider` and
				// `list` blocks only — repeating the resource here fails with "Blocks of type
				// resource are not expected here". The application created by the preceding
				// step is still on the tenant, which is what the checks below match against.
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_security_cloud_ztna_app" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLengthAtLeast("jamfplatform_security_cloud_ztna_app.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_security_cloud_ztna_app.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("app_type"), KnownValue: knownvalue.StringExact("Custom")},
							// A listed application arrives with security unset, for the same
							// reason an imported one does: the cards are Optional-only and the
							// state builder fills only what the target already declares.
							{Path: tfjsonpath.New("security"), KnownValue: knownvalue.Null()},
						},
					),
				},
			},
		},
	})
}

// captureZtnaAppID records the created application's ID for out-of-band manipulation.
func captureZtnaAppID(resourceAddr string, target *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceAddr)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("resource %s has no ID", resourceAddr)
		}
		*target = rs.Primary.ID
		return nil
	}
}
