// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"fmt"
	"net/http"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/egressip"
)

// EgressIPLookupURL is named in the diagnostic as the command to run when the
// lookup itself could not reach the echo service.
const EgressIPLookupURL = egressip.LookupURL

// egressIPLookup reports this host's public source address, indirected so the
// tests below assert on the rendered guidance without network access. Caching
// lives in egressip.Lookup, so the burst an edge block would otherwise cause is
// already handled there.
//
// It calls through rather than copying the function value: a copy is taken at
// package initialisation, so a test in another package that binds
// egressip.Lookup to a stub would still reach the real echo service here — and
// pass, since the fallback wording carries the address line too, leaving the
// network call to go unnoticed.
var egressIPLookup = func() string { return egressip.Lookup() }

// edgeBlockPreamble states what happened, in the order an operator needs it:
// what answered, that the request never arrived, and that nothing changed. The
// last clause matters most, because an apply that failed part-way is the first
// thing they will worry about.
const edgeBlockPreamble = "A CDN, firewall or IP allowlist answered this request. It never reached the Jamf " +
	"API, so nothing changed.\n\n"

// edgeBlockKnownAddress is used when the lookup succeeded, which is the common
// case: the echo service is not the host being blocked, so a Jamf-side
// allowlist refusing this caller does not stop it answering.
const edgeBlockKnownAddress = "Egress IP address: %s\n\n" + edgeBlockSteps

// edgeBlockUnknownAddress is the fallback. It prints the command rather than
// omitting the address, because an operator who cannot reach the echo service
// from this host can still run it from somewhere sharing the same egress.
const edgeBlockUnknownAddress = "Egress IP address: run `curl -s " + EgressIPLookupURL + "`\n\n" + edgeBlockSteps

// edgeBlockSteps is the action, one step per party who can take it.
//
// Two steps because the page could have come from either end and the summary
// above cannot tell the operator which. Their own proxy or firewall
// intercepting the request is theirs to find; a Jamf-side allowlist refusing
// this address is not something they can inspect, so handing the address over is
// the whole of their part. An earlier draft told them to "check whether it is
// allowed", which is the half they have no way to do.
const edgeBlockSteps = "1. Ask your network team whether anything intercepts outbound traffic to the Jamf API.\n" +
	"2. Provide the address above, the status and the request id to Jamf Support."

// gatewayFailureGuidance covers a 5xx page, which needs the opposite remedy to a
// block: the SDK has already retried it, so the answer is to run again rather
// than to go hunting for an allowlist. The split is the one the SDK's godoc asks
// consumers to make.
const gatewayFailureGuidance = "The gateway answered with an error page. The request never reached the Jamf API " +
	"and nothing changed.\n\n" +
	"The provider retried and got the same page, so the fault is on the Jamf side. Re-run the command in " +
	"a few minutes."

// APIErrorDetail renders err as the detail of a Terraform diagnostic, appending
// what to do about it when the response came from something other than Jamf.
//
// Every error rendered into a diagnostic detail goes through here rather than
// through err.Error(), and that is wider than the name: it is a no-op for an
// error any Jamf service produced, which is nearly all of them, and equally a
// no-op for one no service produced at all — a base64 or JSON decode of an
// operator attribute, a version that would not parse. Only the API case is
// named because only the API case gains anything; the rest pass through because
// nothing structural separates them at a call site, so the alternative to
// wrapping them is a per-site exemption, which is the escape hatch the
// conformance guard exists to close.
//
// Applied uniformly rather than at the call sites judged likely to see a block:
// which call hits an edge is not a property of the resource, and the two
// failures that prompted this — CloudFront 502 and 504 pages on CI runs
// 34471595582, 34202213638 and 34128074623 — landed on creates, the operations
// a narrower sweep would have skipped.
//
// The classification is IsEdgeBlocked's, and so the SDK's. What the operator
// sees without this is a condensed one-line page summary carrying a status and
// an edge request id — accurate, and no help at all in deciding whether to
// re-run, check an allowlist, or look for a mistake in the configuration.
func APIErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	if !IsEdgeBlocked(err) {
		return err.Error()
	}

	if apiErr := jamfplatform.AsAPIError(err); apiErr != nil && apiErr.StatusCode >= http.StatusInternalServerError {
		return err.Error() + "\n\n" + gatewayFailureGuidance
	}

	address := edgeBlockUnknownAddress
	if ip := egressIPLookup(); ip != "" {
		address = fmt.Sprintf(edgeBlockKnownAddress, ip)
	}
	return err.Error() + "\n\n" + edgeBlockPreamble + address
}
