// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package sso_domain_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// domainResourceAddress is the address every test in this file uses for the
// subject resource.
const domainResourceAddress = "jamfplatform_account_sso_domain.test"

// Acceptance domains sit under `.example`, the top-level domain RFC 6761 reserves
// for documentation. It can never resolve, so no verification can ever
// accidentally succeed against infrastructure someone operates — and Jamf accepts
// it, wire-probed 2026-09-02: claiming is deliberately not a check that a domain
// is real or reachable.
//
// The suffix keeps runs from colliding, which matters more here than usual: a
// claim is unique across the whole organization, so a leftover from an
// interrupted run refuses the next one outright rather than merely cluttering the
// tenant.
func acceptanceDomain(part string) string {
	return "tf-acc-" + part + "-" + testhelpers.RunSuffix() + ".example"
}

// testAccCheckSSODomainDestroy verifies the claims made during the test were
// withdrawn.
//
// The check scans the organization's domain collection, because Jamf Account
// exposes no read of a single claim — the same reason the resource's own Read
// does. It matches on the domain name rather than the recorded identifier for
// the same reason the resource does: the name is what a claim is addressed by.
func testAccCheckSSODomainDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := account.New(testhelpers.NewAcceptanceClient(t))

		var wanted []string
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_account_sso_domain" {
				continue
			}
			if domain := rs.Primary.Attributes["domain"]; domain != "" {
				wanted = append(wanted, domain)
			}
		}
		if len(wanted) == 0 {
			return nil
		}

		domains, err := c.ListDomains(context.Background())
		if err != nil {
			return fmt.Errorf("error listing Jamf Account SSO domains: %w", err)
		}
		for _, domain := range domains {
			for _, want := range wanted {
				if strings.EqualFold(domain.Domain, want) {
					return fmt.Errorf("Jamf Account SSO domain %s is still claimed", want)
				}
			}
		}
		return nil
	}
}

// TestAccResource_AccountSSODomain_Basic covers the claim and the import round
// trip.
//
// There is no update step and there cannot be one: Jamf Account exposes no way to
// modify a claim, so `domain` is RequiresReplace and every other attribute is
// read-only. Replacement is covered separately, in
// TestAccResource_AccountSSODomain_DomainChangeReplaces.
//
// Four of the assertions are worth stating the reasoning for, since none of them
// is obvious from the value being checked.
//
// `verification_status` is pinned to PENDING because a fresh claim is minted with
// a token and no verification behind it, and a domain under `.example` can never
// move off that state — so this is a stable assertion rather than a snapshot of
// whatever the tenant happened to hold.
//
// `verification_txt_record` is checked for its prefix rather than taken on trust:
// the assembled record is what a DNS resource consumes, and getting the prefix
// wrong fails silently, by the domain simply never verifying.
//
// `created_by`, `last_verified_at` and `parent_domain_id` are asserted *absent*
// rather than empty. Each is legitimately null here — a claim made over an
// integration has no Jamf Account user behind it, the domain has never verified,
// and it inherits from no verified parent — and an empty string in any of them
// would be a state-builder defect that no other assertion would catch.
//
// The import step sets `ImportStateId` explicitly because import is by domain
// name: the harness defaults to the `id` attribute, which for this resource is
// the Jamf-assigned identifier the import path deliberately does not accept.
// Three attributes are ignored on import. `timeouts` is provider-side
// configuration with no counterpart in Jamf Account, so an imported resource can
// never carry it.
//
// `last_modified_at` and `verification_expires_at` are ignored because **Jamf
// moves them on its own, with no request from the client**. Observed twice live on
// 2026-09-02: here, a claim response carrying 14:09:58 against a read two seconds
// later carrying 14:10:00, expiry shifted identically; and separately, a domain
// claimed at 14:21:22 whose lastModifiedDate had become 14:24:16 by the time it
// was next read.
//
// Whatever performs that touch is **not** a verification attempt, which is worth
// stating because it is the obvious guess: lastVerificationDate stayed null across
// it and the status stayed PENDING even on a domain whose DNS record was already
// live and which verified successfully minutes later. So this is not the console's
// "continuous re-check" doing its job, and no assertion should assume it is.
//
// Ignoring them is the correct handling rather than a workaround. Both are
// Computed-only, so a later value is absorbed by refresh without producing a
// diff; only ImportStateVerify compares the two reads. The alternative — polling
// in Read until the value settles — would turn a routine server-side touch into
// a refresh failure, and there is no settled value to wait for anyway, since the
// sweep recurs.
func TestAccResource_AccountSSODomain_Basic(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := acceptanceDomain("basic")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}
				`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainResourceAddress, "domain", domain),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "id"),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "account_id"),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "created_at"),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "last_modified_at"),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "verification_expires_at"),
					resource.TestCheckResourceAttr(domainResourceAddress, "verification_status", account.DomainStatusPending),
					resource.TestCheckResourceAttr(domainResourceAddress, "shared", "false"),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "verification_key"),
					resource.TestCheckResourceAttrWith(domainResourceAddress, "verification_txt_record", func(value string) error {
						if !strings.HasPrefix(value, "jamf-site-verification=") {
							return fmt.Errorf("verification_txt_record = %q, want the jamf-site-verification prefix", value)
						}
						return nil
					}),
					resource.TestCheckNoResourceAttr(domainResourceAddress, "created_by"),
					resource.TestCheckNoResourceAttr(domainResourceAddress, "last_verified_at"),
					resource.TestCheckNoResourceAttr(domainResourceAddress, "parent_domain_id"),
				),
			},
			{
				ResourceName:      domainResourceAddress,
				ImportState:       true,
				ImportStateId:     domain,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"last_modified_at",
					"verification_expires_at",
				},
			},
		},
	})
}

// TestAccResource_AccountSSODomain_DomainChangeReplaces stands in for the update
// round trip a mutable resource would have.
//
// A claim cannot be edited, so the only thing to verify is that Terraform plans a
// replacement rather than an in-place change — and that the replacement is issued
// its own verification token, which is the fact that makes replacement more than
// a bookkeeping detail: the operator's TXT record has to be republished.
func TestAccResource_AccountSSODomain_DomainChangeReplaces(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	first := acceptanceDomain("replace-a")
	second := acceptanceDomain("replace-b")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}
				`, first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainResourceAddress, "domain", first),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}
				`, second),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(domainResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainResourceAddress, "domain", second),
					resource.TestCheckResourceAttrSet(domainResourceAddress, "verification_key"),
				),
			},
		},
	})
}

// TestAccResource_AccountSSODomain_Disappears covers the drift path: a claim
// withdrawn out from under Terraform has to be dropped from state and planned
// afresh rather than failing the refresh. It is worth its own test here because
// absence from the collection is the only signal — there is no read of a single
// claim to return a not-found status.
func TestAccResource_AccountSSODomain_Disappears(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := acceptanceDomain("gone")

	config := fmt.Sprintf(`
		resource "jamfplatform_account_sso_domain" "test" {
			domain = %q
		}
	`, domain)

	var domainID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureAttribute(domainResourceAddress, "id", &domainID),
				),
			},
			{
				PreConfig: func() {
					c := account.New(testhelpers.NewAcceptanceClient(t))
					if err := c.DeleteDomain(context.Background(), domainID); err != nil {
						t.Fatalf("drift preconfig: withdrawing SSO domain %s: %v", domainID, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_AccountSSODomain_DuplicateClaimPointsAtTheDomain drives the one
// refusal a practitioner is most likely to meet. Jamf reports it as a bare
// conflict naming a state and no remedy, so the provider rewrites it — and the
// rewrite is only worth having if it survives contact with the real response.
func TestAccResource_AccountSSODomain_DuplicateClaimPointsAtTheDomain(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := acceptanceDomain("dupe")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}

					resource "jamfplatform_account_sso_domain" "duplicate" {
						domain     = %q
						depends_on = [jamfplatform_account_sso_domain.test]
					}
				`, domain, domain),
				ExpectError: regexpDomainAlreadyClaimed,
			},
		},
	})
}

