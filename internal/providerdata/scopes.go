// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"fmt"
	"slices"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// The scope sets below are what RequireScope call sites pass, and they are
// derived from the SDK privilege registry rather than written out per construct.
//
// SDK v0.22.0 began emitting MethodPrivileges.Scopes — the scope kinds an
// endpoint is published at — so the answer to "which integration scope reaches
// this API family" is now data the SDK carries instead of a literal repeated at
// every Configure. That matters because the answer moves: the Platform API GA
// withdrew X-Tenant-Id from six Platform specs, and a set written out by hand at
// 27 call sites is a set that goes stale silently at 27 call sites. Deriving it
// means a spec ingest that narrows or widens a family arrives as one changed
// value, and scopes_test.go fails on the entries whose justification the change
// retires.
//
// Scopes is an ALTERNATIVES set per method — the endpoint is published at each
// kind listed and the caller picks one, because a client carries exactly one
// scope. A construct calls several methods through one client, so the scope its
// credential must carry is the INTERSECTION across those methods, not the union.
// Scope is declared per spec rather than per operation, so in practice every
// method in a package agrees and the intersection is that package's set;
// TestFamilyScopesAreUniformPerRegistry pins the assumption rather than trusting
// it, since the SDK keeps the field per-method precisely because two specs in
// one package have disagreed before.

// scopeOrder is the order a resolved scope set is reported in, and therefore the
// order RequireScope's diagnostic offers the acceptable scopes: environment
// first, because it is the scope Jamf intends new integrations to be created
// with, then tenant, then organization.
var scopeOrder = []ScopeKind{ScopeEnvironment, ScopeTenant, ScopeOrganization}

// ScopeOrder returns the order this package reports a resolved scope set in.
//
// It is exported for one caller, and that caller is a test rather than
// production code: internal/common/permissions renders the same preference into
// every construct's documentation, from its own scopeOrder over the SDK's enum
// rather than this one, because the two packages speak different vocabularies —
// this one names a scope the way a diagnostic must ("environment"), that one the
// way Jamf Account's picker does ("Platform environment"). Two orderings of one
// user-facing decision can disagree, and the disagreement would be invisible:
// each package's own test checks its order against itself. Handing the order out
// lets the docs side assert the two agree kind-for-kind. The returned slice is a
// copy, so a caller cannot reorder the authorization path's own preference.
func ScopeOrder() []ScopeKind {
	return slices.Clone(scopeOrder)
}

// Scope sets per API family, in the preference order RequireScope documents.
//
// AccountScopes resolves to organization alone from the SDK's own
// config.scopeTypes rather than from a published extension: no account spec
// declares x-scope-types, and the routes resolve the organization from the
// access token. It is the one family whose set is a wire-evidenced assertion on
// the SDK's side too — read MethodPrivileges.ScopesSource to tell that apart
// from a transcription.
var (
	AccountScopes              = resolveFamily("Jamf Account", account.Privileges)
	AIGovernanceScopes         = resolveFamily("Jamf AI Governance", aigovernance.Privileges)
	BlueprintsScopes           = resolveFamily("Blueprints", blueprints.Privileges)
	ComplianceBenchmarksScopes = resolveFamily("Compliance Benchmarks", compliancebenchmarks.Privileges)
	DeviceActionsScopes        = resolveFamily("Platform device actions", deviceactions.Privileges)
	DeviceGroupsScopes         = resolveFamily("Platform device groups", devicegroups.Privileges)
	DevicesScopes              = resolveFamily("Platform devices", devices.Privileges)
	ProScopes                  = resolveFamily("Jamf Pro", pro.Privileges, proclassic.Privileges)
	SecurityCloudScopes        = resolveFamily("Jamf Security Cloud", securitycloud.Privileges)
)

