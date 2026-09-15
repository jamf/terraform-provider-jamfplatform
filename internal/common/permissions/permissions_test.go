// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package permissions

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/audit"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/ddmreport"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

func TestSection_BuildingCRUD(t *testing.T) {
	got := Section(pro.Privileges,
		"CreateBuildingV1", "GetBuildingV1", "UpdateBuildingV1", "DeleteBuildingV1")

	for _, want := range []string{
		"**Required Jamf permissions**",
		"| Category | Permission | Actions | API capability |",
		"| Organizational context | Buildings | Create, Read, Update, Delete | `buildings` |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Section() missing %q\n--- got ---\n%s", want, got)
		}
	}
	// One row per capability, not one per action.
	if n := strings.Count(got, "| `buildings` |"); n != 1 {
		t.Errorf("buildings should occupy one row, got %d\n%s", n, got)
	}
	// The pre-GA privilege names are gone for good.
	if strings.Contains(got, "Read Buildings") || strings.Contains(got, "Jamf Pro privilege") {
		t.Errorf("Section() still renders pre-GA privilege names\n%s", got)
	}
}

func TestSection_DeduplicatesAcrossMethods(t *testing.T) {
	// Read + List for the same resource both require buildings:read; the
	// table must list the capability once, with Read ticked once.
	got := Section(pro.Privileges, "GetBuildingV1", "ListBuildingsV1")
	if n := strings.Count(got, "| `buildings` |"); n != 1 {
		t.Fatalf("buildings should appear once, got %d\n%s", n, got)
	}
	if !strings.Contains(got, "| Buildings | Read | `buildings` |") {
		t.Fatalf("want a read-only Buildings row, got:\n%s", got)
	}
}

func TestSection_NoPrivilegeEndpoint(t *testing.T) {
	// Find a registry entry that requires no special permission and assert the
	// "None" wording renders.
	var none string
	for name, mp := range pro.Privileges {
		if len(mp.Scoped) == 0 {
			none = name
			break
		}
	}
	if none == "" {
		t.Skip("no zero-privilege method in registry")
	}
	got := Section(pro.Privileges, none)
	if !strings.Contains(got, "None beyond scope") {
		t.Errorf("expected the no-privilege wording for %s, got:\n%s", none, got)
	}
	if !strings.Contains(got, "**Platform environment** scope (preferred for new integrations) or **Tenant** scope") {
		t.Errorf("a permission-free endpoint still has a scope to state, got:\n%s", got)
	}
	if !strings.Contains(got, scopeCaveat) {
		t.Errorf("a permission-free endpoint still carries the scope caveat, got:\n%s", got)
	}
	if strings.Contains(got, "| Category |") {
		t.Errorf("expected no table for a permission-free endpoint, got:\n%s", got)
	}
}

// TestSection_NoPrivilegeWordingNeedsEveryMethodResolved covers the asymmetry
// between "no method needs a permission" and "no method resolved". The
// affirmative wording is a positive claim about the underlying endpoints, so a
// list mixing a permission-free method with one absent from the registry must
// render nothing rather than an assurance that was never read.
//
// synth builds entries carrying no Scopes, so this also exercises the
// scope-suppressed form of the wording — the fallback for a registry predating
// MethodPrivileges.Scopes, which no shipped registry is any longer.
func TestSection_NoPrivilegeWordingNeedsEveryMethodResolved(t *testing.T) {
	reg, names := synth([]string{})
	if got := Section(reg, names...); !strings.Contains(got, "None — any authenticated") {
		t.Fatalf("want the no-privilege wording when every method resolved, got:\n%s", got)
	}
	if got := Section(reg, append(slices.Clone(names), "NoSuchMethodXYZ")...); got != "" {
		t.Fatalf("want an empty section when a method is unresolved, got:\n%s", got)
	}
}

func TestSection_EmptyWhenAllMissing(t *testing.T) {
	if got := Section(pro.Privileges, "NoSuchMethodXYZ"); got != "" {
		t.Errorf("expected empty section for unknown method, got:\n%s", got)
	}
}

func TestMissing_DetectsUnknown(t *testing.T) {
	missing := Missing(pro.Privileges, "GetBuildingV1", "NoSuchMethodXYZ")
	if len(missing) != 1 || missing[0] != "NoSuchMethodXYZ" {
		t.Fatalf("want [NoSuchMethodXYZ], got %v", missing)
	}
}

