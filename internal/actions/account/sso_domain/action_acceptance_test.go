// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ssodomainaction_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// What this suite can and cannot cover, stated up front because the gap is
// structural rather than an omission.
//
// A genuine verification cannot be tested. Proving ownership needs a DNS TXT record
// on a domain whose zone the test controls, and CI controls none. The two ways
// around that are both refused: a .example domain is claimable but can never
// resolve, and re-verifying one of the organization's real verified domains would
// be a live write on production sign-in configuration — worse, whether a failed
// verification downgrades an already-verified domain is unprobed, so a re-check
// against a domain whose record has since been removed could take real SSO down.
// So there is no opt-in fixture for it either.
//
// What is covered is everything that does not need DNS control, and it happens to
// include the two behaviours most likely to be got wrong: the five-minute refusal
// that every first verification meets, and the unproven outcome that arrives
// looking like a success. The second needs a domain claimed more than five minutes
// ago, which nothing in a test run can arrange without sleeping for five minutes,
// so it is gated on an operator-declared fixture and skips by default.

// envUnverifiableDomain names a claimed domain that can never verify — a .example
// domain claimed by hand more than five minutes before the run.
//
// It exists because the outcome-classification path cannot be reached in a single
// test run: claiming a domain sets the point the five-minute limit is measured
// from, so any verification a run claims-then-invokes is refused before the
// classification is reached. Sleeping five minutes inside an acceptance test to get
// past that would be worse than skipping.
//
// Every run of this test moves the named domain's last_modified_at and
// verification_expires_at forward, which is exactly the non-idempotence being
// documented. Point it at a throwaway .example domain, never a real one.
const envUnverifiableDomain = "JAMFPLATFORM_ACC_ORGANIZATION_SSO_UNVERIFIABLE_DOMAIN"

// envVerifiedDomain names a claimed domain that is already verified. It is the only
// way to exercise the idempotence path, and it cannot be a fixture this suite
// creates: verification needs a TXT record under a domain the operator controls,
// which no test can publish.
//
// Unlike envUnverifiableDomain this must name a *real* domain, so the guard is the
// opposite one — a .example name is rejected, since it can never verify and would
// make the test fail for the wrong reason. Use a throwaway subdomain of a domain you
// own, with no mailboxes on it: a domain assigned to a connection redirects sign-in
// for every address under it. A subdomain verifies in its own right (wire-probed:
// verifiedTldId comes back null), so the parent's DNS is untouched.
//
// Leave the TXT record published. Jamf re-checks continuously and the verification
// lapses fourteen days after the last success, so removing it turns this test red
// two weeks later for a reason nothing in the repository would explain.
//
// The five-minute limit does not apply to this one: the already-verified refusal is
// returned before the limit is evaluated (wire-probed), so the test is
// deterministic and needs no timing guard.
const envVerifiedDomain = "JAMFPLATFORM_ACC_ORGANIZATION_SSO_VERIFIED_DOMAIN"

func accountClient(t *testing.T) *account.Client {
	t.Helper()
	return account.New(testhelpers.NewAcceptanceClient(t))
}

// claimDomainFixture claims a throwaway .example domain and returns its name and
// identifier, releasing it when the test ends.
//
// The fixture is built through the SDK rather than with a jamfplatform_account_sso_domain
// resource for two reasons. It keeps this package's tests independent of the
// resource package's own suite, and — the reason that matters — the claim is what
// starts the five-minute clock, so a test asserting on the refusal has to know
// exactly when the claim happened.
//
// A reserved-TLD name is deliberate: .example is accepted by the claim (wire-probed)
// and can never resolve, so nothing this suite creates can ever become a verified
// domain in a real organization.
func claimDomainFixture(t *testing.T, label string) (string, string) {
	t.Helper()

	client := accountClient(t)
	name := fmt.Sprintf("tfacc-%s-%s.example", label, testhelpers.RunSuffix())

	domain, err := client.CreateDomain(context.Background(), &account.DomainRequest{Domain: name})
	if err != nil {
		t.Skipf("could not claim the fixture domain %s: %v", name, err)
	}
	if domain == nil || domain.ID == nil {
		t.Skipf("claiming the fixture domain %s reported no identifier", name)
	}
	id := domain.ID.String()

	t.Cleanup(func() {
		if err := client.DeleteDomain(context.Background(), id); err != nil {
			t.Errorf("releasing the fixture domain %s (%s): %v", name, id, err)
		}
	})

	return name, id
}

