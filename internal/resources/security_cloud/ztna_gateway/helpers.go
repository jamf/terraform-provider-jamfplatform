// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Machine-readable error codes the ZTNA gateway endpoints return. Wire-probed
// against production EU on 2026-08-27, except codeDedicatedIPsLimit, which is
// taken from the spec's shared 409 catalogue (SDK spec v1807).
//
// Codes the SDK does not carry as constants. Only the DNS namespace declares its
// error codes in a schema enum, so `securitycloud.ApiErrorItemCode*` covers that
// vocabulary and nothing else — the ZTNA codes below appear in the spec only as
// response examples, which the generator does not emit. Referenced from the SDK
// wherever it has the constant, and declared here where it does not.
const (
	codeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled

	codeGatewayTypeChangeNotSupported = "GATEWAY_TYPE_CHANGE_NOT_SUPPORTED"
	codeIPSecSecretClearNotSupported  = "IPSEC_SECRET_CLEAR_NOT_SUPPORTED"
	codeDedicatedIPsLimit             = "DEDICATED_IPS_LIMIT"
	codeBadRequest                    = "BAD_REQUEST"

	codeReferencedByAccessPolicies  = "GATEWAY_REFERENCED_BY_ACCESS_POLICIES"
	codeReferencedByDNSZones        = "GATEWAY_REFERENCED_BY_DNS_ZONES"
	codeReferencedByGroupedGateways = "GATEWAY_REFERENCED_BY_GROUPED_GATEWAYS"
)