func TestMerge(t *testing.T) {
	a := Registry{"X": jamfplatform.MethodPrivileges{Method: "X"}}
	b := Registry{"Y": jamfplatform.MethodPrivileges{Method: "Y"}}
	merged := Merge(a, b)
	if _, ok := merged["X"]; !ok {
		t.Error("merged registry missing X")
	}
	if _, ok := merged["Y"]; !ok {
		t.Error("merged registry missing Y")
	}
}

// --- rendering ----------------------------------------------------------------

// synth builds a registry from capability permission slices, so the rendering
// tests below state their input as the wire form rather than hunting the SDK
// for a method that happens to require the shape under test.
func synth(scoped ...[]string) (Registry, []string) {
	reg := make(Registry, len(scoped))
	names := make([]string, 0, len(scoped))
	for i, s := range scoped {
		name := string(rune('A' + i))
		reg[name] = jamfplatform.MethodPrivileges{Method: name, Scoped: s}
		names = append(names, name)
	}
	return reg, names
}

func TestActionList_FollowsPlatformOrder(t *testing.T) {
	// Alphabetical order would render "Create, Delete, Read, Update"; the
	// article's order is CRUD then deploy then execute.
	reg, names := synth([]string{
		"blueprints:execute", "blueprints:delete", "blueprints:deploy",
		"blueprints:create", "blueprints:update", "blueprints:read",
	})
	got := Section(reg, names...)
	if want := "| Blueprints | Create, Read, Update, Delete, Deploy, Execute | `blueprints` |"; !strings.Contains(got, want) {
		t.Fatalf("want %q, got:\n%s", want, got)
	}
}

// TestSection_RowsSortAlphabetically pins the sort to category name then
// permission name. Deployment sorts ahead of Inventory ahead of Organizational
// context, and Policies ahead of Scripts within Deployment — an order derived
// from the catalogue rather than from a hand-maintained list of sections.
func TestSection_RowsSortAlphabetically(t *testing.T) {
	reg, names := synth([]string{"policies:read", "buildings:read", "devices:read", "scripts:read"})
	got := Section(reg, names...)
	var order []int
	for _, row := range []string{"| Policies |", "| Scripts |", "| Devices |", "| Buildings |"} {
		i := strings.Index(got, row)
		if i < 0 {
			t.Fatalf("missing row %q:\n%s", row, got)
		}
		order = append(order, i)
	}
	if !slices.IsSorted(order) {
		t.Fatalf("rows out of alphabetical order (offsets %v):\n%s", order, got)
	}
}

// TestSection_UnresolvedRowsSortLast keeps the contract collect documents: a
// capability the catalogue does not know, and a value whose shape splitScoped
// rejected, both follow every row that resolved to picker names. Neither has a
// section to sort into, and both render as dashes, so sorting them among the
// real rows would put an unreadable row in the middle of a readable table.
func TestSection_UnresolvedRowsSortLast(t *testing.T) {
	reg, names := synth([]string{"zzz-unknown:read", "buildings:read", "devices"})
	got := Section(reg, names...)
	iBuildings := strings.Index(got, "| Organizational context | Buildings |")
	iUnknown := strings.Index(got, "`zzz-unknown`")
	iMalformed := strings.Index(got, "`devices`")
	if iBuildings < 0 || iUnknown < 0 || iMalformed < 0 {
		t.Fatalf("missing a row (buildings %d, unknown %d, malformed %d):\n%s",
			iBuildings, iUnknown, iMalformed, got)
	}
	if iBuildings > iUnknown || iBuildings > iMalformed {
		t.Fatalf("a resolved row sorted after an unresolved one:\n%s", got)
	}
}