// TestAccResource_AccountSSODomain_MixedCaseRefusedAtPlan pins the normalisation
// guard. Jamf lower-cases the value it stores, so a mixed-case configuration
// would apply and read back changed — the refusal has to land at plan time, and
// the diagnostic has to name the spelling to use.
func TestAccResource_AccountSSODomain_MixedCaseRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_domain" "test" {
						domain = "TF-Acc-Mixed-Case.example"
					}
				`,
				ExpectError: regexpMixedCase,
			},
		},
	})
}

// TestAccResource_AccountSSODomain_URLRefusedAtPlan pins the paste-a-URL guard.
// Jamf refuses the value too, but with a message naming neither the value nor the
// part that offends.
func TestAccResource_AccountSSODomain_URLRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_domain" "test" {
						domain = "https://tf-acc-url.example/path"
					}
				`,
				ExpectError: regexpNotBareDomain,
			},
		},
	})
}

// TestAccListResource_AccountSSODomain_Basic exercises the list resource via the
// `terraform query` workflow. The domain collection takes no filter, so step 2
// asserts the claim made in step 1 appears among the results rather than pinning
// a total — the organization holds domains this test did not make.
//
// Requires Terraform 1.14+ (list resources).
func TestAccListResource_AccountSSODomain_Basic(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := acceptanceDomain("list")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}
				`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(domainResourceAddress, "id"),
				),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_account_sso_domain" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_account_sso_domain.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(domain)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("domain"), KnownValue: knownvalue.StringExact(domain)},
							{Path: tfjsonpath.New("verification_status"), KnownValue: knownvalue.StringExact(account.DomainStatusPending)},
						},
					),
				},
			},
		},
	})
}

// captureAttribute records an applied attribute so a later step can act on it.
// PreConfig runs without access to state, so a value has to be carried out of the
// apply step this way.
func captureAttribute(address, name string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		value, ok := rs.Primary.Attributes[name]
		if !ok {
			return fmt.Errorf("resource %s has no %s attribute", address, name)
		}
		*into = value
		return nil
	}
}

// Expected-error patterns for the plan- and apply-time refusals. Terraform wraps
// diagnostic text at roughly 80 columns, so each pattern matches a short phrase
// that cannot be split across a line break.
var (
	regexpDomainAlreadyClaimed = regexp.MustCompile(`Domain already claimed`)
	regexpMixedCase            = regexp.MustCompile(`must be lower case`)
	regexpNotBareDomain        = regexp.MustCompile(`not a bare domain`)
)
