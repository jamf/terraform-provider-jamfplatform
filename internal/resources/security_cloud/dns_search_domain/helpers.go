// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes Jamf Security Cloud returns on the search domain
// endpoint. Wire-probed against production EU on 2026-08-29. Both are in the SDK's
// generated ApiErrorItemCode enum, which the DNS namespace declares in its spec
// schema, so both are taken from there rather than restated as literals — checked
// individually rather than assumed from the set, per STYLE_GUIDE §Enum values and
// error codes come from the SDK.
//
// SEARCH_DOMAIN_NOT_SET is deliberately not one of the codes appendWriteDiagnostics
// translates. It is the read path's "nothing configured" answer, carried on a 404,
// and turning it into a write diagnostic would make an ordinary empty state an
// error. It is aliased all the same, because Delete matches on it exactly — see
// isSearchDomainNotSet.
const (
	codeInvalidField       = securitycloud.ApiErrorItemCodeInvalidField
	codeNotEntitled        = securitycloud.ApiErrorItemCodeNotEntitled
	codeSearchDomainNotSet = securitycloud.ApiErrorItemCodeSearchDomainNotSet
)

// isSearchDomainNotSet reports whether err is the endpoint's documented
// "nothing configured" answer: a failure carrying SEARCH_DOMAIN_NOT_SET.
//
// Delete needs this narrow test rather than helpers.IsNotFoundError, which matches
// on status alone. Clearing answers 204 whether or not a search domain was set, so
// any 404 on that path is unexpected — including the gateway's own bare "page not
// found" for a route it does not serve, which carries no code at all. Tolerating
// that as "already cleared" would drop the resource from Terraform state while the
// search domain stayed live on the tenant.
func isSearchDomainNotSet(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	for _, detail := range apiErr.Details() {
		if detail.Code == codeSearchDomainNotSet {
			return true
		}
	}
	return false
}

// appendWriteDiagnostics turns a write failure into the most specific diagnostic
// the error body supports, and reports whether it recognised one.
//
// INVALID_FIELD is worth translating even though the plan-time validator should
// have caught the malformed cases first: reaching apply means the validator and the
// server disagree about the accepted grammar, and a bare "Invalid field value."
// with a null field gives an operator nothing to act on. Naming the attribute at
// least says where to look.
//
// NOT_ENTITLED is the other one worth naming: the credentials are valid and the
// tenant simply does not have the surface, which is invisible in a bare 403.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeInvalidField:
			diags.AddAttributeError(
				path.Root("domain_name"),
				"Search domain not accepted",
				"Jamf Security Cloud refuses this search domain. It must be 1 to 253 characters, each "+
					"dot-separated label at most 63, with no wildcards and a final label that is not entirely "+
					"numeric. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeNotEntitled:
			diags.AddError(
				"Tenant not entitled to Jamf Security Cloud custom DNS",
				"The credentials authenticated successfully but this tenant does not have the custom DNS surface "+
					"enabled. Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}