// appendWriteDiagnostics turns a create/update failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// Three of the four codes worth translating share a property: the message names
// a mechanism rather than a fix. `GATEWAY_TYPE_CHANGE_NOT_SUPPORTED` should not
// normally be reachable — the `ipsec` block's plan modifier replaces the gateway
// instead — so seeing it means something got past that, and the diagnostic says
// as much. `BAD_REQUEST` "No mapping found for one of the supplied ids" is
// exclusively about `tenant_ids`, and says nothing about which id or why.
//
// `DEDICATED_IPS_LIMIT` is the odd one out: nothing in the configuration is
// wrong, the account has simply used every dedicated IP address it is allotted.
// It gets no attribute path because the addresses are provisioned by Jamf and
// surface as the computed `dedicated_egress_ip_addresses` — there is no input to
// point at, and pointing at one would imply an edit that cannot fix it.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeGatewayTypeChangeNotSupported:
			diags.AddError(
				"Gateway form cannot be changed in place",
				"Jamf Security Cloud does not convert a dedicated IPsec gateway into a dedicated internet gateway "+
					"or back. The gateway has to be replaced. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeIPSecSecretClearNotSupported:
			diags.AddAttributeError(
				path.Root("ipsec").AtName("jamf_side").AtName("authentication_secret"),
				"IPsec pre-shared key cannot be cleared",
				"The pre-shared key can be rotated but never removed. Supply a new `authentication_secret` and "+
					"bump `authentication_secret_wo_version`, or leave both alone to keep the stored key. Reported by Jamf "+
					"Security Cloud: "+detail.Description,
			)
		case codeDedicatedIPsLimit:
			diags.AddError(
				"Dedicated IP address limit reached",
				"This account has no dedicated IP addresses left to assign, so Jamf Security Cloud cannot "+
					"provision another dedicated gateway. Nothing in this configuration is wrong: destroy a "+
					"dedicated gateway you no longer need, or contact Jamf to raise the allotment. Reported by "+
					"Jamf Security Cloud: "+detail.Description,
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

// appendDeleteDiagnostics explains the one delete failure that is not a mistake
// in the configuration but an ordering problem in the plan.
//
// Jamf Security Cloud refuses to delete a gateway that anything still references,
// with a `409`. Terraform will happily plan that destroy when the config dependency
// edge disappears in the same apply that removes the reference, so this is worth
// spelling out rather than passing through. In every case the remedy is the same
// two-apply sequence: drop the reference, apply, then destroy the gateway.
//
// All three referrers name themselves. Wire-probed against production EU on
// 2026-08-30 by creating a dedicated gateway, pointing each referrer at it in turn
// and deleting it: an access policy answers GATEWAY_REFERENCED_BY_ACCESS_POLICIES, a
// custom DNS zone name server GATEWAY_REFERENCED_BY_DNS_ZONES, and grouped-gateway
// membership GATEWAY_REFERENCED_BY_GROUPED_GATEWAYS, each with a description naming
// the referrer class and the fix. Releasing the reference and repeating the delete
// answered 204 in all three, so the reference is the sole cause. That is why each
// code gets its own diagnostic naming the one thing to go and look at, rather than
// the operator being handed all three possibilities.
//
// The generic fallback stays, and is not dead code. The 2026-08-27 probe of the zone
// and grouped-gateway cases recorded a bare 409 carrying no structured detail, which
// today's probe contradicts — whether that was a misread then or a server change
// since, an endpoint that has been seen to answer both shapes gets code-keyed
// diagnostics for the shape that carries a code and the three-way explanation for the
// shape that does not.
//
// A ZTNA access policy is a reference Terraform manages: the application's
// `routing.gateway_id` and `routing_overrides[].routing.gateway_id` each name a
// gateway, so moving an application between gateways and destroying the old one in a
// single apply is exactly the ordering trap above. Only an access policy created
// outside Terraform is beyond its reach, and then the reference does live in the
// admin UI.
//
// None of the three codes is an SDK constant: the generated `ApiErrorItemCode` enum
// is declared from the DNS schema, and these appear in the ZTNA spec only as response
// examples, which the generator does not emit. enum_literals_test.go exempts each by
// name so an SDK release that starts generating one fails the guard.
func appendDeleteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusConflict) {
		return false
	}
	for _, detail := range apiErr.Details() {
		summary, referrer, ok := referencedByDetail(detail.Code)
		if !ok {
			continue
		}
		diags.AddError(
			summary,
			"Jamf Security Cloud will not delete a gateway while "+referrer+" still points at it. Remove the "+
				"reference first in a separate apply, then destroy the gateway: dropping the reference and the "+
				"gateway in one apply lets Terraform sequence the destroy before the update that would have "+
				"released it."+reportedDetails(apiErr),
		)
		return true
	}
	diags.AddError(
		"Gateway is still referenced",
		"Jamf Security Cloud refuses to delete a gateway that something still points at: a ZTNA access policy, a "+
			"custom DNS zone name server, or membership of a grouped gateway. It did not say which, so check each "+
			"in turn. Access policies created outside Terraform are invisible to it, so check the Jamf Security "+
			"Cloud admin UI if no `jamfplatform_security_cloud_ztna_app`, zone or grouped gateway in your "+
			"configuration names this gateway. For a reference Terraform does manage, remove it first in a "+
			"separate apply, then destroy the gateway: dropping the reference and the gateway in one apply lets "+
			"Terraform sequence the destroy before the update that would have released it."+reportedDetails(apiErr),
	)
	return true
}

// referencedByDetail maps a referenced-by error code to its diagnostic summary and
// the phrase naming what holds the reference, reporting whether the code is one of
// the three.
func referencedByDetail(code string) (summary, referrer string, ok bool) {
	switch code {
	case codeReferencedByAccessPolicies:
		return "Gateway is still referenced by an access policy application",
			"an access policy application — a `jamfplatform_security_cloud_ztna_app` whose " +
				"`routing.gateway_id` or `routing_overrides[].routing.gateway_id` names this gateway, or one " +
				"created outside Terraform, which appears only in the admin UI —", true
	case codeReferencedByDNSZones:
		return "Gateway is still referenced by a custom DNS zone",
			"a custom DNS zone name server — a `jamfplatform_security_cloud_dns_zone` whose " +
				"`authoritative_name_servers[].gateway_id` names this gateway —", true
	case codeReferencedByGroupedGateways:
		return "Gateway is still a member of a grouped gateway",
			"a grouped gateway — a `jamfplatform_security_cloud_ztna_grouped_gateway` whose `gateway_ids` " +
				"includes this gateway —", true
	}
	return "", "", false
}

