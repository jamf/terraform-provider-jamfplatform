// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ssodomainaction implements the fire-once Jamf Account SSO domain
// action jamfplatform_account_sso_domain_verify — the "Verify" control beside a
// claimed domain on Jamf Account's Single Sign-On > Domains page.
//
// Verification is an action rather than part of the
// jamfplatform_account_sso_domain resource's lifecycle for three reasons the wire
// settled (spike/ACCOUNT_SSO_SPIKE.md §3.5.4, probed 2026-09-02):
//
//   - A failed verification is not an error. POST /sso/v1/domains/{id}/actions/verify
//     against a domain whose TXT record is missing or wrong answers 200 with the
//     full Domain body, domainStatus unchanged and lastVerificationDate still null.
//     The status code carries no outcome, so the response body has to be read.
//   - The call is rate-limited to once every five minutes, measured from
//     lastModifiedDate — which claiming the domain itself sets, so the first verify
//     after a create is always refused. Worse for a poller, the window has no fixed
//     end: Jamf moves lastModifiedDate on its own, so the deadline can be pushed
//     forward while a caller waits. Both facts rule out the resource polling for
//     verification during Create.
//   - It is not idempotent even when it fails. A verify that reports PENDING still
//     bumps lastModifiedDate and pushes verificationExpirationDate out to now + 14
//     days, so every call mutates two server-derived timestamps and restarts its
//     own rate-limit window.
//
// Nothing else on a domain belongs here. Claiming and releasing one are the
// resource's Create and Delete, and there is no update route to wrap: GET, PUT and
// PATCH on /sso/v1/domains/{id} all answer 403 BAD_PERMISSIONS, which by this
// repo's own law (see CLAUDE.md §Jamf Security Cloud) means unmapped rather than
// unprivileged.
package ssodomainaction

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ssoDomainAction shares Configure logic across the Jamf Account SSO domain
// actions.
type ssoDomainAction struct {
	client *account.Client
}

// configure binds the provider-supplied Jamf Account client to the action.
//
// It goes through ConfigureAccount rather than ConfigurePro: Jamf Account is
// organization-level, has no customer-tenant version to gate on, and an
// organization need hold no Jamf Pro tenant at all, so a Pro version read would be
// both meaningless and fatal here.
func (a *ssoDomainAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_domain_verify")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.client = client
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *ssoDomainAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Account client was not configured. Re-run terraform init/apply so the provider can "+
			"configure successfully.",
	)
	return false
}

// resolveDomainID returns the identifier the verify call takes for the domain the
// configuration names.
//
// A configured domain_id is used as given. A configured domain has to be matched
// against the organization's claimed domains, because nothing resolves one
// directly: there is no per-domain read route (GET /sso/v1/domains/{id} is
// unmapped) and no route takes a domain name — GetDomainAllocation does, but
// returns the connections a domain is assigned to and not its id.
//
// The match is case-insensitive because the service stores a claimed domain
// lower-cased whatever case it was claimed in (wire-probed: an uppercase claim
// returns 201 with a lower-cased domain).
func (a *ssoDomainAction) resolveDomainID(ctx context.Context, data VerifySSODomainActionModel, diags *diag.Diagnostics) (string, bool) {
	if !data.DomainID.IsNull() {
		id := strings.TrimSpace(data.DomainID.ValueString())
		if id == "" {
			diags.AddAttributeError(
				path.Root("domain_id"),
				"Domain identifier is blank",
				"`domain_id` holds nothing but whitespace, so there is no domain to verify. Set it to the "+
					"`id` attribute of your jamfplatform_account_sso_domain resource, or name the domain "+
					"with `domain` instead.",
			)
			return "", false
		}
		return id, true
	}

	name := strings.TrimSpace(data.Domain.ValueString())
	if name == "" {
		diags.AddAttributeError(
			path.Root("domain"),
			"Domain name is blank",
			"`domain` holds nothing but whitespace, so there is no domain to verify. Set it to the domain "+
				"name as it appears on Jamf Account's Single Sign-On > Domains page.",
		)
		return "", false
	}

	domains, err := a.client.ListDomains(ctx)
	if err != nil {
		diags.AddError(
			"Could not read this organization's claimed domains",
			fmt.Sprintf("Naming a domain by name means looking it up among the domains this organization has "+
				"claimed, and that read failed, so there is nothing to verify. Set `domain_id` instead to skip "+
				"the lookup. Reported by Jamf Account: %s", helpers.APIErrorDetail(err)),
		)
		return "", false
	}

	for _, domain := range domains {
		if !strings.EqualFold(strings.TrimSpace(domain.Domain), name) {
			continue
		}
		if domain.ID == nil {
			diags.AddAttributeError(
				path.Root("domain"),
				"Claimed domain has no identifier",
				fmt.Sprintf("Jamf Account lists %q among this organization's claimed domains but reports no "+
					"identifier for it, so it cannot be verified. Please report this issue to the provider "+
					"developers.", domain.Domain),
			)
			return "", false
		}
		return domain.ID.String(), true
	}

	diags.AddAttributeError(
		path.Root("domain"),
		"Domain is not claimed by this organization",
		fmt.Sprintf("Jamf Account has no claimed domain %q in this organization, so there is nothing to "+
			"verify. Claim it first with a jamfplatform_account_sso_domain resource, or on Jamf Account's "+
			"Single Sign-On > Domains page, and check the spelling. A domain claimed by a different "+
			"organization is not visible here.", name),
	)
	return "", false
}

