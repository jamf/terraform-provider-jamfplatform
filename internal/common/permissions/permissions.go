// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package permissions renders the Jamf API permissions a resource or data
// source requires into a Markdown table suitable for appending to a schema
// MarkdownDescription.
//
// The data is sourced from the jamfplatform-go-sdk: every generated SDK
// sub-package (pro, devices, proclassic, ...) exposes a Privileges map keyed by
// method name, whose Scoped field carries the GA capability permissions the
// endpoint requires in {capability}:{action} form. A construct declares the SDK
// methods it calls and this package turns the union of their requirements into
// one deduplicated, operator-facing table.
//
// The table is written for the person creating the API integration, which
// happens in Jamf Account, so a row is a row of Jamf Account's permission
// picker: the section it sits under, the name printed beside its checkboxes,
// and which of those checkboxes to tick. catalogue.go holds that mapping.
//
// A rendered block also names the API integration SCOPE the endpoints are
// published at, because permissions are not the whole of what an operator
// chooses in Jamf Account: an integration is created against a platform
// environment, a single tenant, or the organization, and a credential carrying
// the wrong one is refused whatever it has been granted. SDK v0.22.0 began
// emitting MethodPrivileges.Scopes for exactly this, so the line is derived from
// the same registry as the table.
//
// It reports what the SPEC declares, which is not always the whole of what the
// gateway serves — the Platform API GA withdrew X-Tenant-Id from six Platform
// specs while the gateway went on answering it — so this is a recommendation
// rather than an exhaustive list of what will work. The authority on what a
// given provider release actually accepts is providerdata.RequireScope, whose
// Configure diagnostic names the accepted scopes outright; that is where the
// evidenced divergences live, and deliberately not here, since a published table
// should carry Jamf's own declaration rather than this provider's tolerances.
//
// The SDK also publishes each method's pre-GA Jamf Pro privilege names
// (MethodPrivileges.Legacy) and this package used to render them beside the
// capability. It no longer does. Scoped and Legacy are independent sets — the
// GA consolidation mapped several pre-GA privileges onto one capability, and
// where the lengths do match the spec's own orders disagree — so no per-row
// label can be derived from them, and Jamf Account no longer offers the pre-GA
// names to grant.
package permissions

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// platformAPIGettingStartedURL is Jamf's "Getting started with the Platform
// API" page, which covers registering an API integration. A rendered table
// names both Jamf Account and its permission picker, and a reader arriving at a
// single Registry page from a search engine has no other route to either, so
// the lead-in links it. It is an absolute URL because the block is also
// embedded in each page's YAML frontmatter description, where a relative
// Registry link would not resolve.
const platformAPIGettingStartedURL = "https://developer.jamf.com/platform-api/reference/getting-started-with-platform-api"

// Registry is the type of the per-package Privileges maps exported by the SDK
// (e.g. pro.Privileges). Aliased here so callers and Merge read cleanly.
type Registry = map[string]jamfplatform.MethodPrivileges

// Merge combines several SDK package registries into one lookup, for resources
// whose construct spans more than one SDK family (e.g. a Pro resource that
// also calls a ProClassic endpoint). Later registries win on key collision.
//
// Last-wins is only sound while the colliding entries agree, and across
// families they need not: the same method name can front two endpoints
// requiring two different capabilities, in which case the merged registry
// documents whichever registry was passed last. That is not hypothetical —
// aigovernance and proclassic both key ListPolicies, for ai-policies:read and
// policies:read respectively — so a caller merging two families is asserting
// they are disjoint. TestSDKRegistriesAreDisjoint checks every pair of shipped
// registries and carries the known exceptions, so a new collision fails there
// rather than in a published permission table.
func Merge(registries ...Registry) Registry {
	out := make(Registry)
	for _, reg := range registries {
		maps.Copy(out, reg)
	}
	return out
}