// TestSplitScoped covers the shapes the renderer must refuse. The
// three-part beta slug is the one that used to slip through: strings.Cut takes
// the first colon, so "create:pro:buildings" split into the capability "create"
// and rendered a row naming a permission that does not exist.
func TestSplitScoped(t *testing.T) {
	for _, tc := range []struct {
		scoped     string
		capability string
		action     string
		ok         bool
	}{
		{"buildings:read", "buildings", "read", true},
		{":read", "", "", false},
		{"buildings:", "", "", false},
		{"create:pro:buildings", "", "", false},
		{"weird-shape", "", "", false},
		{"", "", "", false},
		{":", "", "", false},
	} {
		capability, action, ok := splitScoped(tc.scoped)
		if capability != tc.capability || action != tc.action || ok != tc.ok {
			t.Errorf("splitScoped(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.scoped, capability, action, ok, tc.capability, tc.action, tc.ok)
		}
	}
}

// TestSection_MalformedValueOnCataloguedCapabilityIsVisiblyBroken guards the
// worst rendering of an unparseable value: "buildings" alone resolves to the
// Buildings row with an empty Actions cell, which in Jamf Account's model is a
// permission granting nothing — and, because buildings IS catalogued, without
// the footnote that would explain it.
func TestSection_MalformedValueOnCataloguedCapabilityIsVisiblyBroken(t *testing.T) {
	reg, names := synth([]string{"buildings"})
	got := Section(reg, names...)
	if strings.Contains(got, "| Organizational context | Buildings |") {
		t.Errorf("malformed value rendered as a grantable Buildings row:\n%s", got)
	}
	if want := "| — | — | " + malformedAction + " | `buildings` |"; !strings.Contains(got, want) {
		t.Errorf("want %q, got:\n%s", want, got)
	}
	if !strings.Contains(got, "no Jamf Account name recorded for") {
		t.Errorf("malformed value should carry the explanatory footnote, got:\n%s", got)
	}
}

// TestSection_MalformedValueDoesNotMergeIntoWellFormedRow guards the quieter
// failure: an unparseable value sharing a capability with a well-formed one used
// to be absorbed into its row and leave no trace at all.
func TestSection_MalformedValueDoesNotMergeIntoWellFormedRow(t *testing.T) {
	reg, names := synth([]string{"buildings", "buildings:read"})
	got := Section(reg, names...)
	if want := "| Organizational context | Buildings | Read | `buildings` |"; !strings.Contains(got, want) {
		t.Errorf("want the well-formed row %q, got:\n%s", want, got)
	}
	if want := "| — | — | " + malformedAction + " | `buildings` |"; !strings.Contains(got, want) {
		t.Errorf("malformed value absorbed into the well-formed row, want %q, got:\n%s", want, got)
	}
	if n := strings.Count(got, "| `buildings` |"); n != 2 {
		t.Errorf("want two buildings rows, one real and one malformed, got %d:\n%s", n, got)
	}
}

// TestActionList_UnlabelledOrderedActionRendersAsSlugOnce pins actionOrder to
// actionLabels. An entry added to the order but not the labels used to append
// an empty string and then repeat itself through the extras path, rendering a
// cell like "Read, , annotate" whose blank the operator cannot act on.
func TestActionList_UnlabelledOrderedActionRendersAsSlugOnce(t *testing.T) {
	original := actionOrder
	actionOrder = append(slices.Clone(original), "annotate")
	t.Cleanup(func() { actionOrder = original })

	if got, want := actionList(map[string]bool{"read": true, "annotate": true}), "Read, annotate"; got != want {
		t.Fatalf("actionList() = %q, want %q", got, want)
	}
}

func TestSection_UncataloguedCapabilityIsMarkedNotGuessed(t *testing.T) {
	reg, names := synth([]string{"buildings:read", "brand-new-thing:read"})
	got := Section(reg, names...)
	if !strings.Contains(got, "| — | — | Read | `brand-new-thing` |") {
		t.Errorf("uncatalogued capability should render dashes, got:\n%s", got)
	}
	if !strings.Contains(got, "no Jamf Account name recorded for") {
		t.Errorf("uncatalogued capability should carry the explanatory note, got:\n%s", got)
	}
	// A known capability alongside it keeps its name, and the note is not
	// emitted for tables without an unknown.
	clean, cleanNames := synth([]string{"buildings:read"})
	if strings.Contains(Section(clean, cleanNames...), "no Jamf Account name recorded for") {
		t.Error("note emitted for a fully catalogued table")
	}
}

func TestSection_UnsplittableScopedSurvivesVerbatim(t *testing.T) {
	// Defensive: the SDK documents the two-part GA form as the only one it
	// emits, so a value without an action is a shape we have never seen. It
	// must still appear rather than be silently dropped.
	reg, names := synth([]string{"weird-shape"})
	got := Section(reg, names...)
	if !strings.Contains(got, "`weird-shape`") {
		t.Fatalf("unsplittable capability dropped:\n%s", got)
	}
}

// --- catalogue drift ----------------------------------------------------------

// sdkRegistries is every privilege registry the SDK publishes. Listed
// explicitly because Go has no way to enumerate a module's packages at
// runtime; a new SDK sub-package is added here by hand, which is the same
// motion as wiring its resources up.
func sdkRegistries() map[string]Registry {
	return map[string]Registry{
		"account":              account.Privileges,
		"aigovernance":         aigovernance.Privileges,
		"audit":                audit.Privileges,
		"blueprints":           blueprints.Privileges,
		"compliancebenchmarks": compliancebenchmarks.Privileges,
		"ddmreport":            ddmreport.Privileges,
		"deviceactions":        deviceactions.Privileges,
		"devicegroups":         devicegroups.Privileges,
		"devices":              devices.Privileges,
		"pro":                  pro.Privileges,
		"proclassic":           proclassic.Privileges,
		"securitycloud":        securitycloud.Privileges,
	}
}

// registryPair names one unordered pair of SDK registries and the method name
// both of them key, which is the shape a Merge collision takes.
type registryPair struct {
	a, b, method string
}

// knownRegistryCollisions is every method name two SDK registries both key as
// of the pinned SDK. There is one, and it is a real conflict rather than a
// harmless duplicate: aigovernance.ListPolicies requires ai-policies:read while
// proclassic.ListPolicies requires policies:read, so a construct merging those
// two registries would document whichever was passed to Merge last and grant the
// operator a permission that does not reach its endpoint.
//
// The entries disagree on scope as well, which doubles the cost of the collision
// now that collect reads Scopes off the winning entry too: the aigovernance
// entry declares environment alone and the proclassic one declares tenant and
// environment, so the losing order does not merely name the wrong
// permission — it either withholds a scope the construct accepts or claims one
// its endpoint refuses, in the same rendered sentence that tells the operator
// how to create the integration.
//
// It is tolerated as an exception rather than fixed here because no construct
// merges that pair — every Merge call in the provider is pro+proclassic or
// pro+devices, both fully disjoint — and the fix belongs in the SDK's method
// naming, not in a renderer. The exception is deliberately keyed on the pair as
// well as the method, so it excuses ListPolicies only between these two
// registries and not the same name colliding anywhere else. What it cannot
// detect is a future construct merging this very pair: the entry is safe only
// while nothing does, so a Merge call naming aigovernance alongside proclassic
// has to delete it and take the SDK rename instead.
var knownRegistryCollisions = map[registryPair]bool{
	{a: "aigovernance", b: "proclassic", method: "ListPolicies"}: true,
}

// TestSDKRegistriesAreDisjoint is the guard on what Merge can safely do. Merge
// resolves a key collision by letting the last registry win, which is only
// sound while the colliding entries agree — and across families they do not,
// because the same method name can front two different endpoints requiring two
// different capabilities. Nothing else in the repo checks this: TestMerge
// exercises a synthetic pair that is disjoint by construction, so it can never
// fail, and a per-construct drift guard sees only the merged result, by which
// point the losing entry is gone without trace.
//
// Every pair is compared, not just the pairs some construct merges today,
// because a construct that merges a new pair is a one-line change and the
// failure it would cause is silent: a published permission table naming the
// wrong permission, with the resource still working for whoever granted both.
// A new collision therefore fails here, at the point the SDK introduces it,
// rather than at the point someone unknowingly relies on it.
func TestSDKRegistriesAreDisjoint(t *testing.T) {
	regs := sdkRegistries()
	names := slices.Sorted(maps.Keys(regs))

	seen := map[registryPair]bool{}
	for i, a := range names {
		for _, b := range names[i+1:] {
			for method := range regs[a] {
				if _, dup := regs[b][method]; !dup {
					continue
				}
				pair := registryPair{a: a, b: b, method: method}
				seen[pair] = true
				if knownRegistryCollisions[pair] {
					continue
				}
				t.Errorf("%s.Privileges and %s.Privileges both key %q — Merge would silently drop one "+
					"(%s requires %v at %v, %s requires %v at %v); rename in the SDK, or record it in "+
					"knownRegistryCollisions with the reason it is harmless",
					a, b, method,
					a, regs[a][method].Scoped, regs[a][method].Scopes,
					b, regs[b][method].Scoped, regs[b][method].Scopes)
			}
		}
	}

	for pair := range knownRegistryCollisions {
		if !seen[pair] {
			t.Errorf("knownRegistryCollisions still excuses %s/%s %q, which no longer collides — delete the entry",
				pair.a, pair.b, pair.method)
		}
	}
}

// TestCatalogueCoversEverySDKCapability is the drift guard on catalogue.go.
// Jamf's permissions map is documentation, not a machine-readable artefact, so
// the transcription cannot be regenerated — this test is what tells us the
// article has moved on. A failure is not a bug in the renderer: it means a
// capability now reachable through the SDK has no Jamf Account permission name
// recorded, and the fix is to add the row from the current article.
func TestCatalogueCoversEverySDKCapability(t *testing.T) {
	type where struct{ pkg, method string }
	missing := map[string]where{}
	for pkgName, reg := range sdkRegistries() {
		for method, mp := range reg {
			for _, scoped := range mp.Scoped {
				capability, action, ok := splitScoped(scoped)
				if !ok {
					t.Errorf("%s.%s: scoped permission %q is not {capability}:{action}", pkgName, method, scoped)
					continue
				}
				if _, known := actionLabels[action]; !known {
					t.Errorf("%s.%s: scoped permission %q uses unknown action %q", pkgName, method, scoped, action)
				}
				if _, known := catalogue[capability]; !known {
					if _, dup := missing[capability]; !dup {
						missing[capability] = where{pkgName, method}
					}
				}
			}
		}
	}
	for capability, w := range missing {
		t.Errorf("capability %q (required by %s.%s) has no catalogue entry — add it from Jamf's permissions map",
			capability, w.pkg, w.method)
	}
}

// TestCataloguePermissionNamesAreUniquePerCategory catches a copy-paste slip in
// the transcription: two capabilities pointing at the same picker row.
func TestCataloguePermissionNamesAreUniquePerCategory(t *testing.T) {
	seen := map[entry]string{}
	for capability, e := range catalogue {
		if prev, dup := seen[e]; dup {
			t.Errorf("capabilities %q and %q both map to %s / %s", prev, capability, e.category, e.name)
			continue
		}
		seen[e] = capability
	}
}

// --- Renders ------------------------------------------------------------------

func TestRenders(t *testing.T) {
	section := Section(pro.Privileges,
		"CreateBuildingV1", "GetBuildingV1", "UpdateBuildingV1", "DeleteBuildingV1")

	for _, want := range []string{
		"buildings:create", "buildings:read", "buildings:update", "buildings:delete",
	} {
		if !Renders(section, want) {
			t.Errorf("Renders(%q) = false, want true\n%s", want, section)
		}
	}
	for _, notWant := range []string{
		"buildings:deploy",   // right row, box not ticked
		"buildings:execute",  // right row, box not ticked
		"departments:read",   // wrong row entirely
		"building:read",      // capability is not a prefix match
		"buildings",          // not a scoped permission
		"buildings:nonsense", // action outside the six
	} {
		if Renders(section, notWant) {
			t.Errorf("Renders(%q) = true, want false\n%s", notWant, section)
		}
	}
}

func TestRenders_IgnoresNonTableText(t *testing.T) {
	if Renders("**Required Jamf permissions**\n\nNone — any authenticated integration", "buildings:read") {
		t.Error("Renders matched the no-permission wording")
	}
	if Renders("", "buildings:read") {
		t.Error("Renders matched an empty section")
	}
}

// TestSection_StatesTheIntegrationScope covers the scope clause the lead-in
// opens with, per family, against the real registries. The three shapes it can
// take — one scope, two with the first recommended, and the organization case —
// each have a shipped family, so nothing here is synthetic.
//
// The clause is deliberately not an instruction to create an integration with
// the named scope. It reports where the endpoints are published and defers to
// the Configure-time diagnostic, because a page telling an operator whose
// tenant-scoped credential already works to register a replacement is the
// failure this wording exists to avoid.
//
// The last case is the one that pins that divergence rather than merely
// permitting it. Platform device groups are declared environment-only by their
// specification and accepted under tenant scope by providerdata.RequireScope,
// which reads providerdata.gatewayWidenings and this package deliberately does
// not: a published table carries Jamf's own declaration, and the provider's
// evidenced tolerances live where they are enforced. Without a widened family
// among the cases, teaching this package about the widenings — or dropping the
// omission — would rewrite the clause on every page of three families with a
// green suite.
func TestSection_StatesTheIntegrationScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reg     Registry
		method  string
		want    string
		notWant string
	}{
		{name: "pro is published at both", reg: pro.Privileges, method: "GetCategoryV1",
			want: "Jamf lists this under **Platform environment** scope " +
				"(preferred for new integrations) or **Tenant** scope."},
		{name: "ai governance is environment only", reg: aigovernance.Privileges, method: "ListPolicies",
			want:    "Jamf lists this under **Platform environment** scope.",
			notWant: "Tenant"},
		{name: "account is organization only", reg: account.Privileges, method: "ListDomains",
			want:    "Jamf lists this under **Organization management** scope.",
			notWant: "Tenant"},
		{name: "a widened family still reports its specification", reg: devicegroups.Privileges, method: "ListDeviceGroups",
			want:    "Jamf lists this under **Platform environment** scope.",
			notWant: "Tenant"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := tc.reg[tc.method]; !ok {
				t.Fatalf("%s is no longer in the registry — pick another method for this case", tc.method)
			}
			got := Section(tc.reg, tc.method)
			if !strings.Contains(got, tc.want) {
				t.Errorf("want the lead-in to open %q, got:\n%s", tc.want, got)
			}
			if !strings.Contains(got, scopeCaveat) {
				t.Errorf("want the scope caveat alongside the clause, got:\n%s", got)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("the clause must not name %q, got:\n%s", tc.notWant, got)
			}
		})
	}
}

