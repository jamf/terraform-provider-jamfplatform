// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package dns_search_domain_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckSearchDomainDestroy asserts the search domain really is gone.
//
// This is the ordinary CheckDestroy contract, not the inverted one
// STYLE_GUIDE §Singleton resources prescribes, because this singleton's Delete is a
// real clear: the endpoint honours DELETE and an unset search domain answers 404.
// Asserting "still exists" here — the Pro singleton shape — would pass while the
// provider silently failed to clear anything.
func testAccCheckSearchDomainDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_dns_search_domain" {
				continue
			}
			got, err := c.GetDnsSearchDomainV1(ctx)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud search domain: %s", err)
			}
			return fmt.Errorf("Jamf Security Cloud search domain still set to %q", got.Suffix)
		}
		return nil
	}
}

// requireNoSearchDomain skips the test unless the tenant holds no search domain, so
// a test only runs when the absent state its Create step needs is already the case.
//
// Create deliberately refuses to overwrite an existing search domain, which makes
// the tenant's prior state a precondition rather than something to arrange. Clearing
// to arrange it would destroy tenant-wide configuration this test does not own and
// cannot restore: there is one search domain per tenant, so whatever holds it was
// set by an administrator or by a concurrent run. Skipping says so instead — an
// occupied tenant is a legitimate environment, not a failure, the same reasoning
// AccPreCheckSecurityCloud applies to a Pro-only tenant.
func requireNoSearchDomain(t *testing.T) {
	t.Helper()
	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	got, err := c.GetDnsSearchDomainV1(context.Background())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			return
		}
		t.Fatalf("cannot read the tenant's search domain: %v", err)
	}
	if got != nil && got.Suffix != "" {
		t.Skipf("this test would destroy the tenant's existing search domain %q", got.Suffix)
	}
}

// TestAccResource_SecurityCloudDNSSearchDomain_Lifecycle covers create, in-place
// replacement and destroy.
//
// The replacement step is the one worth having: the endpoint has no identifier and
// PUT is an upsert, so a second write with a different value must land as an update
// rather than a replace. A RequiresReplace creeping onto domain_name would show up
// here as a destroy-and-create, which on a tenant-wide setting means a window with
// no search domain at all.
func TestAccResource_SecurityCloudDNSSearchDomain_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoSearchDomain(t)
	suffix := testhelpers.RunSuffix()

	first := "tf-acc-" + suffix + ".example.com"
	second := "tf-acc-updated-" + suffix + ".example.org"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSearchDomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_search_domain" "test" {
						domain_name = "` + first + `"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_search_domain.test", "id", helpers.SingletonID),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_search_domain.test", "domain_name", first),
				),
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_search_domain" "test" {
						domain_name = "` + second + `"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_search_domain.test", "id", helpers.SingletonID),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_dns_search_domain.test", "domain_name", second),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_dns_search_domain.test",
				ImportState:       true,
				ImportStateId:     helpers.SingletonID,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSSearchDomain_ImportRejectsOtherIDs pins that the
// import identifier is checked rather than normalised. The endpoint takes no
// identifier, so a mis-typed import would otherwise succeed against whatever the
// tenant holds and hide the mistake.
func TestAccResource_SecurityCloudDNSSearchDomain_ImportRejectsOtherIDs(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoSearchDomain(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSearchDomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_search_domain" "test" {
						domain_name = "tf-acc-import-` + suffix + `.example.com"
					}
				`,
			},
			{
				ResourceName:  "jamfplatform_security_cloud_dns_search_domain.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_SecurityCloudDNSSearchDomain_CreateRefusesToClobber is the whole
// reason Create reads before writing. The wire cannot tell a create from a silent
// takeover — PUT is an unconditional upsert reporting no conflict — so this asserts
// the provider supplies the refusal the server does not.
func TestAccResource_SecurityCloudDNSSearchDomain_CreateRefusesToClobber(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoSearchDomain(t)
	suffix := testhelpers.RunSuffix()

	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	preexisting := "tf-acc-preexisting-" + suffix + ".example.com"
	if err := c.SetDnsSearchDomainV1(context.Background(), &securitycloud.SearchDomain{Suffix: preexisting}); err != nil {
		t.Fatalf("cannot seed a pre-existing search domain: %v", err)
	}
	t.Cleanup(func() {
		if err := c.ClearDnsSearchDomainV1(context.Background()); err != nil {
			t.Errorf("cannot clear the seeded search domain: %v", err)
		}
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_security_cloud_dns_search_domain" "test" {
						domain_name = "tf-acc-clobber-` + suffix + `.example.com"
					}
				`,
				ExpectError: regexp.MustCompile(`Search domain already configured`),
			},
		},
	})

	got, err := c.GetDnsSearchDomainV1(context.Background())
	if err != nil {
		t.Fatalf("reading the search domain after the refused create: %v", err)
	}
	if got.Suffix != preexisting {
		t.Errorf("the refused create still changed the search domain: got %q, want %q", got.Suffix, preexisting)
	}
}

// TestAccResource_SecurityCloudDNSSearchDomain_RejectsMalformedNames pins the
// plan-time validation. Every one of these is refused by the endpoint with the same
// opaque 400 naming no field, so catching them at plan time is the only way an
// operator learns which attribute is at fault.
func TestAccResource_SecurityCloudDNSSearchDomain_RejectsMalformedNames(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	for name, value := range map[string]string{
		"wildcard":            "*.example.com",
		"leading dot":         ".example.com",
		"numeric final label": "example.123",
		"bare ipv4":           "203.0.113.53",
		"spaces":              "not a domain",
	} {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: `
							resource "jamfplatform_security_cloud_dns_search_domain" "test" {
								domain_name = "` + value + `"
							}
						`,
						ExpectError: regexp.MustCompile(`Invalid DNS host name`),
					},
				},
			})
		})
	}
}