// widening records a scope the gateway still serves after the published spec
// withdrew it, so a construct keeps accepting a credential that demonstrably
// works. It is the provider-side counterpart of the SDK's own config.scopeTypes
// override, and carries the same obligation: an entry is an assertion about the
// gateway, evidenced on the wire on a stated date, not a preference.
//
// An entry is deleted, not edited, when either half of its justification goes:
// the spec catching up makes it redundant, and the gateway following the
// withdrawal makes it wrong. TestWideningEntriesAreStillWidenings fails on the
// first case, because a redundant entry is indistinguishable from a live one at
// a call site and would go on claiming evidence for a scope the spec now grants
// outright. The second case cannot be caught from here — it needs a probe — so
// each entry names, in full, the request that was made, what answered it, the
// control that classifies that answer, and which privileges the probe actually
// exercised. Anything less is not re-probeable, and an entry a maintainer cannot
// re-probe is an entry they will either delete while it is still load-bearing or
// keep long after it is wrong.
type widening struct {
	family string
	scope  ScopeKind
	// why states the wire evidence: what was probed, when, and what answered.
	why string
}

// gatewayWidenings are the families whose spec-declared scope set understates
// what the gateway serves.
//
// All three are the Platform API GA's tenant withdrawal (public-apis-oas#436,
// #437, #439), which stripped X-Tenant-Id from six Platform specs while the
// gateway went on serving it. Two independent probes, both 2026-09-04: the
// SDK's, which stands as TestAcceptance_TenantScopePlatformSpecsStillServed in
// jamfplatform/acc_tenant_scope_test.go and is therefore the probe to re-run
// rather than reconstruct, and this repo's own through the SDK against eu with
// the pro-tenant credential.
//
// Each probe carries two controls in the same invocation, and the entries below
// need both because they rest on two different answers. GET
// /pro/v1/jamf-pro-version at 200 is the SERVED control: it rules out a dead or
// unentitled credential, without which a blanket refusal across the namespaces
// reads as a withdrawal that has landed. A bogus path inside the namespace under
// test, answering 403 BAD_PERMISSIONS, is the UNROUTED control, and it is the
// one that gives a 404 its meaning — this repo's law is that an unmapped
// Platform route answers 403 BAD_PERMISSIONS, so a namespace answering anything
// else has a handler behind it and accepted the header. A 200 is
// self-classifying and needs no unrouted control, which is why the devices and
// device-groups entries cite the response alone and the device-actions entry
// cites the pair.
//
// The acceptance suite then ran green under tenant scope for
// jamfplatform_device_group (9 tests) and jamfplatform_devices, which is the
// part a status code cannot show: these constructs work end to end on a
// credential the spec says should not reach them, so narrowing them would break
// working configurations. The SDK test above fails the day the withdrawal
// reaches the gateway, and is where that news will arrive first.
//
// Blueprints and Compliance Benchmarks are deliberately ABSENT, and the reason
// is an observation of Jamf Account rather than a status code. Read the whole of
// it before adding them back.
//
// Their specs withdrew tenant on the same build, and two DIFFERENT tenant-scoped
// credentials are refused 403 BAD_PERMISSIONS on both — the SDK's on 2026-09-04
// and this repo's pro-tenant acceptance credential, first on 2026-09-03 in
// .github/acceptance-lanes.json's evidence field and re-probed 2026-09-04. Those
// 403s classify nothing, by the law stated three paragraphs up: a Platform 403
// BAD_PERMISSIONS is what an unmapped route answers and also what a privilege
// gap answers, and two credentials that both lack the capability tell those
// apart no better than one does. Neither family has been probed with an unrouted
// control of the kind the device-actions entry carries, so on the wire alone the
// question is still open.
//
// It is not decided on the wire. Jamf Account's permission picker does not offer
// the blueprints or compliance-benchmarks capabilities at all when the
// integration being created is tenant-scoped — observed directly in the picker
// by the operator of this repository's acceptance estate on 2026-09-04. No
// tenant credential can therefore ever hold them, whatever the gateway is doing
// with the route, which is what makes the narrowing correct rather than merely
// spec-compliant: there is no configuration a practitioner could write that the
// refusal takes away. It also explains the 403s without needing them to
// classify anything, since a credential that cannot be granted a capability
// answers exactly as an unmapped route does.
//
// Two things follow that are easy to get wrong. The observation is of the
// PICKER, not of a request, so it is falsified by Jamf changing what the picker
// offers and not by the gateway starting to serve either route — a 200 from
// either would mean the picker has changed, and the entry to re-check is this
// one rather than the widenings below. And it does not transfer: the lane
// table's evidence field names securitycloud in the identical 403 list while
// SecurityCloudScopes stays environment-and-tenant, because Security Cloud's
// capabilities ARE offered to a tenant-scoped integration. A shared 403 grouped
// those three families together and the picker separates them, which is the
// whole reason this paragraph exists.
//
// AI Governance needs no entry either and never had one: it is environment-only
// in the spec, refused 403 under tenant scope on the same probe, and was already
// gated ScopeEnvironment by hand before this file existed.
var gatewayWidenings = []widening{
	{
		family: "Platform devices",
		scope:  ScopeTenant,
		why: "GET /devices/v1/devices answered 200 with 13 rows under X-Tenant-Id on 2026-09-04, " +
			"alongside the served control in the same invocation — a 200 is self-classifying and " +
			"needs no unrouted control",
	},
	{
		family: "Platform device groups",
		scope:  ScopeTenant,
		why: "GET /device-groups/v1/groups answered 200 with 53 rows under X-Tenant-Id on " +
			"2026-09-04, alongside the served control in the same invocation — a 200 is " +
			"self-classifying and needs no unrouted control",
	},
	{
		family: "Platform device actions",
		scope:  ScopeTenant,
		why: "POST /device-actions/v1/devices/00000000-0000-0000-0000-000000000000/check-in — the " +
			"device-management-action spec, which the gateway serves under the /device-actions " +
			"prefix — answered 404 under X-Tenant-Id on 2026-09-04 for a device id no tenant can " +
			"hold, against a bogus path in the same namespace answering 403 BAD_PERMISSIONS in " +
			"the same invocation: a routed handler rejecting the id, not the gateway refusing the " +
			"header. Probed through CheckInDevice, so under device-actions:execute ONLY; " +
			"EraseDevice and UnmanageDevice require destructive-device-actions:execute and are " +
			"UNPROBED under tenant scope, which is what jamfplatform_device_erase and " +
			"jamfplatform_device_unmanage are admitted on",
	},
}