// reportedDetails renders whatever structured detail an error carries, for the
// diagnostics that cannot assume one is present.
//
// The delete conflict is the case: the probed referrer cases answered with a bare
// 409, while the spec documents per-referrer codes. Appending what is there costs
// nothing when there is nothing, and stops the diagnostic contradicting the body
// if the endpoint starts sending one.
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

// gatewayStatusPollInterval is how often the readiness wait re-reads the gateway.
//
// Five seconds against a wait measured in minutes costs at most a couple of
// hundred reads spread across it, well inside this provider's ~10 req/s budget
// for the Jamf API. It also bounds how precisely the wait can observe anything:
// see waitForGatewayState on the create-side address figure.
const gatewayStatusPollInterval = 5 * time.Second

// gatewayDownDwell is how long a gateway must report DOWN continuously before a wait
// for UP abandons it.
//
// DOWN is the steady state of an enabled IPsec gateway whose customer-side
// concentrator is not answering, and that is the normal condition immediately after
// creating one: the Jamf side is built first, precisely so its pre-shared key and
// source addresses can be used to configure the other end. Waiting the whole budget
// out there would hang an apply for ten minutes to report something the operator
// already knows.
//
// DOWN still cannot simply be treated as terminal. Whether a gateway can report DOWN
// briefly and then reach UP — a tunnel taking a little longer to establish than Jamf
// takes to first evaluate it — is unmeasured, and testing it needs a real
// concentrator. Requiring DOWN to persist sidesteps the question rather than guessing
// at it: a transient DOWN that recovers inside the dwell still ends with the wait
// seeing UP, and only a gateway that stays down is abandoned.
//
// A minute is the balance struck. Establishing an IPsec tunnel is a seconds-scale
// negotiation once both ends are configured and reachable, so a minute is well beyond
// any plausible recovery, while keeping the common case to about two minutes of apply
// rather than ten.
//
// Two minutes rather than one, because the dwell begins when DOWN is first *seen*, not
// when the wait starts. An update re-provisions the gateway, so it returns to PENDING,
// spends 35 to 50 seconds coming back to DOWN, and only then starts dwelling. Measured
// end to end on 2026-08-31: an enabled IPsec update against an unbuilt tunnel took
// 124s to give up and warn. Anyone tuning this should size it against that total, not
// against the constant alone.
//
// The minute itself is reasoning rather than measurement, so it is worth revisiting if
// a gateway is ever seen recovering from DOWN more slowly than this — the cost of being
// wrong is a warning and a status that corrects itself on the next refresh, not a
// failed apply.
const gatewayDownDwell = 60 * time.Second

// gatewayAbandonState reports which status ends a wait early when it persists, for a
// given target.
//
// Only a wait for UP *alone* has one. Where DOWN is itself an accepted answer — an
// enabled IPsec create — there is nothing to abandon, because arriving at DOWN ends
// the wait successfully. DOWN is where an enabled IPsec gateway settles when its
// tunnel is not up, so a wait for UP that sees DOWN dwelling has almost certainly been
// given a gateway whose other end is not built yet. A wait for DISABLED must not treat
// DOWN that way: disabling the 2026-08-31 IPsec probe took it from DOWN through
// PENDING to DISABLED, so there DOWN is the state being left rather than the state
// being stuck in.
func gatewayAbandonState(want []string) string {
	if len(want) == 1 && want[0] == securitycloud.GatewayStatusStateUp {
		return securitycloud.GatewayStatusStateDown
	}
	return ""
}

// gatewayReader reads one gateway by ID. securitycloud.Client.GetZtnaGatewayV1
// satisfies it, so the wait takes the method value directly and unit tests
// substitute a closure.
type gatewayReader func(ctx context.Context, id string) (*securitycloud.Gateway, error)

