// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// actionCreate and actionChange are the operation names the two writes pass
// appendWriteDiagnostics. They are named because the create branch has to be told
// apart: the same internal failure means an operator-fixable problem on a create
// and a fault at Jamf on a change, so the two cannot share one diagnostic. The
// word is also what a diagnostic says out loud, so the call sites and the switch
// have to agree on it.
const (
	actionCreate = "create"
	actionChange = "change"
)

// appendWriteDiagnostics turns a create or change failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// The first case reads one internal failure two ways, because the same code means
// two different things depending on which operation asked. On a create it is an
// overloaded catch-all standing in for several problems an operator can put right
// — an unclaimed or unverified domain, a name carrying more than letters and
// digits, settings that disagree with the declared family, a value that family
// requires and the settings do not carry, and an organization already holding as
// many connections as Jamf allows — so the diagnostic lists them rather than
// telling an operator the fault is Jamf's at the moment their own configuration is
// the problem. On a change it really is Jamf's: every change is refused the same
// way, including one carrying the exact values a create accepts, so nothing an
// operator sends will help and the diagnostic says so and points at the console.
//
// The remaining cases are the two Jamf does attribute. An unrecognised
// vocabulary value is refused with the offending value named in the message but
// no field named at all, so the message is passed through verbatim — it is the
// only thing that localises the problem. A missing top-level field is one of the
// four Jamf names, and reaches an operator only through a configuration that
// resolved to nothing.
func appendWriteDiagnostics(diags *diag.Diagnostics, action string, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeUpstreamError:
			if action == actionCreate {
				diags.AddError(
					"Jamf Account refused to create the SSO connection",
					"Jamf Account refused the connection with an internal failure that names no field, and answers "+
						"that same failure for several different problems, so work through the ones it is known to "+
						"cover:\n\n"+
						"  - every name in `domains` has to be claimed and verified by your organization;\n"+
						"  - `name` has to be letters and digits only;\n"+
						"  - the settings block has to be the one `connection_type` names, and has to carry every "+
						"value that family requires;\n"+
						"  - your organization may already hold as many connections as Jamf Account allows, which is "+
						"refused this same way: remove one you no longer need, or ask Jamf Support for more.\n\n"+
						"If none of those fit, raise it with Jamf Support and quote the trace identifier: "+
						traceIDOrUnknown(apiErr)+". Reported by Jamf Account: "+detail.Description,
				)
				break
			}
			diags.AddError(
				"Jamf Account cannot "+action+" an SSO connection",
				"Jamf Account refused to "+action+" the connection with an internal failure that carries no "+
					"detail. This is a known fault in Jamf Account, not a problem with this configuration: every "+
					"attempt to "+
					"change a connection is refused the same way, in every region, and nothing you send alters "+
					"that — the same refusal comes back for the exact values a create accepts.\n\n"+
					"Creating, reading, listing and destroying connections all work. Until it is fixed, edit "+
					"connections in the Jamf Account console and use this provider to read them. If you raise it "+
					"with Jamf Support, quote the trace identifier: "+traceIDOrUnknown(apiErr)+". Reported by "+
					"Jamf Account: "+detail.Description,
			)
		case codeBadRequest:
			diags.AddError(
				"Jamf Account refused a value on the connection",
				"Jamf Account refused one of the values on this connection and does not say which attribute it "+
					"was on: "+
					"it names only the value. Reported by Jamf Account: "+detail.Description,
			)
		case codeFieldValidation:
			diags.AddError(
				"A required part of the connection did not reach Jamf Account",
				"Jamf Account reports a required part of the connection as missing or empty. This usually means a "+
					"variable or a reference resolved to nothing — `domains` in particular has to hold at least "+
					"one verified domain name. Reported by Jamf Account: "+detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// traceIDOrUnknown renders the trace identifier from a refusal, or says there was
// none, so the diagnostic can always tell an operator what to quote.
func traceIDOrUnknown(apiErr *jamfplatform.APIResponseError) string {
	if id := apiErr.TraceID; id != "" {
		return id
	}
	return "none was returned"
}

// consentFlowExplanation is the one fact both consent-flow diagnostics rest on,
// stated once so the two cannot drift apart.
const consentFlowExplanation = "A connection set up with Microsoft's admin-consent flow has no client " +
	"registration of its own — Jamf holds the consent — and nothing this provider can send expresses that, so " +
	"any change Terraform tried to apply would be refused or would silently replace the consent with a client " +
	"that does not exist."

// appendConsentFlowDiagnostics refuses a connection built through Microsoft's
// admin-consent flow, and reports whether it did.
//
// Such a connection reads back cleanly and looks ordinary in every attribute an
// operator would think to check, which is exactly why it needs refusing where it
// is first seen rather than where it first fails. Letting one into state buys an
// entry that can never be applied: it has no client identifier, the settings
// Jamf accepts have no way to declare the consent, and the only escape is
// `terraform state rm` by hand.
//
// The refusal lands on Read, which covers both the read that follows an import
// and an ordinary refresh, and on Update, so a refresh Terraform skipped cannot
// let one through. Destroy is deliberately allowed: withdrawal works, and
// refusing it would trap an operator who already holds one.
func appendConsentFlowDiagnostics(diags *diag.Diagnostics, c *account.Connection) bool {
	if c == nil || !c.ConsentFlow {
		return false
	}
	diags.AddAttributeError(
		path.Root("consent_flow"),
		"Connection uses Microsoft admin consent and cannot be managed",
		"The connection \""+c.Name+"\" was set up with Microsoft's admin-consent flow, so "+
			"jamfplatform_account_sso_connection cannot manage it. "+consentFlowExplanation+" Read it with the "+
			"`jamfplatform_account_sso_connection` data source instead, which reports such a connection without "+
			"claiming to own it, and make any change in the Jamf Account console. If Terraform already holds it, "+
			"run `terraform state rm` on the address to drop it.",
	)
	return true
}

// appendConsentFlowUpdateDiagnostics refuses to change a connection built with
// Microsoft admin consent.
//
// Keyed on the value already in state rather than on a refusal from Jamf, which
// makes it deterministic and needs no guess about how such a write would be
// answered. That guess would be a particularly bad one at the moment: every
// change is refused identically whatever the reason, so a wire-keyed check could
// not tell this apart from the fault affecting every connection. Read refuses
// such a connection before it can reach state at all, so this covers the state
// an earlier provider version could have written, and it runs before the request
// so no doomed write is ever issued.
func appendConsentFlowUpdateDiagnostics(diags *diag.Diagnostics, name string) {
	diags.AddError(
		"Connection using Microsoft admin consent cannot be changed",
		"Terraform holds \""+name+"\" as a managed connection, but Jamf Account reports it as using Microsoft's "+
			"admin-consent flow. "+consentFlowExplanation+" Nothing Terraform can send would change it, so no "+
			"request was made. Change it in the Jamf Account console, run `terraform state rm` on this address "+
			"to drop it from state, and read it with the `jamfplatform_account_sso_connection` data source if "+
			"you still need its values.",
	)
}

// appendGhostConnectionDiagnostics reports a connection the organization's
// collection lists but which cannot be read on its own identifier.
//
// This is a disagreement inside Jamf between its own store and whatever the
// single-connection read consults, observed on one connection in the probe
// organization. The important half is what this must *not* do: treat the
// not-found as a withdrawal and drop the resource. It exists — the collection is
// Jamf's own record of it — so removing it from state would plan a fresh create
// of a connection that is already there, and the create would collide or, worse,
// leave two.
//
// Because it is an error and a refresh runs ahead of every operation by default,
// it also blocks its own destroy, which leaves an unattended pipeline with nothing
// to do but stop. So the diagnostic names both ways past it: dropping the resource
// from state, and destroying without a refresh.
func appendGhostConnectionDiagnostics(diags *diag.Diagnostics, id, name string) {
	diags.AddError(
		"Jamf Account cannot read this SSO connection on its own identifier",
		"The connection \""+name+"\" (identifier "+id+") is listed among your organization's connections, but "+
			"reading it on that identifier reports it missing. That is a disagreement inside Jamf between its "+
			"own list and the record behind a single connection, not a sign the connection is gone — so "+
			"Terraform has deliberately left it in state rather than planning to create it again, which would "+
			"risk a duplicate.\n\n"+
			"Nothing in this configuration can fix it. Raise it with Jamf Support, quoting the identifier above. "+
			"Meanwhile every plan, apply and destroy against this address reports this, because each one "+
			"refreshes first: `terraform state rm` on the address stops Terraform trying to read it and leaves "+
			"the connection with Jamf, while `terraform destroy -refresh=false` takes the connection away "+
			"without reading it first, which is the way to tear it down without editing state by hand.",
	)
}

// findSummary returns the collection entry for one identifier, or nil.
func findSummary(summaries []account.ConnectionSummary, id string) *account.ConnectionSummary {
	for i := range summaries {
		if summaries[i].ID == id {
			return &summaries[i]
		}
	}
	return nil
}

// findSummariesByName returns every collection entry whose stored name matches.
//
// It returns all of them rather than the first because the stored name is not a
// unique key: Jamf may hold a uniquified form of whatever name was configured, so
// two connections asked for the same name can both answer to a name lookup. A
// caller reporting an ambiguity is more useful than one silently picking.
func findSummariesByName(summaries []account.ConnectionSummary, name string) []account.ConnectionSummary {
	var out []account.ConnectionSummary
	for i := range summaries {
		if summaries[i].Name == name {
			out = append(out, summaries[i])
		}
	}
	return out
}

// summaryNames renders the stored names of a collection for a diagnostic, sorted
// so the list is stable.
func summaryNames(summaries []account.ConnectionSummary) string {
	names := make([]string, 0, len(summaries))
	for _, s := range summaries {
		names = append(names, "\""+s.Name+"\" ("+s.ID+")")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// splitFilterGroups turns Jamf's comma-separated group list into its members.
//
// An empty string yields an empty slice rather than one empty member, because an
// operator with no groups is a real configuration and has to round-trip as an
// empty set. Members are trimmed, since nothing guarantees Jamf stores them
// without spaces around the separators.
func splitFilterGroups(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return []string{}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// joinFilterGroups renders group names as Jamf's comma-separated list, sorted so
// the same set always produces the same string. The attribute is a set, whose
// members reach the provider in an arbitrary order, so sorting is what keeps a
// write deterministic.
func joinFilterGroups(groups []string) string {
	sorted := make([]string, len(groups))
	copy(sorted, groups)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// decodeJSONObject parses a JSON object string.
func decodeJSONObject(raw string) (map[string]any, error) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// equivalentJSON reports whether two strings describe the same JSON.
//
// A value that will not parse is compared byte-wise, so a malformed document is
// still stable in state rather than looking different from itself. The
// attribute's validator is what reports it, against the right attribute.
func equivalentJSON(left, right string) bool {
	var decodedLeft, decodedRight any
	if json.Unmarshal([]byte(left), &decodedLeft) != nil || json.Unmarshal([]byte(right), &decodedRight) != nil {
		return left == right
	}
	return sameJSON(decodedLeft, decodedRight)
}

// sameJSON compares two decoded JSON values structurally, treating numbers by
// value so 1 and 1.0 agree.
func sameJSON(a, b any) bool {
	switch typedA := a.(type) {
	case map[string]any:
		typedB, ok := b.(map[string]any)
		if !ok || len(typedA) != len(typedB) {
			return false
		}
		for name, valueA := range typedA {
			valueB, present := typedB[name]
			if !present || !sameJSON(valueA, valueB) {
				return false
			}
		}
		return true
	case []any:
		typedB, ok := b.([]any)
		if !ok || len(typedA) != len(typedB) {
			return false
		}
		for i := range typedA {
			if !sameJSON(typedA[i], typedB[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// preferEquivalentJSON keeps the value Terraform planned when Jamf's copy says
// the same thing in different bytes.
//
// Jamf re-serialises the claim mapping on the way out, and `jsonencode` emits
// keys sorted and unindented, so the two forms differ constantly while meaning
// the same. Reconciling here rather than through a semantic-equality type is
// deliberate: semantic equality is never consulted while planning, so it cannot
// settle a difference between a configuration and prior state, whereas this runs
// on every path that writes state and settles all of them the same way.
func preferEquivalentJSON(planned types.String, fromWire *string) types.String {
	incoming := stringOrNull(fromWire)
	if planned.IsNull() || planned.IsUnknown() || incoming.IsNull() {
		return incoming
	}
	if equivalentJSON(planned.ValueString(), incoming.ValueString()) {
		return planned
	}
	return incoming
}

// stringOrNull renders an optional string, treating an empty one as absent —
// which is how Jamf reports a field it holds no value for.
func stringOrNull(v *string) types.String {
	if v == nil || *v == "" {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// boolOrNull renders an optional boolean.
func boolOrNull(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

// int64OrNull renders an optional whole number.
func int64OrNull(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// omitLoginHintFromWire inverts Jamf's login-hint flag into the console's
// question.
//
// Jamf records whether the hint is *forwarded*; the console asks whether it is
// *omitted*. See the package doc for why the console's sense is the one this
// provider exposes. The inverse is omitLoginHintToWire, and both directions are
// pinned by unit tests because a pair that inverts twice is indistinguishable
// from one that never inverts.
func omitLoginHintFromWire(aliasToIdp bool) types.Bool {
	return types.BoolValue(!aliasToIdp)
}

// omitLoginHintToWire inverts the console's question back into Jamf's flag.
//
// An unset value yields true, which is the Jamf field's own sense of "forward the
// hint" and matches the console's unticked box, so an operator who says nothing
// gets the smoother sign-in.
func omitLoginHintToWire(omit types.Bool) bool {
	if omit.IsNull() || omit.IsUnknown() {
		return true
	}
	return !omit.ValueBool()
}

// setToStrings reads a string set into a slice, treating an absent set as empty.
func setToStrings(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() {
		return []string{}, nil
	}
	var out []string
	diags := set.ElementsAs(ctx, &out, false)
	if out == nil {
		out = []string{}
	}
	return out, diags
}

// stringsToSet builds a string set, always known so a Computed attribute resolves
// at apply.
func stringsToSet(values []string) (types.Set, diag.Diagnostics) {
	elements := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elements = append(elements, types.StringValue(v))
	}
	return types.SetValue(types.StringType, elements)
}

// connectionIdentity is what a collection entry has to agree with before it can
// be called the connection a failed create left behind: the configured name, the
// provider family and the domains.
//
// The name alone will not do. Jamf does not require names to be unique, a
// collection entry carries no time it was made, and Jamf appends a suffix of its
// own to the stored name, so a connection someone made by hand months ago is
// indistinguishable from one this apply just made if the name is all that is
// compared. The family and the domains do not make the answer certain either, but
// they rule out the case that costs most — offering an operator a live production
// connection to adopt because it happened to share a name.
type connectionIdentity struct {
	name           string
	connectionType string
	domains        []string
}

// connectionIdentityFromPlan reduces a plan to the three values a collection
// entry can be compared on, translating the provider family back to Jamf's own
// spelling because that is what an entry carries.
func connectionIdentityFromPlan(ctx context.Context, plan ConnectionResourceModel) (connectionIdentity, diag.Diagnostics) {
	domains, diags := setToStrings(ctx, plan.Domains)
	return connectionIdentity{
		name:           plan.Name.ValueString(),
		connectionType: connectionTypeToWire[plan.ConnectionType.ValueString()],
		domains:        domains,
	}, diags
}

// connectionsMatchingPlan returns every connection that could be the one a failed
// create left behind: the configured name, or that name with Jamf's own suffix
// appended, carrying the same provider family and the same domains.
//
// Jamf stores a connection under the name it was sent plus a suffix of its own —
// `tfProbeOidc` becomes `tfProbeOidc-jqxld7tl4m454ed7s35647nmje5bmq` — and the
// suffix is random per connection, so a caller cannot predict the stored value.
// Matching both forms covers the four connections in a probed organization that
// carry no suffix at all alongside the eighteen that do.
//
// An entry with no family recorded is not a match: it cannot be shown to be this
// apply's, and a bare create failure is a better answer than one naming somebody
// else's connection.
//
// It is only ever used to answer "did a write that reported an error take effect
// anyway", never to resolve a name to a single connection: two connections can
// still agree on all three values, so more than one match is possible and the
// caller decides what to do about it.
func connectionsMatchingPlan(all []account.ConnectionSummary, want connectionIdentity) []account.ConnectionSummary {
	var matched []account.ConnectionSummary
	for _, candidate := range all {
		if candidate.Name != want.name && !strings.HasPrefix(candidate.Name, want.name+"-") {
			continue
		}
		if candidate.Type == nil || *candidate.Type != want.connectionType {
			continue
		}
		if !sameStrings(candidate.Domains, want.domains) {
			continue
		}
		matched = append(matched, candidate)
	}
	return matched
}

// sameStrings reports whether two collections hold the same members, ignoring
// order and repeats, which is how a domain set and Jamf's own copy of one have to
// be compared: the attribute is a set and Jamf returns a list.
func sameStrings(left, right []string) bool {
	outstanding := make(map[string]struct{}, len(left))
	for _, v := range left {
		outstanding[v] = struct{}{}
	}
	for _, v := range right {
		if _, ok := outstanding[v]; !ok {
			return false
		}
		delete(outstanding, v)
	}
	return len(outstanding) == 0
}

// appendOrphanedCreateDiagnostics reports a create that failed but may have left a
// connection behind.
//
// The premise is one unreproduced observation rather than settled behaviour: on
// 2026-09-02 a create answering 500 UPSTREAM_ERROR had made the connection
// regardless, and a later probe of roughly twenty refused creates left the
// organization's count unchanged every time. The safety net is kept anyway,
// because the cost of it is one collection read on a path that has already
// failed, and the cost of being wrong the other way is a connection nobody
// manages and a second one on the next apply.
//
// A ready-to-run import is offered only when one candidate survives the match. A
// collection entry carries no time it was made, so several candidates cannot be
// told apart, and naming the first would invite an operator to adopt a connection
// this apply never made. Neither way out can be chosen here even when the
// candidate is unambiguous: adopting it silently would commit state for an object
// whose contents were never confirmed, and taking it away would destroy something
// that may be in use.
func appendOrphanedCreateDiagnostics(diags *diag.Diagnostics, name string, orphans []account.ConnectionSummary, cause error) {
	identifiers := make([]string, 0, len(orphans))
	for _, orphan := range orphans {
		identifiers = append(identifiers, orphan.ID+" ("+orphan.Name+")")
	}

	if len(orphans) == 1 {
		diags.AddError(
			"Jamf Account SSO connection was created despite the error",
			"Creating the connection \""+name+"\" reported a failure, but Jamf Account holds a connection "+
				"matching it, so the create took effect and Terraform has no record of it. Applying again would "+
				"create a second one.\n\nConnection found: "+identifiers[0]+"\n\nEither import it:\n\n"+
				"    terraform import <this resource address> "+orphans[0].ID+"\n\nor remove it in the Jamf "+
				"Account console and apply again. Terraform cannot choose for you: adopting it would record state "+
				"for a connection whose contents were never confirmed, and removing it might destroy one already "+
				"in use.\n\nReported by Jamf Account: "+helpers.APIErrorDetail(cause),
		)
		return
	}

	diags.AddError(
		"Jamf Account SSO connection may have been created despite the error",
		"Creating the connection \""+name+"\" reported a failure, and your organization holds more than one "+
			"connection matching it. Nothing Jamf Account reports says when a connection was made, so Terraform "+
			"cannot tell which of these, if any, this apply created, and will not name one for you to import.\n\n"+
			"Connections found: "+strings.Join(identifiers, ", ")+"\n\nFind out in the Jamf Account console "+
			"which of them is new. Then either import that one:\n\n"+
			"    terraform import <this resource address> <its identifier>\n\nor remove it there and apply "+
			"again. Do not import one you did not just create: Terraform would adopt a connection someone else "+
			"manages and replace it on the next apply.\n\nReported by Jamf Account: "+helpers.APIErrorDetail(cause),
	)
}

// appendUnconfirmedUpdateDiagnostics reports a change that failed against a
// connection that still exists.
//
// The same asymmetry as a create applies, with the outcome inverted: the object is
// already in state, so nothing leaks, but whether the change landed is unknown —
// Jamf reports the same opaque error whether it applied the change or rejected it.
// Recording the planned values would be a lie if it did not, and leaving the prior
// state is a lie if it did, so it reports and refreshes rather than guessing.
func appendUnconfirmedUpdateDiagnostics(diags *diag.Diagnostics, id string, cause error) {
	diags.AddError(
		"Jamf Account SSO connection change could not be confirmed",
		"Changing connection "+id+" reported a failure, and Jamf Account does not say whether the change was "+
			"applied or refused. Terraform has left the previous values in state rather than record values it "+
			"cannot confirm.\n\nRun `terraform plan -refresh-only` to see what Jamf Account currently holds, "+
			"then apply again.\n\nReported by Jamf Account: "+helpers.APIErrorDetail(cause),
	)
}