// resolvedFamilies records the family names the provider's own scope sets were
// resolved under. gatewayWidenings is keyed by that name, and a key that matches
// nothing widens nothing — silently, since a widening that does not apply looks
// exactly like a family the gateway never diverged on.
// TestWideningFamiliesAreResolved reads this map so a mistyped or renamed family
// fails the build instead.
//
// Only resolveFamily writes it, and only the var block above calls resolveFamily.
// That separation is the point: the map used to be written by declaredScopes,
// which the tests also call, so a test passing gatewayWidenings[0].family seeded
// the exact key the guard reads and a typo in an entry survived four of six
// -shuffle seeds. A guard a test can satisfy on the guarded value's behalf is not
// a guard.
var resolvedFamilies = map[string]bool{}

// resolveFamily resolves one of the provider's own API families against the live
// gatewayWidenings table and records its name for the widening-key guard.
func resolveFamily(family string, regs ...map[string]jamfplatform.MethodPrivileges) []ScopeKind {
	resolvedFamilies[family] = true
	return declaredScopes(family, gatewayWidenings, regs...)
}

// declaredScopes resolves the scope kinds a credential may carry to reach every
// method in the given SDK privilege registries, widened by any evidenced entry
// in the given widening table for the family.
//
// The table is a parameter rather than a package read so the widening path can be
// asserted against a synthetic table. Parameterising it is what lets the mechanism
// stay under test on the day the spec catches up and the real entries are deleted,
// which the entries' own lifecycle rule says is the expected maintenance act;
// indexing the live table from a test instead made emptying it panic the whole
// package before nine unrelated tests could report.
//
// It panics rather than returning an empty set, because RequireScope treats an
// empty allowed list as "no scope requirement" and would let an unreachable
// construct plan and then fail at the gateway. All three panics are
// deterministic and fire at package initialisation, so `go test ./...` reaches
// them before any build ships: no registry at all, or any one of several empty,
// means the SDK renamed or dropped that package — checked per registry rather
// than across all of them, because ProScopes folds two and an empty pro.Privileges
// beside a populated proclassic.Privileges would otherwise resolve the gate for
// ~115 packages from a registry describing none of their endpoints. An empty
// intersection means two specs in one package disagree so completely that no
// single credential reaches the family, which is a defect in the split rather
// than a scope to enforce.
//
// The intersection panic deliberately precedes the widening loop. Widening first
// would let an emptied intersection be rescued to the widened scope alone —
// dropping environment, the preferred scope and the only one those specs
// declare — so the one family set the panic cannot fire for would be a widened
// one, which is to say the three families with the most spec churn behind them. A
// widening may only ever add to a set the registry already resolved.
func declaredScopes(family string, widenings []widening, regs ...map[string]jamfplatform.MethodPrivileges) []ScopeKind {
	if len(regs) == 0 {
		panic(fmt.Sprintf("providerdata: no SDK privilege registry was given for %s — "+
			"its scope cannot be resolved from nothing", family))
	}
	var accepted []ScopeKind
	seen := false
	for i, reg := range regs {
		if len(reg) == 0 {
			panic(fmt.Sprintf("providerdata: SDK privilege registry %d of %d for %s is empty — "+
				"the package was renamed or withdrawn, so its scope cannot be resolved",
				i+1, len(regs), family))
		}
		for _, mp := range reg {
			kinds := methodScopes(family, mp)
			if !seen {
				accepted, seen = kinds, true
				continue
			}
			accepted = slices.DeleteFunc(accepted, func(k ScopeKind) bool {
				return !slices.Contains(kinds, k)
			})
		}
	}
	if len(accepted) == 0 {
		panic(fmt.Sprintf("providerdata: no scope reaches every %s method — the SDK registry's "+
			"specs disagree, so no single credential can serve the family", family))
	}
	for _, w := range widenings {
		if w.family == family && !slices.Contains(accepted, w.scope) {
			accepted = append(accepted, w.scope)
		}
	}
	return slices.DeleteFunc(slices.Clone(scopeOrder), func(k ScopeKind) bool {
		return !slices.Contains(accepted, k)
	})
}

// methodScopes maps one registry entry's SDK scope kinds onto the provider's,
// deduplicated.
func methodScopes(family string, mp jamfplatform.MethodPrivileges) []ScopeKind {
	out := make([]ScopeKind, 0, len(mp.Scopes))
	for _, s := range mp.Scopes {
		k := scopeKindFromSDK(family, mp.Method, s)
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// scopeKindFromSDK maps an SDK scope kind onto the provider's.
//
// An unrecognised kind panics instead of falling through to a scope name. The
// SDK models organization as its zero value, so a silent default would map a
// fourth kind the SDK grew onto organization — widening every construct that
// reads it to a credential the endpoint does not accept, which is the failure
// this whole file exists to make impossible.
func scopeKindFromSDK(family, method string, s jamfplatform.ScopeKind) ScopeKind {
	switch s {
	case jamfplatform.ScopeOrganization:
		return ScopeOrganization
	case jamfplatform.ScopeEnvironment:
		return ScopeEnvironment
	case jamfplatform.ScopeTenant:
		return ScopeTenant
	}
	panic(fmt.Sprintf("providerdata: %s.%s declares SDK scope kind %d, which this provider has "+
		"no ScopeKind for — add one rather than letting it default", family, method, s))
}
