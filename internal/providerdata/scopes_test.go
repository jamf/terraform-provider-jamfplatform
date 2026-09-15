// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"maps"
	"slices"
	"strings"
	"testing"

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

// familyRegistries pairs each family name declaredScopes is called with against
// the SDK registries it was given, so the per-method assertions below cover
// exactly what the exported sets were built from.
var familyRegistries = map[string][]map[string]jamfplatform.MethodPrivileges{
	"Jamf Account":            {account.Privileges},
	"Jamf AI Governance":      {aigovernance.Privileges},
	"Blueprints":              {blueprints.Privileges},
	"Compliance Benchmarks":   {compliancebenchmarks.Privileges},
	"Platform device actions": {deviceactions.Privileges},
	"Platform device groups":  {devicegroups.Privileges},
	"Platform devices":        {devices.Privileges},
	"Jamf Pro":                {pro.Privileges, proclassic.Privileges},
	"Jamf Security Cloud":     {securitycloud.Privileges},
}

// TestFamilyScopesMatchTheSDKRegistry pins every exported scope set.
//
// This is the drift guard the derivation exists for: the sets are computed from
// the SDK, so an ingest that moves a family's scope changes provider behaviour
// with no provider edit at all. Pinning them here turns that into a named
// failure a reviewer has to agree with. Widen or narrow the expectation only
// alongside the reason — a spec change is enough for a narrowing, while widening
// past what the spec declares takes a gatewayWidenings entry and its evidence.
func TestFamilyScopesMatchTheSDKRegistry(t *testing.T) {
	want := map[string][]ScopeKind{
		"AccountScopes":              {ScopeOrganization},
		"AIGovernanceScopes":         {ScopeEnvironment},
		"BlueprintsScopes":           {ScopeEnvironment},
		"ComplianceBenchmarksScopes": {ScopeEnvironment},
		"DeviceActionsScopes":        {ScopeEnvironment, ScopeTenant},
		"DeviceGroupsScopes":         {ScopeEnvironment, ScopeTenant},
		"DevicesScopes":              {ScopeEnvironment, ScopeTenant},
		"ProScopes":                  {ScopeEnvironment, ScopeTenant},
		"SecurityCloudScopes":        {ScopeEnvironment, ScopeTenant},
	}
	got := map[string][]ScopeKind{
		"AccountScopes":              AccountScopes,
		"AIGovernanceScopes":         AIGovernanceScopes,
		"BlueprintsScopes":           BlueprintsScopes,
		"ComplianceBenchmarksScopes": ComplianceBenchmarksScopes,
		"DeviceActionsScopes":        DeviceActionsScopes,
		"DeviceGroupsScopes":         DeviceGroupsScopes,
		"DevicesScopes":              DevicesScopes,
		"ProScopes":                  ProScopes,
		"SecurityCloudScopes":        SecurityCloudScopes,
	}
	if len(got) != len(want) {
		t.Fatalf("the pinned set list and the exported set list disagree: %d pinned, %d exported — "+
			"a new family needs a pinned expectation", len(want), len(got))
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if !slices.Equal(got[name], want[name]) {
			t.Errorf("%s = %s, want %s — the SDK registry moved; agree with the change here or "+
				"record a gatewayWidenings entry", name, names(got[name]), names(want[name]))
		}
	}
}

// TestFamilyScopesAreOrderedAndDeduplicated asserts every resolved set is a
// subsequence of scopeOrder. RequireScope renders `allowed` verbatim into its
// diagnostic, so the order is user-facing: environment has to come first because
// it is the scope Jamf intends new integrations to carry.
func TestFamilyScopesAreOrderedAndDeduplicated(t *testing.T) {
	for _, family := range slices.Sorted(maps.Keys(familyRegistries)) {
		got := declaredScopes(family, gatewayWidenings, familyRegistries[family]...)
		if len(got) == 0 {
			t.Errorf("%s resolved no scope", family)
			continue
		}
		prev := -1
		for _, k := range got {
			at := slices.Index(scopeOrder, k)
			if at <= prev {
				t.Errorf("%s = %s, which is not in scopeOrder %s", family, names(got), names(scopeOrder))
				break
			}
			prev = at
		}
	}
}

