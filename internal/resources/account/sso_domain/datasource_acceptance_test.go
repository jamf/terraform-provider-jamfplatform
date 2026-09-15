// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package sso_domain_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccDataSource_AccountSSODomain_ReadsAClaimAndItsAssignments covers the
// singular data source against a claim made in the same apply.
//
// The assignment list is asserted empty rather than skipped: a freshly claimed
// domain is in use by no connection, and an empty list — as opposed to a null one
// — is what lets a configuration count the connections a destroy would narrow.
// That is the whole reason the assignment lookup is on this data source.
func TestAccDataSource_AccountSSODomain_ReadsAClaimAndItsAssignments(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := acceptanceDomain("ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}

					data "jamfplatform_account_sso_domain" "by_domain" {
						domain     = jamfplatform_account_sso_domain.test.domain
						depends_on = [jamfplatform_account_sso_domain.test]
					}
				`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_account_sso_domain.by_domain", "domain", domain),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_account_sso_domain.by_domain", "id",
						domainResourceAddress, "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_account_sso_domain.by_domain", "verification_key",
						domainResourceAddress, "verification_key",
					),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_account_sso_domain.by_domain", "verification_txt_record",
						domainResourceAddress, "verification_txt_record",
					),
					resource.TestCheckResourceAttr("data.jamfplatform_account_sso_domain.by_domain", "verification_status", account.DomainStatusPending),
					resource.TestCheckResourceAttr("data.jamfplatform_account_sso_domain.by_domain", "shared", "false"),
					resource.TestCheckResourceAttr("data.jamfplatform_account_sso_domain.by_domain", "assigned_connections.#", "0"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_account_sso_domain.by_domain", "jamf_id_enabled"),
				),
			},
		},
	})
}

// TestAccDataSource_AccountSSODomain_UnclaimedDomainIsWritten pins that a domain
// the organization does not hold produces a written diagnostic pointing at
// `domain`, rather than the bare emptiness of a collection scan finding nothing.
func TestAccDataSource_AccountSSODomain_UnclaimedDomainIsWritten(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_sso_domain" "missing" {
						domain = "tf-acc-no-such-domain-zzz.example"
					}
				`,
				ExpectError: regexpDomainNotClaimed,
			},
		},
	})
}

// TestAccDataSource_AccountSSODomain_MixedCaseRefusedAtPlan pins that the data
// source enforces the same lower-case rule as the resource. Jamf stores a domain
// lower-cased, so a mixed-case lookup would be matched case-insensitively and
// then written back in Jamf's spelling — which for a Required attribute is an
// inconsistent result.
func TestAccDataSource_AccountSSODomain_MixedCaseRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_sso_domain" "mixed" {
						domain = "TF-Acc-DS-Mixed.example"
					}
				`,
				ExpectError: regexpMixedCase,
			},
		},
	})
}

// TestAccDataSource_AccountSSODomains_ListsAClaim checks the plural data source
// surfaces a claim made in the same apply. It asserts the claim is present rather
// than asserting a total, because the organization holds domains this test did
// not make.
func TestAccDataSource_AccountSSODomains_ListsAClaim(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := acceptanceDomain("plural")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}

					data "jamfplatform_account_sso_domains" "all" {
						depends_on = [jamfplatform_account_sso_domain.test]
					}
				`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_account_sso_domains.all", "id", "sso_domains"),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_account_sso_domains.all", "sso_domains.*", map[string]string{
						"domain":              domain,
						"verification_status": account.DomainStatusPending,
						"shared":              "false",
					}),
				),
			},
		},
	})
}

// regexpDomainNotClaimed matches the singular data source's not-found diagnostic.
// Terraform wraps diagnostic text at roughly 80 columns, so the pattern is short
// enough not to straddle a line break.
var regexpDomainNotClaimed = regexp.MustCompile(`Unable to find Jamf Account SSO domain`)