// malformedAction is the action recorded for a scoped value splitScoped could
// not parse. It is prose rather than a slug and has no actionLabels entry, so
// actionList prints it verbatim: the row's Actions cell says the shape was not
// understood instead of sitting empty, which in Jamf Account's model would read
// as a permission granting nothing.
const malformedAction = "unrecognised permission shape"

// scopeLabels names each scope kind the way Jamf Account labels it when an API
// integration is created, since the rendered block is written throughout in that
// picker's vocabulary rather than the wire's. Organization management sends no
// scope header at all — the gateway resolves it from the access token — so it is
// named by what an operator picks, not by a header they will never see.
var scopeLabels = map[jamfplatform.ScopeKind]string{
	jamfplatform.ScopeEnvironment:  "Platform environment",
	jamfplatform.ScopeTenant:       "Tenant",
	jamfplatform.ScopeOrganization: "Organization management",
}

// scopeOrder is the order a rendered scope list is offered in: environment
// first, because it is the scope Jamf intends new integrations to be created
// with and the one the withdrawals are moving every Platform surface towards.
var scopeOrder = []jamfplatform.ScopeKind{
	jamfplatform.ScopeEnvironment,
	jamfplatform.ScopeTenant,
	jamfplatform.ScopeOrganization,
}

// scopeCaveat is the sentence every rendered scope clause carries, and it says
// the two things the scope name alone does not.
//
// The first is that the choice is permanent: an API integration is created
// against one scope in Jamf Account and there is no attribute anywhere that
// moves it afterwards, so an operator reading a scope on a Registry page needs
// to know whether the page is describing a decision they can revisit. Stating
// it here puts it in front of every reader of every construct, once, rather
// than in a guide the reader arriving from a search engine never opens.
//
// The second is that this block reports the specification and not this
// provider's enforcement, and it points at the enforcement rather than
// describing it, because a per-construct page cannot say which side of the
// divergence its own construct falls on: permissions deliberately does not read
// gatewayWidenings, so the sentence has to hold on all ~370 pages and it is a
// widening on three of them. The guide carries the detail. The two deliberately differ — the Platform API GA
// withdrew X-Tenant-Id from six Platform specs while the gateway went on
// answering it, and providerdata.RequireScope accepts the wider set for the
// families where that was probed — so a flat imperative to create the
// integration with the declared scope would tell an operator whose tenant-scoped
// credential already works that they must register a replacement. The
// Configure-time diagnostic is named as the authority because it is derived from
// what the release enforces, and it is silent when the scope is accepted, which
// is precisely the case a published imperative would get wrong.
const scopeCaveat = "You choose an integration's scope when you create it in Jamf Account, and " +
	"cannot change it afterwards. The provider names the scopes it accepts when you configure it, " +
	"and for a few families that is wider than Jamf lists here."

// requirement is one deduplicated table row: a capability plus every action on
// it the construct needs. One row per capability rather than per
// capability-action pair, because that is how Jamf Account presents it — a
// permission with a checkbox per action — and it collapses the four rows of an
// ordinary CRUD resource into one.
//
// malformed marks a row built from a scoped value splitScoped rejected. Such a
// row is kept apart from a well-formed row for the same capability rather than
// merged into it, because merging would let the malformed value vanish into a
// sibling's checkboxes and leave no trace that anything was unreadable.
type requirement struct {
	capability string
	actions    map[string]bool
	malformed  bool
}

// splitScoped divides a GA capability permission into its capability and action
// halves and reports whether the value had that shape at all. The SDK's Scoped
// field documents the two-part GA form as the only one it emits and every entry
// in every shipped registry is that form, so anything else is a shape this
// package has never seen: an empty half, no colon, or more than one colon as in
// the retired three-part beta slug "create:pro:buildings". More than one colon
// is rejected rather than cut at the first, because "create:pro:buildings"
// would otherwise yield the capability "create" — naming a permission that does
// not exist, under a footnote blaming the wrong cause.
func splitScoped(scoped string) (capability, action string, ok bool) {
	capability, action, ok = strings.Cut(scoped, ":")
	if !ok || capability == "" || action == "" || strings.Contains(action, ":") {
		return "", "", false
	}
	return capability, action, true
}