// TestFamilyScopesAreUniformPerRegistry checks the assumption the package-level
// intersection rests on: that every method in a family declares the same scope
// set, because scope is a spec-root extension rather than an operation one.
//
// It is not redundant with the pinning test above. An intersection hides a
// disagreement — one method declaring environment alone inside a package that
// otherwise accepts both would silently narrow the whole family, and the pinned
// value would simply have to be edited to match. The SDK keeps Scopes per method
// precisely because two specs in one package have disagreed before, so this is
// the assertion that says which one it was.
func TestFamilyScopesAreUniformPerRegistry(t *testing.T) {
	for _, family := range slices.Sorted(maps.Keys(familyRegistries)) {
		for _, reg := range familyRegistries[family] {
			var first []ScopeKind
			var firstMethod string
			for _, name := range slices.Sorted(maps.Keys(reg)) {
				kinds := methodScopes(family, reg[name])
				if firstMethod == "" {
					first, firstMethod = kinds, name
					continue
				}
				if !slices.Equal(kinds, first) {
					t.Errorf("%s: %s declares %s but %s declares %s — the family's scope is no longer "+
						"uniform, so a package-level set cannot express it",
						family, name, names(kinds), firstMethod, names(first))
					break
				}
			}
		}
	}
}

// TestWideningFamiliesAreResolved fails on a gatewayWidenings entry whose family
// name matches none of the provider's own resolved families. Such an entry
// widens nothing at all, and nothing else would say so: a widening that does not
// apply is indistinguishable from a family the gateway never diverged on.
//
// resolvedFamilies is written only by resolveFamily, which only the exported var
// block calls, so nothing in this file can seed the key it is asked about. That
// is deliberate — the map was previously written by declaredScopes, which the
// tests below also call, and a test that passed gatewayWidenings[0].family
// through it seeded the exact key this test reads, letting a typoed family name
// survive four of six -shuffle seeds.
func TestWideningFamiliesAreResolved(t *testing.T) {
	for _, w := range gatewayWidenings {
		if !resolvedFamilies[w.family] {
			t.Errorf("gatewayWidenings names family %q, which no resolveFamily call resolves — "+
				"the entry widens nothing; fix the name or delete it", w.family)
		}
	}
}

// TestResolvedFamiliesMatchTheTestRegistryMap ties familyRegistries to the
// families the provider actually resolves.
//
// Without it the two could drift apart silently, and every per-method assertion
// in this file would go on passing while covering a family set the provider no
// longer gates on — including the widening-key guard above, whose whole value is
// that resolvedFamilies is production data.
func TestResolvedFamiliesMatchTheTestRegistryMap(t *testing.T) {
	for _, family := range slices.Sorted(maps.Keys(familyRegistries)) {
		if !resolvedFamilies[family] {
			t.Errorf("familyRegistries names family %q, which no resolveFamily call resolves — "+
				"the test map and the exported sets disagree", family)
		}
	}
	for _, family := range slices.Sorted(maps.Keys(resolvedFamilies)) {
		if _, ok := familyRegistries[family]; !ok {
			t.Errorf("family %q is resolved but absent from familyRegistries — its per-method "+
				"assertions are not running", family)
		}
	}
}

// TestWideningEntriesAreStillWidenings fails on an entry the SDK registry has
// caught up with.
//
// A redundant entry is worse than an absent one. It reads at the call site as
// though the scope rests on this provider's own wire evidence when the spec now
// grants it outright, so the day the gateway narrows again the entry looks like
// a deliberate override rather than a stale note nobody deleted. Delete it; the
// pinned set does not change.
func TestWideningEntriesAreStillWidenings(t *testing.T) {
	for _, w := range gatewayWidenings {
		regs, ok := familyRegistries[w.family]
		if !ok {
			continue // TestWideningFamiliesAreResolved owns this failure.
		}
		declared := true
		for _, reg := range regs {
			for _, mp := range reg {
				if !slices.Contains(methodScopes(w.family, mp), w.scope) {
					declared = false
				}
			}
		}
		if declared {
			t.Errorf("gatewayWidenings widens %s to %s, but the SDK registry now declares it — "+
				"delete the entry, the resolved set is unchanged either way", w.family, w.scope)
		}
		if strings.TrimSpace(w.why) == "" {
			t.Errorf("the %s / %s widening carries no wire evidence", w.family, w.scope)
		}
	}
}

