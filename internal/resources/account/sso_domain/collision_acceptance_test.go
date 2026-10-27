// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package sso_domain_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_AccountSSODomain_Collision(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := "tf-acc-collide-" + testhelpers.RunSuffix() + ".example"
	c := account.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					created, err := c.CreateDomain(ctx, &account.DomainRequest{Domain: domain})
					if err != nil {
						t.Fatalf("claiming domain %q outside Terraform: %v", domain, err)
					}
					t.Cleanup(func() { _ = c.DeleteDomain(ctx, created.ID.String()) })
				},
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_domain" "test" {
						domain = %q
					}
				`, domain),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Domain\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Account`),
			},
		},
	})
}

func twoDomainsConfig(domainOne, domainTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_account_sso_domain" "one" {
  domain = %q
}

resource "jamfplatform_account_sso_domain" "two" {
  depends_on = [jamfplatform_account_sso_domain.one]

  domain = %q
}
`, domainOne, domainTwo)
}

func TestAccResource_AccountSSODomain_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix + ".example"
	two := "tf-acc-collide-managed-two-" + suffix + ".example"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{Config: twoDomainsConfig(one, two)},
			{
				Config:      twoDomainsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Domain\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_AccountSSODomain_TwoNewWithOneDomainExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := "tf-acc-collide-pair-" + testhelpers.RunSuffix() + ".example"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSODomainDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      twoDomainsConfig(domain, domain),
				ExpectError: regexp.MustCompile(`(?s)Domain\s+already\s+claimed.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
