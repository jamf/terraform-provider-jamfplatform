// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package dns_hostname_mappings_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckHostnameMappingsDestroy asserts the mapping set really is empty.
//
// This is the ordinary CheckDestroy contract, not the inverted one
// STYLE_GUIDE §Singleton resources prescribes, because this singleton's Delete is a
// real clear. Asserting "still exists" — the Pro singleton shape — would pass while
// the provider silently cleared nothing.
func testAccCheckHostnameMappingsDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_dns_hostname_mappings" {
				continue
			}
			got, err := c.GetDnsCustomHostnameMappingsV1(ctx)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud hostname mappings: %s", err)
			}
			if len(got.Results) > 0 {
				return fmt.Errorf("Jamf Security Cloud still holds %d hostname mapping(s)", len(got.Results))
			}
		}
		return nil
	}
}

// requireEmptyHostnameMappings skips unless the tenant already holds no mappings.
//
// It does not clear them, and that is the whole point. These mappings drive internal
// name resolution for every enrolled device on the tenant, the endpoint is a full
// replace with no per-mapping route, and nothing here could put back a set it
// overwrote. So a populated tenant is a precondition these tests do not meet, not
// state to wipe on the way past — the same judgement the resource itself makes when
// Create refuses to clobber an existing set.
//
// A read that fails skips too: absence cannot be inferred from a failed read, and
// guessing "probably empty" is how the destructive version got written.
func requireEmptyHostnameMappings(t *testing.T) {
	t.Helper()
	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	got, err := c.GetDnsCustomHostnameMappingsV1(context.Background())
	if err != nil {
		t.Skipf("could not read the tenant's hostname mappings, so cannot tell whether this test would destroy them: %v", err)
	}
	if got != nil && len(got.Results) > 0 {
		t.Skipf("this test would destroy the tenant's %d existing hostname mapping(s); remove them by hand first if they are disposable", len(got.Results))
	}
}

// TestAccResource_SecurityCloudDNSHostnameMappings_Lifecycle covers create, in-place
// replacement of the whole collection, and destroy.
//
// The second step is the one worth having. The endpoint is a full replace with no
// per-mapping route, so adding one mapping and removing another has to land as a
// single update: a RequiresReplace creeping onto `mappings` would show up here as a
// destroy-and-create, which on a tenant-wide collection means a window with no
// mappings at all.
func TestAccResource_SecurityCloudDNSHostnameMappings_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireEmptyHostnameMappings(t)
	suffix := testhelpers.RunSuffix()

	first := "tf-acc-one-" + suffix + ".example.com"
	second := "tf-acc-two-" + suffix + ".example.com"
	third := "tf-acc-three-" + suffix + ".example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostnameMappingsDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							{
								hostname              = "` + first + `"
								ipv4_addresses        = ["10.0.0.1"]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
							{
								hostname              = "` + second + `"
								ipv6_addresses        = ["2001:db8::1"]
								connect_to_ztna       = true
								connect_to_secure_dns = true
							},
						]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_hostname_mappings.test", "id", helpers.SingletonID),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.*", map[string]string{
						"hostname":              first,
						"ipv4_addresses.#":      "1",
						"ipv4_addresses.0":      "10.0.0.1",
						"connect_to_ztna":       "false",
						"connect_to_secure_dns": "false",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.*", map[string]string{
						"hostname":              second,
						"ipv6_addresses.#":      "1",
						"ipv6_addresses.0":      "2001:db8::1",
						"connect_to_ztna":       "true",
						"connect_to_secure_dns": "true",
					}),
				),
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							{
								hostname              = "` + first + `"
								ipv4_addresses        = ["10.0.0.1", "10.0.0.2"]
								ipv6_addresses        = ["2001:db8::2"]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
							{
								hostname              = "` + third + `"
								ipv4_addresses        = ["10.0.0.3"]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
						]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.*", map[string]string{
						"hostname":         first,
						"ipv4_addresses.#": "2",
						"ipv6_addresses.#": "1",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.*", map[string]string{
						"hostname":         third,
						"ipv4_addresses.#": "1",
					}),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_dns_hostname_mappings.test",
				ImportState:       true,
				ImportStateId:     helpers.SingletonID,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSHostnameMappings_ServerDedupesAddresses pins the