// collect returns the deduplicated capability requirements across the named
// methods, plus the names not found in the registry. A missing slice lets the
// caller (and the drift-guard test) detect a method that was renamed or removed
// in the SDK. noPrivilege reports that every method that did resolve requires
// no permission at all, which is a different answer from none of them
// resolving.
//
// Rows are ordered by section name and then by permission name, alphabetically.
// The order is therefore derived from the catalogue itself rather than from a
// second hand-maintained list: Jamf Account's row order is a weaker contract
// than its names, since the picker can be reordered without anything being
// renamed and no test could detect it. A row with no catalogue entry, and a row
// built from a scoped value splitScoped rejected, both sort after every
// resolved row — neither has picker names to sort on, and both render as
// visibly incomplete.
//
// A method absent from the registry suppresses the scope claim as well as
// marking the row set incomplete, for the same reason intersectScopes treats an
// entry declaring no scope as narrowing the result to nothing: the intersection
// would then be computed from a strict subset of what the construct calls, and
// a scope every resolved method accepts can still be refused by the one that was
// skipped. That subset renders identically to a complete answer, so the only
// safe rendering of a partial fold is none. It is a separate condition from
// known == 0 because a mixed list still resolves methods, which is exactly the
// case a count of resolutions cannot see.
func collect(reg Registry, methods []string) (reqs []requirement, noPrivilege bool, missing []string, scopes []jamfplatform.ScopeKind) {
	seen := make(map[string]int)
	known := 0
	scopes = slices.Clone(scopeOrder)
	for _, name := range methods {
		mp, ok := reg[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		known++
		scopes = intersectScopes(scopes, mp.Scopes)
		for _, scoped := range mp.Scoped {
			capability, action, parsed := splitScoped(scoped)
			key := capability
			if !parsed {
				capability, action = scoped, malformedAction
				key = malformedAction + "\x00" + capability
			}
			idx, dup := seen[key]
			if !dup {
				idx = len(reqs)
				seen[key] = idx
				reqs = append(reqs, requirement{
					capability: capability,
					actions:    map[string]bool{},
					malformed:  !parsed,
				})
			}
			reqs[idx].actions[action] = true
		}
	}
	noPrivilege = known > 0 && len(reqs) == 0
	if known == 0 || len(missing) > 0 {
		scopes = nil
	}
	sort.Slice(reqs, func(i, j int) bool {
		ci, ni, ki := rowKey(reqs[i])
		cj, nj, kj := rowKey(reqs[j])
		if ki != kj {
			return ki
		}
		if ci != cj {
			return ci < cj
		}
		if ni != nj {
			return ni < nj
		}
		return reqs[i].capability < reqs[j].capability
	})
	return reqs, noPrivilege, missing, scopes
}

// intersectScopes narrows have to the kinds also present in add, preserving
// have's order.
//
// The fold is an intersection, not a union, because Scopes is an alternatives
// set per method while a client carries exactly one scope: a construct calling
// several methods needs a credential every one of them accepts. A union would
// name a scope one of the endpoints refuses, sending an operator to create the
// wrong integration.
//
// An entry declaring no scope at all narrows the result to nothing. The SDK
// guarantees a non-empty set — a spec declaring none fails its generation — so
// this can only mean a registry that predates the field, and claiming a scope
// from the methods that do carry one would publish a partial answer as a
// complete one. Section suppresses the line instead.
func intersectScopes(have, add []jamfplatform.ScopeKind) []jamfplatform.ScopeKind {
	if len(add) == 0 {
		return nil
	}
	return slices.DeleteFunc(have, func(k jamfplatform.ScopeKind) bool {
		return !slices.Contains(add, k)
	})
}

// scopePhrase names the resolved scope set the way the lead-in reads it — for
// example "**Platform environment** scope" or "**Platform environment** scope
// (preferred for new integrations) or **Tenant** scope" — and
// returns "" when nothing can be claimed.
//
// Where two kinds are accepted the first carries a recommendation rather than
// being listed flatly, because the choice is not free in either direction: an
// integration targets one scope, crossing the header over is refused 403
// OWNERSHIP_FORBIDDEN even inside one customer, and the Platform surfaces are
// being moved to environment one spec at a time. The recommendation says what
// it is for rather than sitting as a bare "(preferred)", which told an operator
// nothing about why they should prefer it. Whose recommendation it is comes from
// the lead-in Section builds around this phrase, whose subject is Jamf, so the
// parenthetical does not repeat the attribution. A kind with no scopeLabels
// entry suppresses the phrase entirely rather than printing a partial list,
// which would understate what the endpoints accept.
func scopePhrase(scopes []jamfplatform.ScopeKind) string {
	labels := make([]string, 0, len(scopes))
	for _, k := range scopes {
		label, ok := scopeLabels[k]
		if !ok {
			return ""
		}
		labels = append(labels, "**"+label+"** scope")
	}
	switch len(labels) {
	case 0:
		return ""
	case 1:
		return labels[0]
	default:
		return labels[0] + " (preferred for new integrations) or " + joinOr(labels[1:])
	}
}

// joinOr renders a label list as "a", "a or b", or "a, b or c".
func joinOr(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1]
	}
}

