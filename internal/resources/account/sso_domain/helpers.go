// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// appendClaimDiagnostics turns a claim failure into the most specific diagnostic
// the error body supports, and reports whether it recognised one.
//
// All three translated codes are about the domain itself, so all three attach to
// `domain` — the value in the configuration is the only thing the operator can
// change in response. The translation is worth doing anyway because Jamf's own
// messages name a state ("Domain is already added to your organization") without
// naming the remedy, and because a claim held by another organization is
// indistinguishable from one held by this one in the raw body.
//
// CONFLICT is deliberately not narrowed to "you already claimed it": the same
// code is documented for a domain another organization has verified, which is
// the same wire shape and a completely different fix.
func appendClaimDiagnostics(diags *diag.Diagnostics, domain string, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeConflict:
			diags.AddAttributeError(
				path.Root("domain"),
				"Domain already claimed",
				"The domain \""+domain+"\" is already claimed. A domain can belong to only one Jamf Account "+
					"organization: either your organization already holds it — in which case import it rather "+
					"than claiming it again — or another organization has verified it, in which case Jamf "+
					"Support has to release it first. Reported by Jamf Account: "+detail.Description,
			)
		case codeBadRequest:
			diags.AddAttributeError(
				path.Root("domain"),
				"Domain not accepted",
				"Jamf Account refused \""+domain+"\" as a domain name. It has to be a bare domain — no scheme, "+
					"no path, no port and no user part, so `example.com` rather than `https://example.com/` or "+
					"`user@example.com`. Reported by Jamf Account: "+detail.Description,
			)
		case codeFieldValidation:
			diags.AddAttributeError(
				path.Root("domain"),
				"Domain is required",
				"Jamf Account rejected the claim because no domain name reached it. This usually means a "+
					"variable or a reference resolved to an empty string. Reported by Jamf Account: "+
					detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// sharedDomainExplanation is the one fact both shared-domain diagnostics rest
// on, stated once so the two cannot drift apart.
const sharedDomainExplanation = "A shared domain is owned by whichever Jamf Account organization claimed it: " +
	"yours may assign it to an SSO connection, but Jamf Account refuses to change or withdraw it."

// appendSharedDomainDiagnostics refuses a domain another organization owns, and
// reports whether it did.
//
// The domain collection returns the domains an organization has claimed and the
// domains shared with it, in one undifferentiated list, and a shared entry looks
// like an owned one in every field a practitioner would think to check. Letting
// one into state as a managed resource buys an entry that can never leave it:
// every attribute is RequiresReplace and the destroy half of a replacement is a
// withdrawal Jamf refuses, so the operator is left running `terraform state rm`
// by hand. The refusal therefore lands where the record is first seen — the read
// that follows an import, and an ordinary refresh of a domain the owning
// organization shared in after Terraform adopted it.
//
// The diagnostic attaches to `domain` because the name is the only value the
// operator supplied, and it names the owning account so the surprise is
// attributable rather than a bare "this is shared".
func appendSharedDomainDiagnostics(diags *diag.Diagnostics, d *account.Domain) bool {
	if d == nil || !d.SharedDomain {
		return false
	}
	diags.AddAttributeError(
		path.Root("domain"),
		"Domain is shared, not owned",
		"The domain \""+d.Domain+"\" is owned by Jamf Account organization \""+d.AccountID+"\" and shared with "+
			"yours, so jamfplatform_account_sso_domain cannot manage it. "+sharedDomainExplanation+" Read it with "+
			"the `jamfplatform_account_sso_domain` data source instead, which reports a shared domain and the "+
			"connections it is assigned to without claiming to own it. If Terraform already holds it, run "+
			"`terraform state rm` on the address to drop it.",
	)
	return true
}

// appendSharedDomainDeleteDiagnostics refuses to withdraw a shared domain.
//
// This is keyed on the `shared` value already in state rather than on a wire
// refusal, which makes it deterministic and needs no guess about the code Jamf
// answers a cross-organization withdrawal with. Read refuses a shared domain
// before it can reach state at all, so this covers the state a provider version
// without that guard could have written, and it is checked before the request so
// no cross-organization delete is ever issued.
func appendSharedDomainDeleteDiagnostics(diags *diag.Diagnostics, domain string) {
	diags.AddError(
		"Shared Jamf Account SSO domain cannot be withdrawn",
		"Terraform holds the domain \""+domain+"\" as a managed claim, but Jamf Account reports it as shared "+
			"into your organization rather than claimed by it. "+sharedDomainExplanation+" Nothing Terraform can "+
			"send will withdraw it, so no request was made. Run `terraform state rm` on this address to drop it "+
			"from state, and read the domain with the `jamfplatform_account_sso_domain` data source if you still "+
			"need its values.",
	)
}

// findDomain returns the claim whose name matches domain, or nil.
//
// The comparison is case-insensitive because Jamf lower-cases a domain when it
// stores it, so a claim made or imported in mixed case has to match the stored
// spelling. The resource's own validator already refuses a non-lower-case
// configuration, which leaves import as the path this actually serves — a
// practitioner typing `terraform import … Example.com` finds the claim rather
// than being told it does not exist.
func findDomain(domains []account.Domain, domain string) *account.Domain {
	for i := range domains {
		if strings.EqualFold(domains[i].Domain, domain) {
			return &domains[i]
		}
	}
	return nil
}

// numberValueOrNull renders one of Jamf Account's numeric identifiers as a state
// string.
//
// The identifiers arrive as *json.Number because Jamf's own schema calls them
// strings while the values it sends are bare numbers; the SDK decodes either
// form. Terraform state always holds an ID as a string, per STYLE_GUIDE §ID type
// handling, and the value is never sent back, so the decimal text is carried
// through verbatim rather than being parsed into an integer and reformatted.
func numberValueOrNull(n *json.Number) types.String {
	if n == nil || n.String() == "" {
		return types.StringNull()
	}
	return types.StringValue(n.String())
}

// timeValueOrNull renders an optional timestamp in RFC 3339, the form every other
// timestamp in this provider takes.
func timeValueOrNull(v *time.Time) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(v.UTC().Format(time.RFC3339))
}

// verificationTXTRecord assembles the complete TXT record value from the
// verification key, or null when Jamf has minted no key.
func verificationTXTRecord(key string) types.String {
	if key == "" {
		return types.StringNull()
	}
	return types.StringValue(verificationTXTRecordPrefix + key)
}