// waitForGatewayState polls the gateway until it reports itself operational, and
// returns the last representation it read, the last state it saw, and whether the
// wait was satisfied.
//
// Why wait at all, and why on this signal. Terraform's contract is that a finished
// apply leaves a usable resource, and a gateway that has not yet reported itself
// operational is not one. The earlier of the two available signals — the dedicated egress
// addresses appearing — was considered and rejected: it leaves state recording a
// provisioning gateway after every apply, and on an update it needs a comparison
// against the prior addresses, because for a window after an egress-region change
// the list is non-empty and holds the *old* region's addresses. Waiting for the
// operational state subsumes that window entirely, so no such comparison exists
// here and none should be reintroduced.
//
// Measured against production EU on 2026-08-31 under tenant scope, one dedicated
// **internet** gateway created in eu-west-1, moved to eu-central-1 and destroyed,
// re-read every 5 seconds:
//
//	Phase                          Egress addresses populated   State reached UP
//	----------------------------   --------------------------   ----------------
//	Create (eu-west-1)             within 6s                    275s
//	Egress region → eu-central-1   35s                          295s
//
// The state went PENDING → UP exactly once per phase, monotonically, and DOWN was
// never observed. The tunnel state stayed null throughout, as it does on the
// internet form. Nothing came near the probe's own 20-minute cap.
//
// A second probe the same day took the other form: a dedicated IPsec gateway whose
// peer address pointed at nothing went PENDING at 5s and DOWN at 35s with its
// tunnel state DOWN, and stayed there. It never reached UP, and its dedicated
// egress addresses stayed empty throughout — which is the schema's "always empty on
// an IPsec gateway", now measured rather than asserted. That probe is what makes the
// internet-only gate in gatewayWaitTarget a measurement rather than a caution.
//
// UP is what the gateway reports about itself, and that is all this waits for. It is
// necessary for traffic to flow and not sufficient: a Jamf support case on
// grouped-gateway failover records a tunnel reporting itself established while
// passing 0 B/s. So nothing here — and nothing in the diagnostics or descriptions —
// should claim a gateway that reached UP is confirmed usable.
//
// Two caveats on those numbers. The create-side address figure is bounded by the
// 5-second poll interval — the first re-read already showed them, so the true
// value is somewhere in 0–6s, not 6s. And this is a single sample from one EU
// tenant and one region pair: it sizes the default budget, it does not promise a
// duration.
//
// DOWN is deliberately not terminal. The internet-form probe never produced it, so
// whether it can appear transiently mid-provisioning on that form is unknown;
// treating it as terminal on that ignorance would fail an apply that was about to
// succeed. The IPsec probe's settled DOWN does not change that, because the IPsec
// form is never waited on in the first place. Only the caller's context budget ends
// the wait.
//
// A read failure ends the wait rather than being swallowed, matching
// waitForBenchmarkSync in internal/resources/cbengine/benchmark. The caller reads
// the gateway again afterwards, so a persistent failure still produces a real
// error diagnostic there rather than being hidden behind a full-budget spin.
//
// jamfplatform.PollUntil calls its checker before it ever sleeps, which is what
// makes an already-operational gateway free: one read, no delay. That is what lets
// the update path wait unconditionally instead of trying to work out whether the
// change re-provisions anything.
func waitForGatewayState(ctx context.Context, read gatewayReader, id string, want []string, interval time.Duration, abandonState string, abandonAfter time.Duration) (observed *securitycloud.Gateway, lastState string, reached bool) {
	var abandonedAt time.Time
	var dwellStart time.Time
	err := jamfplatform.PollUntil(ctx, interval, func(pollCtx context.Context) (bool, error) {
		got, err := read(pollCtx, id)
		if err != nil {
			tflog.Debug(pollCtx, "polling Jamf Security Cloud ZTNA gateway failed", map[string]any{"id": id, "error": err.Error()})
			return false, err
		}
		observed = got
		state := ""
		if got.Status != nil {
			state = got.Status.State
		}
		if state != "" {
			lastState = state
		}
		tflog.Debug(pollCtx, "Jamf Security Cloud ZTNA gateway state", map[string]any{"id": id, "state": state})
		if slices.Contains(want, state) {
			return true, nil
		}
		if abandonState == "" || state != abandonState {
			dwellStart = time.Time{}
			return false, nil
		}
		now := time.Now()
		if dwellStart.IsZero() {
			dwellStart = now
			return false, nil
		}
		if now.Sub(dwellStart) < abandonAfter {
			return false, nil
		}
		tflog.Debug(pollCtx, "abandoning wait for Jamf Security Cloud ZTNA gateway", map[string]any{
			"id": id, "state": state, "dwelt_for": now.Sub(dwellStart).String(),
		})
		abandonedAt = now
		return true, nil
	})
	return observed, lastState, err == nil && abandonedAt.IsZero()
}