// appendInvokeDiagnostics turns a refused verification into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// The rate limit is the one that earns a named diagnostic. It is measured from the
// domain's lastModifiedDate, which claiming the domain sets, so the very first
// verification of a newly claimed domain is refused — and the raw body says only
// "Can only verify once every five minutes" with field null, which reads like a
// provider bug rather than something to wait out.
//
// BAD_REQUEST is also the code for a malformed identifier, so the description is
// matched as well as the code. That is the SDK's advice inverted deliberately
// (prefer the code over the description): the code alone is not specific enough
// here, and the description is the only thing that separates the two.
func appendInvokeDiagnostics(diags *diag.Diagnostics, err error, target string, idPath path.Path) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}

	matched := false
	for _, detail := range apiErr.Details() {
		switch {
		case detail.Code == codeBadRequest && strings.Contains(strings.ToLower(detail.Description), rateLimitMarker):
			addRateLimited(diags, target, detail.Description)
		case detail.Code == codeNotFound:
			diags.AddAttributeError(
				idPath,
				"Domain not found",
				fmt.Sprintf("Jamf Account has no claimed domain %s in this organization, so there is nothing "+
					"to verify. A domain that was released and claimed again is issued a new identifier, so a "+
					"hard-coded one goes stale — reference the `id` of your jamfplatform_account_sso_domain "+
					"resource, or name the domain with `domain` instead. Reported by Jamf Account: %s",
					target, detail.Description),
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// addRateLimited reports the five-minute verification limit as something to wait
// out rather than debug.
//
// It says why the provider does not wait: a pause inside an apply holds the whole
// run for a delay the operator can neither shorten nor observe, the first
// verification after a claim is guaranteed to hit it, and a retry loop would be
// actively harmful because every call — refused or not — restarts the window and
// pushes the domain's expiry out another 14 days.
//
// It also tells the operator to *re-read* the domain rather than counting five
// minutes from a lastModifiedDate they already hold, because the window has no
// fixed end. Observed live on 2026-09-02: a domain claimed at 14:21:22 had its
// lastModifiedDate moved to 14:24:16 with no request from the client, so a verify
// at 14:26:39 — comfortably past five minutes from the claim — was still refused,
// and the call that succeeded landed at 14:29:41. Whatever performs that touch is
// not a verification attempt: lastVerificationDate stayed null across it and the
// status stayed PENDING even though the DNS record was already live.
func addRateLimited(diags *diag.Diagnostics, target, description string) {
	diags.AddError(
		"Jamf Account allows only one domain verification every five minutes",
		fmt.Sprintf("Domain %s changed less than five minutes ago, so Jamf Account refused to check it "+
			"again. Claiming a domain counts as a change, so the first verification after a domain is claimed "+
			"is always refused for five minutes.\n\n"+
			"Nothing is wrong with the configuration. Re-read the domain, wait until five minutes have passed "+
			"since its current `last_modified_at`, then invoke the action again — publishing the DNS record "+
			"usually takes longer than that anyway. Re-read rather than counting from the value you already "+
			"have: Jamf Account moves `last_modified_at` on its own, without any request, which restarts the "+
			"window. This provider does not wait it out on your behalf, because the wait has no fixed end: a "+
			"pause inside an apply would hold the whole run for a delay that can be extended while it waits, "+
			"and every verification restarts the window whether or not it succeeds.\n\n"+
			"Reported by Jamf Account: %s", target, description),
	)
}

// isAlreadyVerified reports whether Jamf Account refused the check because the
// domain is already verified.
//
// That refusal is not a failure of the action: the state the operator asked for is
// the state the domain is in. Reporting it as an error would make an action fail on
// its second run for having succeeded on its first, which no re-runnable pipeline
// can live with — and unlike the rate limit there is nothing to wait for and
// nothing to fix.
//
// CONFLICT alone is not enough to match on: a duplicate domain claim carries the
// same code, and Field is null on both, so the description does the separating.
func isAlreadyVerified(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	for _, detail := range apiErr.Details() {
		if detail.Code == codeConflict && strings.Contains(strings.ToLower(detail.Description), alreadyVerifiedMarker) {
			return true
		}
	}
	return false
}