// TestSection_ScopeIsIntersectedAcrossMethods pins the direction of the fold at
// the rendered level. A construct calls every one of its methods through a
// single client, so a union would send an operator to create an integration one
// of the endpoints refuses.
func TestSection_ScopeIsIntersectedAcrossMethods(t *testing.T) {
	reg := Registry{
		"Both": {Method: "Both", Scoped: []string{"categories:read"}, Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeEnvironment, jamfplatform.ScopeTenant,
		}},
		"EnvironmentOnly": {Method: "EnvironmentOnly", Scoped: []string{"categories:read"}, Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeEnvironment,
		}},
	}
	got := Section(reg, "Both", "EnvironmentOnly")
	if !strings.Contains(got, "Jamf lists this under **Platform environment** scope.") {
		t.Errorf("want the intersection, got:\n%s", got)
	}
	if strings.Contains(got, "Tenant") {
		t.Errorf("a method refusing tenant must remove it from the claim, got:\n%s", got)
	}
}

// TestSection_ScopeSuppressedWhenAMethodDeclaresNone asserts a registry entry
// with no Scopes drops the clause rather than answering from the entries that do
// carry one. Every shipped registry now populates the field, so this covers a
// downgrade — an SDK old enough to predate it — where a partial answer rendered
// as a complete one would send an operator to the wrong integration.
func TestSection_ScopeSuppressedWhenAMethodDeclaresNone(t *testing.T) {
	reg := Registry{
		"Scoped": {Method: "Scoped", Scoped: []string{"categories:read"}, Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeEnvironment,
		}},
		"Unscoped": {Method: "Unscoped", Scoped: []string{"categories:read"}},
	}
	got := Section(reg, "Scoped", "Unscoped")
	if strings.Contains(got, "Jamf lists this under") {
		t.Errorf("want no scope claim when a method declares none, got:\n%s", got)
	}
	if !strings.Contains(got, "Grant the API integration the following permissions") {
		t.Errorf("want the scope-free lead-in, got:\n%s", got)
	}
}