// verifyConfig builds a configuration whose only job is to invoke the action.
//
// terraform_data carries the trigger because an action has to be attached to a
// resource lifecycle event to run at all, and this suite has no resource of its own
// to hang it on — the domain fixture is created outside Terraform on purpose.
func verifyConfig(arguments string) string {
	return fmt.Sprintf(`
action "jamfplatform_account_sso_domain_verify" "test" {
  config {
%s
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_account_sso_domain_verify.test]
    }
  }
}
`, arguments)
}

// TestAccAction_AccountVerifySSODomain_RateLimitedByName pins the refusal a
// practitioner meets first, and the resolution path the documentation leads with.
//
// The domain is named in upper case while the service stores it lower-cased, so a
// pass also proves the case-insensitive match — the run would fail with "not
// claimed by this organization" if that fold were dropped, which is a different
// diagnostic and therefore a real assertion rather than an incidental one.
func TestAccAction_AccountVerifySSODomain_RateLimitedByName(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name, _ := claimDomainFixture(t, "byname")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: verifyConfig(fmt.Sprintf("    domain = %q", strings.ToUpper(name))),
			// Anchored on no-space tokens with \s+ for the gaps: Terraform wraps
			// error output at ~80 columns and where the wrap lands shifts with the
			// message around it.
			ExpectError: regexp.MustCompile(`one\s+domain\s+verification\s+every\s+five\s+minutes`),
		}},
	})
}

// TestAccAction_AccountVerifySSODomain_RateLimitedByID covers the same refusal
// through the identifier form.
//
// Worth its own test rather than a variation: the identifier form skips the lookup
// entirely, so it is the only path that exercises the action against a domain it
// never read, and the only one a caller holding just the update permission can use.
func TestAccAction_AccountVerifySSODomain_RateLimitedByID(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	_, id := claimDomainFixture(t, "byid")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      verifyConfig(fmt.Sprintf("    domain_id = %q", id)),
			ExpectError: regexp.MustCompile(`one\s+domain\s+verification\s+every\s+five\s+minutes`),
		}},
	})
}

// TestAccAction_AccountVerifySSODomain_Unverifiable is the one test that reaches the
// outcome classification against the live service: a claimed domain that can never
// resolve answers the verification successfully and reports no proof of ownership,
// and the action has to turn that into a failure.
//
// It needs a domain claimed more than five minutes before the run, which is why it
// is declared rather than created — see envUnverifiableDomain. It moves that
// domain's timestamps every time it runs.
func TestAccAction_AccountVerifySSODomain_Unverifiable(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	name := testhelpers.AccEnv(envUnverifiableDomain)
	if name == "" {
		t.Skipf("%s must name a claimed throwaway .example domain, claimed more than five minutes before the "+
			"run, for this test — the five-minute limit is measured from the claim, so a domain this run "+
			"claims cannot reach the verification outcome", envUnverifiableDomain)
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".example") {
		t.Skipf("%s must name a .example domain; every invocation moves the domain's timestamps forward, so "+
			"it must never be a real one", envUnverifiableDomain)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      verifyConfig(fmt.Sprintf("    domain = %q", name)),
			ExpectError: regexp.MustCompile(`ownership\s+was\s+not\s+verified`),
		}},
	})
}

// TestAccAction_AccountVerifySSODomain_NotClaimed pins the diagnostic for a domain
// the organization has not claimed, which is the mistake the name form makes
// possible in exchange for its ergonomics.
//
// It needs no fixture, which is the point: it runs on any organization.
func TestAccAction_AccountVerifySSODomain_NotClaimed(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: verifyConfig(fmt.Sprintf("    domain = %q",
				fmt.Sprintf("tfacc-never-claimed-%s.example", testhelpers.RunSuffix()))),
			ExpectError: regexp.MustCompile(`not\s+claimed\s+by\s+this\s+organization`),
		}},
	})
}