// round-trip that made both address collections sets. The server accepts a repeated
// address and stores it once, so a configuration written with a duplicate must settle
// rather than diff on every plan.
func TestAccResource_SecurityCloudDNSHostnameMappings_ServerDedupesAddresses(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireEmptyHostnameMappings(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostnameMappingsDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							{
								hostname              = "tf-acc-dedupe-` + suffix + `.example.com"
								ipv4_addresses        = ["10.0.0.1", "10.0.0.1", "10.0.0.2"]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
						]
					}
				`,
				Check: resource.TestCheckTypeSetElemNestedAttrs("jamfplatform_security_cloud_dns_hostname_mappings.test", "mappings.*", map[string]string{
					"ipv4_addresses.#": "2",
				}),
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSHostnameMappings_OrderDoesNotDiff pins the other
// reason `mappings` is a set. The server returns the collection in an order of its own
// — sending z, a, m reads back m, a, z — so a reordered configuration must produce no
// plan at all.
func TestAccResource_SecurityCloudDNSHostnameMappings_OrderDoesNotDiff(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireEmptyHostnameMappings(t)
	suffix := testhelpers.RunSuffix()

	z := "tf-acc-z-" + suffix + ".example.com"
	a := "tf-acc-a-" + suffix + ".example.com"
	m := "tf-acc-m-" + suffix + ".example.com"

	mapping := func(hostname, address string) string {
		return `{ hostname = "` + hostname + `", ipv4_addresses = ["` + address + `"], connect_to_ztna = false, connect_to_secure_dns = false }`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostnameMappingsDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							` + mapping(z, "10.0.0.1") + `,
							` + mapping(a, "10.0.0.2") + `,
							` + mapping(m, "10.0.0.3") + `,
						]
					}
				`,
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							` + mapping(a, "10.0.0.2") + `,
							` + mapping(m, "10.0.0.3") + `,
							` + mapping(z, "10.0.0.1") + `,
						]
					}
				`,
				PlanOnly: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSHostnameMappings_CreateRefusesToClobber is the whole
// reason Create reads before writing. The wire cannot tell a create from a silent
// takeover — PUT is an unconditional full replace reporting no conflict — so this
// asserts the provider supplies the refusal the server does not, and that the refusal
// leaves the existing mappings intact.
//
// The pre-existing mapping it seeds is the test's own, which is why the cleanup clears
// rather than restoring: it runs only on a tenant that held no mappings to begin with,
// so clearing returns the tenant to the state it was found in.
func TestAccResource_SecurityCloudDNSHostnameMappings_CreateRefusesToClobber(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireEmptyHostnameMappings(t)
	suffix := testhelpers.RunSuffix()

	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	preexisting := "tf-acc-preexisting-" + suffix + ".example.com"
	seeded := []securitycloud.Mapping{{Hostname: preexisting, ARecords: &[]string{"10.0.0.9"}}}
	if err := c.ReplaceDnsCustomHostnameMappingsV1(context.Background(), &seeded); err != nil {
		t.Fatalf("cannot seed a pre-existing mapping: %v", err)
	}
	t.Cleanup(func() {
		if err := c.ClearDnsCustomHostnameMappingsV1(context.Background()); err != nil {
			t.Errorf("cannot clear the seeded mapping: %v", err)
		}
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							{
								hostname              = "tf-acc-clobber-` + suffix + `.example.com"
								ipv4_addresses        = ["10.0.0.1"]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
						]
					}
				`,
				ExpectError: regexp.MustCompile(`Hostname mappings already configured`),
			},
		},
	})

	got, err := c.GetDnsCustomHostnameMappingsV1(context.Background())
	if err != nil {
		t.Fatalf("reading the mappings after the refused create: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Hostname != preexisting {
		t.Errorf("the refused create still changed the mappings: got %+v", got.Results)
	}
}