// TestSection_ScopeSuppressedForAnUnlabelledKind covers a scope kind the SDK
// grows that this package has no Jamf Account label for. Printing the kinds it
// does recognise would understate what the endpoints accept, so the clause goes
// entirely.
//
// This case reaches the suppression through the intersection rather than
// through scopePhrase's own guard: a kind absent from scopeOrder cannot survive
// the fold, so the set arriving at scopePhrase is empty. The mixed-set case
// below is what exercises the guard itself.
func TestSection_ScopeSuppressedForAnUnlabelledKind(t *testing.T) {
	reg := Registry{
		"Method": {Method: "Method", Scoped: []string{"categories:read"}, Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeKind(99),
		}},
	}
	if got := Section(reg, "Method"); strings.Contains(got, "Jamf lists this under") {
		t.Errorf("want no scope claim for an unlabelled kind, got:\n%s", got)
	}
}

// TestSection_ScopeSuppressedForAMixedLabelledSet is the case that pins
// scopePhrase's all-or-nothing rule: one kind labelled, another not, must render
// no clause at all rather than the half it can name, which would understate what
// the endpoints accept and send an operator to create an integration narrower
// than they need.
//
// Reaching it takes the scopeOrder mutation. intersectScopes folds against
// scopeOrder, so a kind this package does not order is dropped before
// scopePhrase ever sees it — meaning the guard is live only for a kind that is
// ordered and unlabelled, which is precisely the drift the size check in
// TestScopeLabelsCoverEveryKnownKind exists to fail on. Setting that state up is
// what makes the guard's behaviour asserted rather than assumed: without it,
// relaxing the guard to skip the unlabelled kind and print the rest passes every
// other test in this file.
func TestSection_ScopeSuppressedForAMixedLabelledSet(t *testing.T) {
	unlabelled := jamfplatform.ScopeKind(99)
	original := scopeOrder
	scopeOrder = append(slices.Clone(original), unlabelled)
	t.Cleanup(func() { scopeOrder = original })

	reg := Registry{
		"Method": {Method: "Method", Scoped: []string{"categories:read"}, Scopes: []jamfplatform.ScopeKind{
			jamfplatform.ScopeEnvironment, unlabelled,
		}},
	}
	got := Section(reg, "Method")
	if strings.Contains(got, "Jamf lists this under") {
		t.Errorf("want no scope claim when one of the kinds has no label, got:\n%s", got)
	}
	if !strings.Contains(got, "Grant the API integration the following permissions") {
		t.Errorf("want the scope-free lead-in, got:\n%s", got)
	}
}