// gatewayWaitTarget reports which settled statuses would end an apply's wait, and
// whether to wait at all.
//
// The principle throughout is that an apply records a *settled* status, never the
// PENDING every transition drifts through for a few seconds. What differs per
// configuration is which settled status is actually within reach at that moment.
//
// A disabled gateway settles at DISABLED, whatever its form and whichever operation
// is running — disabling is a Jamf-side change that owes nothing to any tunnel.
// Measured 2026-08-31: 12s on a dedicated internet gateway, and 30s on an IPsec one
// taken from DOWN through PENDING to DISABLED.
//
// An enabled gateway normally settles at UP, measured at 275s to 321s across three
// creates and one enable. A DOWN dwelling past gatewayDownDwell releases that wait —
// see gatewayAbandonState.
//
// **Creating** an enabled IPsec gateway is the one case with two acceptable answers,
// because UP is not reachable and DOWN is. The tunnel's other end is built from values
// the create returns: Jamf's Quick Connect procedure creates the gateway in step 1 and
// spends steps 2 through 7 building a Linux VM, installing strongSwan, configuring NAT
// and opening the firewall, and the Custom IPSec flow has the same shape — a Jamf
// support case has a gateway reporting DOWN until the Jamf-side subnet reached the
// customer's AWS config, then UP. So the settled status here is DOWN, reached in 35 to
// 50 seconds, and waiting for UP alone would spend the whole budget on something the
// operator cannot yet have done.
//
// DOWN alone would be just as wrong in the other direction. Everything the far end
// needs is caller-supplied through this resource — the pre-shared key, the subnets,
// the source addresses — so an operator who has pre-staged their concentrator can have
// a gateway go straight from PENDING to UP without ever reporting DOWN, and a wait for
// DOWN alone would then never be satisfied. Accepting either is what makes the wait
// reachable in both cases; in effect it waits for the status to leave PENDING.
//
// An unknown `enabled` waits for nothing, having no target to aim at. A null one is
// treated as enabled, matching the schema default — that default is what stops a null
// ever arriving here, and TestGatewayWaitTarget_DependsOnTheEnabledDefault fails if it
// is removed or flipped, because the two have to agree.
func gatewayWaitTarget(plan *GatewayResourceModel, op gatewayWaitOperation) (want []string, wait bool) { //nolint:revive // named results document the pair
	if plan.Enabled.IsUnknown() {
		return nil, false
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.ValueBool() {
		return []string{securitycloud.GatewayStatusStateDisabled}, true
	}
	if plan.IPSec != nil && op.isCreate {
		return []string{securitycloud.GatewayStatusStateUp, securitycloud.GatewayStatusStateDown}, true
	}
	return []string{securitycloud.GatewayStatusStateUp}, true
}

// gatewayWaitOperation names the apply phase a readiness wait ran under, for the
// warning issued when it runs out: the verb to describe what already succeeded,
// and the `timeouts` attribute that extends the budget.
type gatewayWaitOperation struct {
	pastTense   string
	timeoutAttr string
	isCreate    bool
}

var (
	gatewayWaitCreate = gatewayWaitOperation{pastTense: "created", timeoutAttr: "create", isCreate: true}
	gatewayWaitUpdate = gatewayWaitOperation{pastTense: "updated", timeoutAttr: "update"}
)

// lastStateOrUnknown renders the state a wait ended on for a diagnostic, standing in
// a readable word where the status could not be read at all rather than leaving an
// empty pair of quotes in the operator's terminal.
func lastStateOrUnknown(lastState string) string {
	if lastState == "" {
		return "an unknown status"
	}
	return lastState
}

// appendGatewayWaitWarning reports a readiness wait that ran out, as a warning.
//
// A warning and never an error. By the time the wait is exhausted the gateway
// exists and is billable; an error returned from a create the server accepted
// makes Terraform mark the resource tainted and destroy and recreate it on the
// next apply, so the failure mode of being strict here is a paid resource
// discarded and replaced for being slow.
//
// The status reached is named because the three cases mean different things and
// call for different responses. Still provisioning is almost always just slow and
// settles on a later refresh. Unreachable or degraded is a plausible real fault
// and is worth investigating rather than waiting out. Anything else is unexpected
// enough to be worth repeating verbatim rather than paraphrasing — including the
// case where the status could not be read at all.
//
// State values come from the SDK's generated constants rather than restated
// literals, per STYLE_GUIDE §"Enum values and error codes come from the SDK".
func appendGatewayWaitWarning(diags *diag.Diagnostics, op gatewayWaitOperation, want []string, lastState string) {
	if slices.Contains(want, securitycloud.GatewayStatusStateDisabled) {
		diags.AddWarning(
			"Gateway is not yet reported as disabled",
			"The gateway was "+op.pastTense+" successfully and Terraform has recorded it as disabled. Jamf "+
				"Security Cloud had not caught up by the time Terraform stopped waiting — it still reports "+
				"\""+lastStateOrUnknown(lastState)+"\". A gateway takes a few seconds to settle after being "+
				"disabled, so this is usually only a timing difference and the next refresh corrects the "+
				"recorded status. To wait longer during the apply itself, raise `"+op.timeoutAttr+"` in this "+
				"resource's `timeouts` block.",
		)
		return
	}

	var summary, cause string
	switch lastState {
	case securitycloud.GatewayStatusStatePending:
		summary = "Gateway is still provisioning"
		cause = "Terraform stopped waiting before Jamf Security Cloud finished provisioning it. That usually " +
			"just means provisioning is taking longer than it normally does, though it can also mean it has " +
			"stalled."
	case securitycloud.GatewayStatusStateDown:
		summary = "Gateway is unreachable or degraded"
		cause = "Jamf Security Cloud reports the gateway as unreachable or degraded rather than still " +
			"provisioning. That is more likely a fault than slowness, so look at the gateway in the Jamf " +
			"Security Cloud admin UI rather than waiting it out."
	case "":
		summary = "Gateway status could not be read"
		cause = "Terraform could not read the gateway's status while waiting for it to become operational, so " +
			"whether it is ready is unknown."
	default:
		summary = "Gateway is not yet operational"
		cause = "Jamf Security Cloud reports its status as \"" + lastState + "\", which is not a status the " +
			"provider expects while a gateway is coming up."
	}
	diags.AddWarning(
		summary,
		"The gateway was "+op.pastTense+" successfully and Terraform has recorded it. "+cause+" It does not yet "+
			"report itself operational, so do not assume it is carrying traffic. Nothing needs to be "+
			"re-applied: the status settles on a later refresh, so the next `terraform plan` picks it up. To "+
			"wait longer during the apply itself, raise `"+op.timeoutAttr+"` in this resource's `timeouts` "+
			"block.",
	)
}

// readBackGateway returns the gateway representation a write path should record,
// reusing whatever the readiness wait already read.
//
// The wait reads the gateway on every tick, so its last successful read is as
// fresh as anything a further read could produce — and, importantly, it is still
// available when the wait exhausted the caller's context budget, at which point a
// further read on that same context could only fail. Reusing it is therefore not
// an optimisation: it is what keeps an exhausted wait a warning on a successful
// apply rather than an error on a gateway that already exists and is billable.
//
// When no wait ran — an IPsec or disabled gateway — or when it ended before it
// managed a single read, the gateway is read here instead.
func readBackGateway(ctx context.Context, read gatewayReader, id string, observed *securitycloud.Gateway) (*securitycloud.Gateway, error) {
	if observed != nil {
		return observed, nil
	}
	return read(ctx, id)
}