// TestDeclaredScopesAppliesAWidening proves the widening path does something,
// against a synthetic registry AND a synthetic widening table.
//
// Both halves have to be synthetic for the claim to hold. Parameterising the
// registry alone leaves the assertion driven by whatever gatewayWidenings
// happens to hold, and the entries' own lifecycle rule says deleting them is the
// expected maintenance act — all three share one justification, so they
// plausibly go together. Indexing the live table made that act panic the package
// at gatewayWidenings[0] before nine unrelated tests, the RequireScope ones
// included, could report at all. Nothing here reads the live table, so emptying
// it leaves this file passing and still asserting the mechanism.
func TestDeclaredScopesAppliesAWidening(t *testing.T) {
	table := []widening{
		{family: "widened family", scope: ScopeTenant, why: "synthetic: asserts the mechanism, not the gateway"},
		{family: "some other family", scope: ScopeOrganization, why: "synthetic: must not apply here"},
	}
	got := declaredScopes("widened family", table, map[string]jamfplatform.MethodPrivileges{
		"OnlyMethod": {Method: "OnlyMethod", Scopes: []jamfplatform.ScopeKind{jamfplatform.ScopeEnvironment}},
	})
	if want := []ScopeKind{ScopeEnvironment, ScopeTenant}; !slices.Equal(got, want) {
		t.Fatalf("declaredScopes = %s, want %s — the widening for the family under test must be "+
			"added and the one for another family must not", names(got), names(want))
	}
}

// TestDeclaredScopesPanicsRatherThanRescuingAWidenedFamily pins the order of the
// intersection panic and the widening loop.
//
// Widening first would rescue an emptied intersection to the widened scope alone,
// dropping environment — the preferred scope, and the only one the widened
// families' specs declare. The panic sells itself as the reason this file cannot
// fail open, so the families it cannot fire for must not be exactly the three
// with the most spec churn behind them.
func TestDeclaredScopesPanicsRatherThanRescuingAWidenedFamily(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("declaredScopes rescued a disjoint widened family to the widened scope alone " +
				"instead of panicking — the widening loop is running before the intersection guard")
		}
	}()
	declaredScopes("widened family", []widening{
		{family: "widened family", scope: ScopeTenant, why: "synthetic"},
	}, map[string]jamfplatform.MethodPrivileges{
		"Environment": {Method: "Environment", Scopes: []jamfplatform.ScopeKind{jamfplatform.ScopeEnvironment}},
		"Organization": {Method: "Organization", Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeOrganization,
		}},
	})
}

// TestDeclaredScopesIntersectsRatherThanUnions pins the direction of the fold. A
// union would hand a construct a scope one of its methods refuses, which fails
// at the gateway mid-apply — the failure RequireScope exists to pre-empt.
func TestDeclaredScopesIntersectsRatherThanUnions(t *testing.T) {
	got := declaredScopes("uniform pair", nil, map[string]jamfplatform.MethodPrivileges{
		"Both": {Method: "Both", Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeEnvironment, jamfplatform.ScopeTenant,
		}},
		"EnvironmentOnly": {Method: "EnvironmentOnly", Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeEnvironment,
		}},
	})
	if want := []ScopeKind{ScopeEnvironment}; !slices.Equal(got, want) {
		t.Errorf("declaredScopes = %s, want %s", names(got), names(want))
	}
}

// TestDeclaredScopesPanicsOnAnEmptyRegistry and the three tests below it cover
// every way a resolved set could come back empty: a withdrawn registry, one
// withdrawn registry among several, no registry at all, and a set of specs no
// single scope satisfies. All four panic rather than returning nil, because
// RequireScope reads an empty allowed list as "no scope requirement" and would
// let an unreachable construct plan and then die at the gateway.
func TestDeclaredScopesPanicsOnAnEmptyRegistry(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("declaredScopes over an empty registry returned instead of panicking")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "registry") {
			t.Errorf("panic message %q does not say the registry was empty", r)
		}
	}()
	declaredScopes("withdrawn family", nil, map[string]jamfplatform.MethodPrivileges{})
}