// TestAccAction_AccountVerifySSODomain_UnknownID pins the diagnostic for an
// identifier the organization has no domain for — the failure the identifier form
// makes possible, and the reason its description warns against hard-coding one.
func TestAccAction_AccountVerifySSODomain_UnknownID(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      verifyConfig(`    domain_id = "99999999"`),
			ExpectError: regexp.MustCompile(`Domain\s+not\s+found`),
		}},
	})
}

// TestAccAction_AccountVerifySSODomain_PlanTimeValidation covers the refusals that
// never reach Jamf Account.
//
// Naming neither identifier is the case a per-attribute conflict rule would miss
// entirely: it would plan cleanly and then reach the invocation with nothing to act
// on, part-way through an apply.
//
// Both of the ExactlyOneOf cases match the shared *detail* rather than the
// summary, because the summary differs between them — "Invalid Attribute
// Combination" when both are set, "Missing Attribute Configuration" when neither
// is. Asserting on either summary passes one case for the wrong reason and fails
// the other, which is exactly what happened on the first live run.
func TestAccAction_AccountVerifySSODomain_PlanTimeValidation(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	tests := []struct {
		name        string
		arguments   string
		expectError *regexp.Regexp
	}{
		{
			name:        "neither identifier",
			arguments:   "",
			expectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
		},
		{
			name: "both identifiers",
			arguments: "    domain    = \"claimed.example\"\n" +
				"    domain_id = \"26917\"",
			expectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
		},
		{
			name:        "empty domain",
			arguments:   `    domain = ""`,
			expectError: regexp.MustCompile(`at\s+least\s+1`),
		},
		{
			name:        "empty identifier",
			arguments:   `    domain_id = ""`,
			expectError: regexp.MustCompile(`at\s+least\s+1`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      verifyConfig(tc.arguments),
					ExpectError: tc.expectError,
				}},
			})
		})
	}
}

// TestAccAction_AccountVerifySSODomain_AlreadyVerifiedIsNotAnError covers the
// idempotence path: checking a domain that is already verified succeeds and reports
// nothing to do.
//
// Wire-probed 2026-09-02, and it is not what the code suggests. Jamf Account
// refuses the check with 409 CONFLICT "Domain is already verified" rather than
// answering 200 with domainStatus VERIFIED, and it refuses before applying the
// five-minute limit — so this test is deterministic and needs no timing guard.
// Treating that refusal as a failure would make the action fail on its second run
// for having succeeded on its first, which no re-runnable pipeline tolerates.
//
// **The first-time transition from unverified to verified cannot be tested here,
// and not for want of trying.** It needs a domain whose freshly minted
// verificationKey is then published in DNS, and the key is reissued every time a
// domain is claimed — so a record published for an earlier claim is stale against a
// new one. Automating it would require the suite to write DNS. The classification
// logic behind that outcome is covered by unit tests instead
// (TestReportOutcome_VerifiedReportsProgress and its siblings), and the negative
// half is covered live by TestAccAction_AccountVerifySSODomain_Unverifiable.
//
// The assertion is that the apply succeeds. An action holds no state, and the
// outcome is reported through SendProgress, which the harness does not expose — so
// success is the absence of the diagnostic the unverified case asserts is present.
// The pair is what gives either test its force.
func TestAccAction_AccountVerifySSODomain_AlreadyVerifiedIsNotAnError(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	name := strings.ToLower(strings.TrimSpace(testhelpers.AccEnv(envVerifiedDomain)))
	if name == "" {
		t.Skipf("%s must name a claimed, already-verified domain for this test", envVerifiedDomain)
	}
	if strings.HasSuffix(name, ".example") {
		t.Skipf("%s names %q, but a .example domain can never verify; this test needs a real domain you "+
			"own whose ownership record is published", envVerifiedDomain, name)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: verifyConfig(fmt.Sprintf("    domain = %q", name)),
		}},
	})
}
