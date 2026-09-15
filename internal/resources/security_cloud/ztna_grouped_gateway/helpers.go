// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes the grouped-gateway endpoints return. Wire-probed
// against production EU on 2026-08-27, except codeGatewayNotFound, which is
// taken from the spec's shared 422 catalogue (SDK spec v1807).
//
// Codes the SDK does not carry as constants. Only the DNS namespace declares its
// error codes in a schema enum, so `securitycloud.ApiErrorItemCode*` covers that
// vocabulary and nothing else — the ZTNA codes below appear in the spec only as
// response examples, which the generator does not emit. Referenced from the SDK
// wherever it has the constant, and declared here where it does not.
const (
	codeGatewayNotFound = securitycloud.ApiErrorItemCodeGatewayNotFound
	codeNotEntitled     = securitycloud.ApiErrorItemCodeNotEntitled

	codeMixedTunnelTypes    = "MIXED_TUNNEL_TYPES"
	codeMixedDedicatedIPs   = "MIXED_DEDICATED_IPS_TYPES"
	codeSharedGatewayMember = "SHARED_GATEWAY_MEMBER"
	codeBadRequest          = "BAD_REQUEST"
)

// appendWriteDiagnostics turns a create/update failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// The membership codes are the ones worth translating, because each describes a
// property of the *members* rather than of the group being written, and none of
// the messages says which member is at fault.
//
// `MIXED_DEDICATED_IPS_TYPES` is the one that most needs it: the server's message
// names a wire field that has no counterpart on this schema and none on the
// gateway resource either, because a gateway's form is derived from whether it
// declares an `ipsec` block rather than set by a flag. The translation therefore
// names the block, and the gateway data source as the way to read the form of a
// member this configuration does not manage.
//
// `GATEWAY_NOT_FOUND` is the same ordering trap the DNS zone has for its name
// servers: a member gateway must exist before the group can name it, and
// Terraform will plan the group's create alongside the member's when the
// dependency edge is only implicit. It also covers a gateway that exists but
// belongs to another customer, which the spec folds into the same code, so the
// diagnostic names both readings rather than asserting the id is absent.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeMixedTunnelTypes:
			diags.AddAttributeError(
				path.Root("gateway_ids"),
				"Member gateways have different forms",
				"Every member of a grouped gateway must be the same form — all dedicated IPsec gateways, or all "+
					"dedicated internet gateways. A gateway is an IPsec gateway when it has an `ipsec` block. "+
					"Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeMixedDedicatedIPs:
			diags.AddAttributeError(
				path.Root("gateway_ids"),
				"Member gateways disagree about dedicated egress IPs",
				"Every member of a grouped gateway must have the same dedicated egress IP setting — either all "+
					"members have them or none does. A gateway's form is set by whether it declares an `ipsec` "+
					"block, so make every member match; for a gateway this configuration does not manage, read its "+
					"form with the `jamfplatform_security_cloud_ztna_gateway` data source. Reported by Jamf "+
					"Security Cloud: "+detail.Description,
			)
		case codeSharedGatewayMember:
			diags.AddAttributeError(
				path.Root("gateway_ids"),
				"Shared gateway cannot be a group member",
				"Members must be your own dedicated gateways. One of these IDs names a Jamf-managed shared gateway "+
					"— those cannot be grouped, and are read with "+
					"`jamfplatform_security_cloud_ztna_shared_gateways`. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		case codeGatewayNotFound:
			diags.AddAttributeError(
				path.Root("gateway_ids"),
				"Member gateway not found",
				"One of the IDs in `gateway_ids` does not name a gateway this account can reach — either it does "+
					"not exist, or it belongs to another customer. Every member must exist before the group can "+
					"reference it, so if the member is managed in the same configuration, make the dependency "+
					"explicit with `depends_on` or by referencing its `id` attribute. Reported by Jamf Security "+
					"Cloud: "+detail.Description,
			)
		case codeBadRequest:
			diags.AddAttributeError(
				path.Root("tenant_ids"),
				"Unknown tenant ID",
				"Jamf Security Cloud could not resolve one of the supplied tenant IDs. Every tenant must belong to "+
					"the same organization as the credentials the provider is configured with. Reported by Jamf "+
					"Security Cloud: "+detail.Description,
			)
		case codeNotEntitled:
			diags.AddError(
				"Tenant not entitled to Jamf Security Cloud ZTNA",
				"The credentials authenticated successfully but this tenant does not have the ZTNA surface enabled. "+
					"Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// appendDeleteDiagnostics explains the delete refusal, which is an ordering
// problem rather than a configuration mistake.
//
// A grouped gateway that something still points at is refused with a
// `409 CONFLICT`. The bundled spec names GROUPED_GATEWAY_REFERENCED_BY_DNS_ZONES
// and GROUPED_GATEWAY_REFERENCED_BY_ACCESS_POLICIES; the 2026-08-27 probe of the
// zone case answered a bare 409 with no structured detail, so the codes are not
// something the wire has been seen to send. Details() is appended when present
// rather than relied on.
//
// Access policies matter most in that list because the provider does not manage
// them: that reference lives in the admin UI, and no apply ordering will release
// it.
func appendDeleteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusConflict) {
		return false
	}
	diags.AddError(
		"Grouped gateway is still referenced",
		"Jamf Security Cloud refuses to delete a grouped gateway that something still points at: a ZTNA access "+
			"policy, or a custom DNS zone name server. Access policies are not managed by this provider, so if no "+
			"zone names this group, check its access policies in the Jamf Security Cloud admin UI. For a reference "+
			"Terraform does manage, remove it first in a separate apply, then destroy the group: dropping the "+
			"reference and the group in one apply lets Terraform sequence the destroy before the update that would "+
			"have released it."+reportedDetails(apiErr),
	)
	return true
}

// reportedDetails renders whatever structured detail an error carries, for the
// diagnostics that cannot assume one is present. See the delete conflict above for
// why this is appended rather than depended on.
func reportedDetails(apiErr *jamfplatform.APIResponseError) string {
	var b strings.Builder
	for _, detail := range apiErr.Details() {
		if detail.Description == "" {
			continue
		}
		b.WriteString(" Reported by Jamf Security Cloud: ")
		b.WriteString(detail.Description)
	}
	return b.String()
}