// TestSection_ScopeSuppressedWhenAMethodIsMissing covers the third way a fold
// can be partial: a method the registry does not carry contributes no Scopes at
// all, so the intersection is computed from a strict subset of what the
// construct calls while reading as the whole answer. A scope every resolved
// method accepts can still be refused by the one that was skipped, so the table
// renders without a clause and the drift-guard test is left to fail on the
// missing method.
func TestSection_ScopeSuppressedWhenAMethodIsMissing(t *testing.T) {
	got := Section(pro.Privileges, "GetCategoryV1", "NoSuchMethodXYZ")
	if !strings.Contains(got, "| Category | Permission | Actions | API capability |") {
		t.Fatalf("want the table to still render, got:\n%s", got)
	}
	if strings.Contains(got, "Jamf lists this under") {
		t.Errorf("want no scope claim when a method is unresolved, got:\n%s", got)
	}
}

// TestScopeLabelsCoverEveryKnownKind pins the two scope tables to each other
// and to the three kinds this package knows: each is labelled, each is ordered,
// and the tables are the same size, so a kind added to one and forgotten in the
// other cannot silently stop the clause rendering across every construct at
// once.
//
// It does not catch an SDK that grows a fourth kind. The kinds are iterated from
// a literal because there is nothing to iterate instead — jamfplatform.ScopeKind
// is an alias of the client package's type, three explicit constants with no
// generated values helper — so a new kind adds nothing here to fail on. The
// guard for that is providerdata.scopeKindFromSDK, which panics at package
// initialisation on a kind it has no mapping for rather than defaulting to one;
// `go test ./...` reaches it before any build ships. This package's own exposure
// to a fourth kind is bounded by scopePhrase, which suppresses the whole clause
// when any kind is unlabelled.
func TestScopeLabelsCoverEveryKnownKind(t *testing.T) {
	for _, k := range []jamfplatform.ScopeKind{
		jamfplatform.ScopeOrganization, jamfplatform.ScopeEnvironment, jamfplatform.ScopeTenant,
	} {
		if _, ok := scopeLabels[k]; !ok {
			t.Errorf("scopeLabels has no Jamf Account label for %s", k)
		}
		if !slices.Contains(scopeOrder, k) {
			t.Errorf("scopeOrder does not carry %s, so it can never be rendered", k)
		}
	}
	if len(scopeLabels) != len(scopeOrder) {
		t.Errorf("scopeLabels has %d entries and scopeOrder %d — a kind in one and not the other "+
			"either renders unordered or never renders", len(scopeLabels), len(scopeOrder))
	}
}
