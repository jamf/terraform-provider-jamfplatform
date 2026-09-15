// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes Jamf Security Cloud returns on the DNS zone
// endpoints. Wire-probed against production EU on 2026-08-27; each one is
// translated into a diagnostic attached to the attribute that caused it, because
// the raw message names the code and not the fix.
// Every code this resource translates is in the SDK's generated
// `ApiErrorItemCode` enum, which the DNS namespace declares in its spec schema,
// so all six are taken from there rather than restated as literals.
const (
	codeDomainConflict         = securitycloud.ApiErrorItemCodeDomainConflict
	codeGatewayNotFound        = securitycloud.ApiErrorItemCodeGatewayNotFound
	codeNameServerIPRestricted = securitycloud.ApiErrorItemCodeNameserverIpRestricted
	codeNameServerIPOutOfRange = securitycloud.ApiErrorItemCodeNameserverIpOutOfRange
	codeListSizeExceeded       = securitycloud.ApiErrorItemCodeListSizeExceeded
	codeNotEntitled            = securitycloud.ApiErrorItemCodeNotEntitled
)

// appendWriteDiagnostics turns a create/update failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// The codes worth translating are the ones whose cause is not the field the
// server names. GATEWAY_NOT_FOUND is the clearest case: the request that fails
// is a zone write, but the thing that does not exist is a gateway, so the
// diagnostic has to point at `authoritative_name_servers` — a zone cannot be
// created before the gateway its name servers are reachable through.
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
		case codeDomainConflict:
			diags.AddAttributeError(
				path.Root("domains"),
				"Domain already claimed by another DNS zone",
				"Jamf Security Cloud allows a domain to belong to only one custom DNS zone. Remove the domain from "+
					"the zone that already holds it, or drop it from this one. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		case codeGatewayNotFound:
			diags.AddAttributeError(
				path.Root("authoritative_name_servers"),
				"Referenced gateway not found",
				"One of this zone's name servers names a gateway that does not exist in Jamf Security Cloud. The "+
					"gateway must exist before the zone can reference it — check `gateway_id` against your ZTNA "+
					"gateways and the Jamf-managed shared gateways. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		case codeNameServerIPRestricted, codeNameServerIPOutOfRange:
			diags.AddAttributeError(
				path.Root("authoritative_name_servers"),
				"Name server IP address not allowed",
				"Jamf Security Cloud refuses this name server address. Reserved ranges — private, loopback and "+
					"similar — are not accepted. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeListSizeExceeded:
			diags.AddError(
				"DNS zone collection size out of range",
				"A collection on this zone is outside the size Jamf Security Cloud accepts: `domains` takes 1 to "+
					"100 entries and `authoritative_name_servers` takes 1 to 20. There is also a per-tenant cap "+
					"on the number of zones. Reported by Jamf Security Cloud: "+detail.Description,
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