// rowKey returns the sort key of a rendered row: the Jamf Account section and
// permission name it will print, plus whether those names resolved at all.
// Sorting on the names themselves keeps the order alphabetical and derivable,
// and the known flag is what puts an unresolved row last — stated as its own
// term rather than smuggled in as a category string chosen to sort after every
// real one.
func rowKey(r requirement) (category, name string, known bool) {
	e, ok := catalogue[r.capability]
	if !ok || r.malformed {
		return "", "", false
	}
	return e.category, e.name, true
}

// actionList renders an action set as the words Jamf Account prints beside the
// checkboxes, in the platform's own create/read/update/delete/deploy/execute
// order. An action with no actionLabels entry — one outside those six, or the
// malformedAction sentinel — is passed through verbatim so it shows up as
// itself rather than vanishing. The label lookup is guarded so that an action
// added to actionOrder but not to actionLabels renders once as its slug through
// that same path, rather than contributing an empty entry the operator cannot
// identify and then repeating itself.
func actionList(actions map[string]bool) string {
	var out []string
	for _, a := range actionOrder {
		if label, ok := actionLabels[a]; ok && actions[a] {
			out = append(out, label)
		}
	}
	var extra []string
	for a := range actions {
		if _, known := actionLabels[a]; !known {
			extra = append(extra, a)
		}
	}
	sort.Strings(extra)
	out = append(out, extra...)
	return strings.Join(out, ", ")
}