// TestAccResource_SecurityCloudDNSHostnameMappings_RejectsInvalidConfig pins the
// plan-time validation. Every one of these is refused by the endpoint, and all but the
// size violations arrive with no field named at all — the duplicate host name arrives
// as a 500 with an empty errors array — so catching them at plan time is the only way
// an operator learns what is wrong.
//
// The empty-collection case pairs the empty list with a populated sibling on purpose.
// An empty ipv4_addresses on its own is caught by EachMappingHasAnAddress first, which
// is correct but tests the wrong rule: what needs pinning is that an explicitly empty
// collection is refused even when the mapping does resolve to an address, because
// absent and empty are the same thing on the wire and accepting `[]` as distinct would
// mean a permanent diff.
func TestAccResource_SecurityCloudDNSHostnameMappings_RejectsInvalidConfig(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	cases := map[string]struct {
		mappings    string
		expectError *regexp.Regexp
	}{
		"no addresses": {
			mappings:    `[{ hostname = "a.example.com", connect_to_ztna = false, connect_to_secure_dns = false }]`,
			expectError: regexp.MustCompile(`resolves to no address`),
		},
		"empty ipv4 collection alongside a populated ipv6": {
			mappings:    `[{ hostname = "a.example.com", ipv4_addresses = [], ipv6_addresses = ["2001:db8::1"], connect_to_ztna = false, connect_to_secure_dns = false }]`,
			expectError: regexp.MustCompile(`(?s)ipv4_addresses.*at least 1`),
		},
		"duplicate hostname": {
			mappings: `[
				{ hostname = "dup.example.com", ipv4_addresses = ["10.0.0.1"], connect_to_ztna = false, connect_to_secure_dns = false },
				{ hostname = "dup.example.com", ipv4_addresses = ["10.0.0.2"], connect_to_ztna = false, connect_to_secure_dns = false },
			]`,
			expectError: regexp.MustCompile(`(?s)hostname`),
		},
		"wildcard hostname": {
			mappings:    `[{ hostname = "*.example.com", ipv4_addresses = ["10.0.0.1"], connect_to_ztna = false, connect_to_secure_dns = false }]`,
			expectError: regexp.MustCompile(`Invalid DNS host name`),
		},
		"ipv6 in ipv4_addresses": {
			mappings:    `[{ hostname = "a.example.com", ipv4_addresses = ["2001:db8::1"], connect_to_ztna = false, connect_to_secure_dns = false }]`,
			expectError: regexp.MustCompile(`Invalid IPv4 address`),
		},
		"ipv4 in ipv6_addresses": {
			mappings:    `[{ hostname = "a.example.com", ipv6_addresses = ["10.0.0.1"], connect_to_ztna = false, connect_to_secure_dns = false }]`,
			expectError: regexp.MustCompile(`Invalid IPv6 address`),
		},
		"empty collection": {
			mappings:    `[]`,
			expectError: regexp.MustCompile(`(?s)mappings.*at least 1`),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: `
							resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
								mappings = ` + tc.mappings + `
							}
						`,
						ExpectError: tc.expectError,
					},
				},
			})
		})
	}
}

// TestAccResource_SecurityCloudDNSHostnameMappings_RejectsElevenAddresses pins the
// per-mapping cap at plan time. The endpoint does name the field here, but only after
// the write has been attempted.
func TestAccResource_SecurityCloudDNSHostnameMappings_RejectsElevenAddresses(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	var addresses strings.Builder
	for i := 1; i <= 11; i++ {
		addresses.WriteString(fmt.Sprintf("%q,", fmt.Sprintf("10.0.0.%d", i)))
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							{
								hostname              = "a.example.com"
								ipv4_addresses        = [` + addresses.String() + `]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
						]
					}
				`,
				ExpectError: regexp.MustCompile(`(?s)ipv4_addresses.*at most 10`),
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSHostnameMappings_ImportRejectsOtherIDs pins that the
// import identifier is checked rather than normalised.
func TestAccResource_SecurityCloudDNSHostnameMappings_ImportRejectsOtherIDs(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireEmptyHostnameMappings(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostnameMappingsDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_hostname_mappings" "test" {
						mappings = [
							{
								hostname              = "tf-acc-import-` + suffix + `.example.com"
								ipv4_addresses        = ["10.0.0.1"]
								connect_to_ztna       = false
								connect_to_secure_dns = false
							},
						]
					}
				`,
			},
			{
				ResourceName:  "jamfplatform_security_cloud_dns_hostname_mappings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}