// TestDeclaredScopesPanicsOnOneEmptyRegistryAmongSeveral covers the guard where
// it is actually load-bearing.
//
// ProScopes is the only two-registry call, and it is the gate for ~115 pro/
// packages plus every Pro action. A guard that fired only when EVERY registry
// handed in was empty would pass an empty pro.Privileges beside a populated
// proclassic.Privileges, resolving that gate from a registry describing none of
// those endpoints — a silently wrong answer, not an absent one.
func TestDeclaredScopesPanicsOnOneEmptyRegistryAmongSeveral(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("declaredScopes returned for one empty registry beside a populated one — " +
				"the emptiness guard keys on all registries rather than each")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "registry") {
			t.Errorf("panic message %q does not say a registry was empty", r)
		}
	}()
	declaredScopes("half-withdrawn family", nil,
		map[string]jamfplatform.MethodPrivileges{
			"Present": {Method: "Present", Scopes: []jamfplatform.ScopeKind{jamfplatform.ScopeEnvironment}},
		},
		map[string]jamfplatform.MethodPrivileges{},
	)
}

// TestDeclaredScopesPanicsWhenGivenNoRegistryAtAll covers the variadic's own
// empty case, which no caller writes today and which would otherwise resolve to
// no scope at all — read by RequireScope as "no scope requirement".
func TestDeclaredScopesPanicsWhenGivenNoRegistryAtAll(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("declaredScopes with no registry returned instead of panicking")
		}
	}()
	declaredScopes("family with no registry", nil)
}

func TestDeclaredScopesPanicsWhenNoScopeReachesEveryMethod(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("declaredScopes over disjoint scope sets returned instead of panicking")
		}
	}()
	declaredScopes("disjoint family", nil, map[string]jamfplatform.MethodPrivileges{
		"Environment": {Method: "Environment", Scopes: []jamfplatform.ScopeKind{jamfplatform.ScopeEnvironment}},
		"Tenant":      {Method: "Tenant", Scopes: []jamfplatform.ScopeKind{jamfplatform.ScopeTenant}},
	})
}

// TestScopeKindFromSDKMapsEveryKnownKind and its sibling cover the boundary the
// SDK could move under us. The SDK models organization as its zero value, so a
// default branch would map a fourth kind it grew onto organization — widening
// every construct that reads the family to a credential the endpoint refuses.
func TestScopeKindFromSDKMapsEveryKnownKind(t *testing.T) {
	for sdk, want := range map[jamfplatform.ScopeKind]ScopeKind{
		jamfplatform.ScopeOrganization: ScopeOrganization,
		jamfplatform.ScopeEnvironment:  ScopeEnvironment,
		jamfplatform.ScopeTenant:       ScopeTenant,
	} {
		if got := scopeKindFromSDK("family", "Method", sdk); got != want {
			t.Errorf("scopeKindFromSDK(%s) = %s, want %s", sdk, got, want)
		}
	}
}

func TestScopeKindFromSDKPanicsOnAnUnknownKind(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("scopeKindFromSDK returned for an unknown SDK kind instead of panicking")
		}
	}()
	scopeKindFromSDK("family", "Method", jamfplatform.ScopeKind(99))
}

// TestMethodScopesDeduplicates guards the intersection fold, which compares
// slices for equality in the uniformity test above: a registry entry listing one
// kind twice would otherwise read as a different set from the same kind once.
func TestMethodScopesDeduplicates(t *testing.T) {
	got := methodScopes("family", jamfplatform.MethodPrivileges{
		Method: "Method",
		Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeTenant, jamfplatform.ScopeTenant, jamfplatform.ScopeEnvironment,
		},
	})
	if want := []ScopeKind{ScopeTenant, ScopeEnvironment}; !slices.Equal(got, want) {
		t.Errorf("methodScopes = %s, want %s", names(got), names(want))
	}
}

// names renders a scope set the way a failure message should read.
func names(kinds []ScopeKind) string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k.String())
	}
	return "[" + strings.Join(out, " ") + "]"
}