// Section renders the full "Required Jamf permissions" Markdown block (heading,
// lead-in, and table) for the SDK methods a construct calls, ready to append to
// a schema MarkdownDescription. It returns "" when none of the named methods
// resolve to a registry entry, so callers can append it unconditionally.
// Unknown method names are silently skipped here; the per-construct drift-guard
// test is responsible for failing the build when a declared method is absent
// from the registry.
//
// The affirmative "None" wording is published only when every named method
// resolved. It is a positive claim that the underlying endpoints need no
// permission, and a method absent from the registry has requirements this
// package never read, so a list mixing the two renders nothing rather than an
// assurance it cannot support.
//
// A row whose capability has no catalogue entry, or whose scoped value
// splitScoped rejected, prints "—" in both name columns and triggers the
// footnote. The footnote exists because the two look identical in a table: a
// dash means this provider's copy of Jamf's permissions map does not cover the
// row, not that the permission is ungrantable.
func Section(reg Registry, methods ...string) string {
	reqs, noPrivilege, missing, scopes := collect(reg, methods)
	phrase := scopePhrase(scopes)

	if noPrivilege && len(missing) == 0 {
		none := "None — any authenticated Jamf Platform API integration may call the underlying endpoints."
		if phrase != "" {
			none = "None beyond scope. Jamf lists this under " + phrase +
				", and it needs no permission of its own. " + scopeCaveat
		}
		return "\n\n**Required Jamf permissions**\n\n" + none
	}
	if len(reqs) == 0 {
		return ""
	}

	lead := "Grant the API integration the following permissions in Jamf Account — see "
	if phrase != "" {
		lead = "Jamf lists this under " + phrase + ". " + scopeCaveat +
			" Grant the API integration the following permissions in Jamf Account — see "
	}

	var b strings.Builder
	b.WriteString("\n\n**Required Jamf permissions**\n\n")
	b.WriteString(lead +
		"[Getting started with the Platform API](" + platformAPIGettingStartedURL + "). " +
		"`Category` and `Permission` name the section and row of the permission picker; " +
		"`Actions` are the boxes to tick within that row.\n\n")
	b.WriteString("| Category | Permission | Actions | API capability |\n|---|---|---|---|\n")
	unknown := false
	for _, r := range reqs {
		e, ok := catalogue[r.capability]
		if !ok || r.malformed {
			unknown = true
			e = entry{category: "—", name: "—"}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | `%s` |\n", e.category, e.name, actionList(r.actions), r.capability)
	}
	if unknown {
		b.WriteString("\n`—` marks a row this provider release has no Jamf Account name recorded for: " +
			"either the capability postdates its copy of the Jamf permissions map, or the permission was " +
			"not in the expected `{capability}:{action}` form. Look the capability up in the " +
			"[Jamf Pro permissions map](" + permissionsMapURL + ") and open an issue so the row is added.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Missing resolves the named methods against the registry and returns those
// absent from it. Drift-guard tests call this to assert a construct's declared
// SDK method list stays in sync with the SDK.
func Missing(reg Registry, methods ...string) []string {
	_, _, missing, _ := collect(reg, methods)
	return missing
}

// Renders reports whether a Section-rendered block grants scoped, a GA
// capability permission in {capability}:{action} form.
//
// It exists for the per-construct drift-guard tests, which assert that a
// construct's table really did render the permissions its SDK methods require.
// Those tests used to look for the scoped string with strings.Contains, which
// stopped working when the table moved to Jamf Account's own vocabulary: the
// capability and the action now sit in different cells, and the action is a
// word rather than a slug. Parsing the table back beats a substring match on
// the capability alone, which would pass a row that ticks the wrong boxes.
//
// It checks the two machine-derived columns only, API capability and Actions.
// Category and Permission are deliberately unchecked: they are transcribed from
// Jamf's prose, so nothing machine-readable exists to compare them against, and
// an assertion over them could only compare catalogue.go with itself. A
// mistyped picker name therefore passes every per-construct guard;
// testdata/catalogue.golden is what pins those two columns, by turning an
// edited row into a reviewable diff.
func Renders(section, scoped string) bool {
	capability, action, ok := splitScoped(scoped)
	if !ok {
		return false
	}
	label, ok := actionLabels[action]
	if !ok {
		return false
	}
	for line := range strings.SplitSeq(section, "\n") {
		cells := tableCells(line)
		if len(cells) != 4 || cells[3] != "`"+capability+"`" {
			continue
		}
		if slices.Contains(strings.Split(cells[2], ", "), label) {
			return true
		}
	}
	return false
}

// tableCells splits one Markdown table row into its trimmed cells, or returns
// nil for a line that is not one.
func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	var out []string
	for cell := range strings.SplitSeq(strings.Trim(line, "|"), "|") {
		out = append(out, strings.TrimSpace(cell))
	}
	return out
}
