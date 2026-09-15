// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// stubPolicySource is an in-memory PolicySource, so the sweep, the reverse index
// and the union arithmetic are all tested without HTTP.
type stubPolicySource struct {
	ids      []string
	policies map[string]*proclassic.Policy
	// listErr fails the initial listing, which is the one fatal failure.
	listErr error
	// policyErrs fails individual policy reads, which must be survivable.
	policyErrs map[string]error
	// delay holds each read briefly. Without it the stub returns so fast that
	// workers never overlap and maxConcurrent stays at 1, which would make the
	// concurrency assertion vacuous rather than wrong.
	delay time.Duration

	mu        sync.Mutex
	listCalls int
	getCalls  map[string]int
	// maxConcurrent records the high-water mark of simultaneous reads, so the
	// sweep's concurrency bound can be asserted rather than assumed.
	inFlight      int
	maxConcurrent int
}

func (s *stubPolicySource) PolicyIDs(_ context.Context) ([]string, error) {
	s.mu.Lock()
	s.listCalls++
	s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.ids, nil
}

func (s *stubPolicySource) Policy(_ context.Context, id string) (*proclassic.Policy, error) {
	s.mu.Lock()
	if s.getCalls == nil {
		s.getCalls = map[string]int{}
	}
	s.getCalls[id]++
	s.inFlight++
	if s.inFlight > s.maxConcurrent {
		s.maxConcurrent = s.inFlight
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	if err, ok := s.policyErrs[id]; ok {
		return nil, err
	}
	return s.policies[id], nil
}

// --- builders, so each test states only what it cares about ---
//
// The wire types are pointer-heavy, so the fixtures lean on new(expr) to take
// the address of a literal inline.

type policyOpt func(*proclassic.Policy)

func testPolicy(id int, name string, opts ...policyOpt) *proclassic.Policy {
	p := &proclassic.Policy{
		General: &proclassic.PolicyGeneral{
			ID: new(id), Name: new(name), Enabled: new(true),
		},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func disabled() policyOpt {
	return func(p *proclassic.Policy) { p.General.Enabled = new(false) }
}

func withScripts(ids ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.PolicyScriptsScriptItem, 0, len(ids))
		for _, id := range ids {
			items = append(items, proclassic.PolicyScriptsScriptItem{ID: new(id)})
		}
		p.Scripts = &proclassic.PolicyScripts{Script: &items}
	}
}

func withPackages(ids ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.PolicyPackageConfigurationPackagesPackageItem, 0, len(ids))
		for _, id := range ids {
			items = append(items, proclassic.PolicyPackageConfigurationPackagesPackageItem{ID: new(id)})
		}
		p.PackageConfiguration = &proclassic.PolicyPackageConfiguration{
			Packages: &proclassic.PolicyPackageConfigurationPackages{Package: &items},
		}
	}
}

func withPrinters(ids ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.PolicyPrintersPrinterItem, 0, len(ids))
		for _, id := range ids {
			items = append(items, proclassic.PolicyPrintersPrinterItem{ID: new(id)})
		}
		p.Printers = &proclassic.PolicyPrinters{Printer: &items}
	}
}

func withDockItems(ids ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.PolicyDockItemsDockItemItem, 0, len(ids))
		for _, id := range ids {
			items = append(items, proclassic.PolicyDockItemsDockItemItem{ID: new(id)})
		}
		p.DockItems = &proclassic.PolicyDockItems{DockItem: &items}
	}
}

func withDirectoryBindings(ids ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.IDName, 0, len(ids))
		for _, id := range ids {
			items = append(items, proclassic.IDName{ID: new(id)})
		}
		p.AccountMaintenance = &proclassic.PolicyAccountMaintenance{
			DirectoryBindings: &proclassic.PolicyAccountMaintenanceDirectoryBindings{Binding: &items},
		}
	}
}

func withDiskEncryption(applyID, remediateID *int) policyOpt {
	return func(p *proclassic.Policy) {
		p.DiskEncryption = &proclassic.PolicyDiskEncryption{
			DiskEncryptionConfigurationID:          applyID,
			RemediateDiskEncryptionConfigurationID: remediateID,
		}
	}
}

// scopeOf returns the policy's scope block, creating it on first use so the scope
// builders compose in any order rather than the last one silently winning.
func scopeOf(p *proclassic.Policy) *proclassic.PolicyScope {
	if p.Scope == nil {
		p.Scope = &proclassic.PolicyScope{}
	}
	return p.Scope
}

// withGroupScope scopes the policy to computer groups by numeric id.
func withGroupScope(groupIDs ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.IDName, 0, len(groupIDs))
		for _, id := range groupIDs {
			items = append(items, proclassic.IDName{ID: new(id)})
		}
		scopeOf(p).ComputerGroups = &proclassic.PolicyScopeComputerGroups{ComputerGroup: &items}
	}
}

// withExcludedGroups excludes computer groups by numeric id.
func withExcludedGroups(groupIDs ...int) policyOpt {
	return func(p *proclassic.Policy) {
		items := make([]proclassic.IDName, 0, len(groupIDs))
		for _, id := range groupIDs {
			items = append(items, proclassic.IDName{ID: new(id)})
		}
		scopeOf(p).Exclusions = &proclassic.PolicyScopeExclusions{
			ComputerGroups: &proclassic.PolicyScopeExclusionsComputerGroups{ComputerGroup: &items},
		}
	}
}

func withAllComputers() policyOpt {
	return func(p *proclassic.Policy) {
		scopeOf(p).AllComputers = new(true)
	}
}

// depCache builds a Cache wired to both stub sources.
func depCache(groups *stubSource, policies *stubPolicySource) *Cache {
	return NewCacheWithPolicies(groups, policies)
}

// twoGroupTenant is a tenant with two overlapping computer groups, which is what
// makes the union arithmetic observable: group 1 and group 2 share device "b".
func twoGroupTenant() *stubSource {
	return &stubSource{
		groups: []Group{
			{PlatformID: "u1", JamfProID: "1", Name: "Group One", DeviceType: DeviceTypeComputer, MembershipCount: 2},
			{PlatformID: "u2", JamfProID: "2", Name: "Group Two", DeviceType: DeviceTypeComputer, MembershipCount: 2},
		},
		totals:  Totals{ManagedComputers: 4},
		members: map[string][]string{"u1": {"a", "b"}, "u2": {"b", "c"}},
	}
}

// TestPolicyUses_IndexesEveryDependencyKind covers all six DependencyKind values.
//
// All six deliberately, not a representative sample: each kind is a separate block
// in policyDependencies reading a differently-shaped wire section, so a kind with no
// case here is a kind whose block can be deleted with the suite still green.
func TestPolicyUses_IndexesEveryDependencyKind(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{
		ids: []string{"10", "11", "12"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Runs script", withScripts(500), withPackages(80)),
			"11": testPolicy(11, "Encrypts", withDiskEncryption(new(59), new(60))),
			"12": testPolicy(12, "Sets up the desk",
				withPrinters(31), withDockItems(41), withDirectoryBindings(51)),
		},
	}
	c := depCache(twoGroupTenant(), src)

	for _, tc := range []struct {
		kind DependencyKind
		id   string
		want string
	}{
		{DependencyScript, "500", "Runs script"},
		{DependencyPackage, "80", "Runs script"},
		{DependencyPrinter, "31", "Sets up the desk"},
		{DependencyDockItem, "41", "Sets up the desk"},
		{DependencyDirectoryBinding, "51", "Sets up the desk"},
		{DependencyDiskEncryptionConfiguration, "59", "Encrypts"},
		// The remediate field makes the policy depend on that configuration too, so
		// editing configuration 60 must also be reported.
		{DependencyDiskEncryptionConfiguration, "60", "Encrypts"},
	} {
		uses, stats, err := c.PolicyUses(context.Background(), tc.kind, tc.id)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.kind, tc.id, err)
		}
		if stats.Searched != 3 || stats.Unreadable != 0 {
			t.Errorf("%s %s: stats = %+v, want 3 searched and none unreadable", tc.kind, tc.id, stats)
		}
		if len(uses) != 1 || uses[0].Name != tc.want {
			t.Errorf("%s %s: uses = %+v, want one use named %q", tc.kind, tc.id, uses, tc.want)
		}
	}
}

func TestPolicyDependencies_CountsOnePolicyOncePerObject(t *testing.T) {
	t.Parallel()
	// Apply and remediate pointing at the same configuration is an ordinary setup —
	// both fields are Optional+Computed on the policy resource. Indexed twice, one
	// policy reads as "via 2 policies", lists its name twice, and pushes combineScopes
	// off its exact single-policy path, so an exact figure becomes an inflated "up to".
	shared := 59
	src := &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Encrypts", withDiskEncryption(&shared, &shared), withGroupScope(1)),
		},
	}
	c := depCache(twoGroupTenant(), src)

	uses, _, err := c.PolicyUses(context.Background(), DependencyDiskEncryptionConfiguration, "59")
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 1 {
		t.Fatalf("uses = %d, want 1 — one policy referencing one object twice is one use", len(uses))
	}

	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyDiskEncryptionConfiguration,
		ID: "59", Name: "FileVault", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	if !strings.Contains(summary, "via 1 policy") {
		t.Errorf("summary must count the policy once: %q", summary)
	}
	// Group One holds two devices and the policy excludes nothing, so a single
	// contributor keeps its exclusions and the figure stays exact — the doubled use
	// showed up as "up to" here.
	if !strings.Contains(summary, "2 of 4 computers") || strings.Contains(summary, "up to") {
		t.Errorf("summary must stay exact: %q", summary)
	}
	if strings.Count(detail, "Encrypts") != 1 {
		t.Errorf("detail names the policy more than once:\n%s", detail)
	}
}

func TestPolicyDependencies_IgnoresUnsetZeroIDs(t *testing.T) {
	t.Parallel()
	// The Classic API emits 0 for an unset disk-encryption field rather than omitting
	// it, so a policy that encrypts nothing would otherwise index a reference to
	// configuration "0".
	src := &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Encrypts nothing", withDiskEncryption(new(0), new(0)), withGroupScope(1)),
		},
	}
	c := depCache(twoGroupTenant(), src)
	uses, _, err := c.PolicyUses(context.Background(), DependencyDiskEncryptionConfiguration, "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 0 {
		t.Errorf("uses = %+v, want none — id 0 means the field is unset", uses)
	}
}

func TestPolicyDependencies_AbsentGeneralFailsOpen(t *testing.T) {
	t.Parallel()
	// A policy whose General block did not come back must be treated as enabled: as
	// disabled it would be listed but contribute no devices, understating the figure,
	// and it would be named "policy " with a dangling trailing space.
	p := &proclassic.Policy{}
	withScripts(500)(p)
	use, refs := policyDependencies(p)
	if !use.Enabled {
		t.Error("an absent General block must fail open to enabled, as an absent Enabled flag does")
	}
	if strings.TrimSpace(use.Name) != use.Name || use.Name == "" {
		t.Errorf("Name = %q, want a name that is not a dangling prefix", use.Name)
	}
	if len(refs) != 1 {
		t.Errorf("refs = %+v, want the script reference regardless of the missing identity", refs)
	}
}

func TestPolicyUses_SweepsOnceAcrossKindsAndConcurrentCallers(t *testing.T) {
	t.Parallel()
	ids := make([]string, 0, 30)
	policies := map[string]*proclassic.Policy{}
	for i := 1; i <= 30; i++ {
		id := string(rune('0'+i/10)) + string(rune('0'+i%10))
		ids = append(ids, id)
		policies[id] = testPolicy(i, "Policy "+id, withScripts(500))
	}
	src := &stubPolicySource{ids: ids, policies: policies, delay: 2 * time.Millisecond}
	c := depCache(twoGroupTenant(), src)

	// Terraform evaluates resources concurrently, so several dependency resources
	// can ask at once; they must share one sweep rather than each starting theirs.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, _, err := c.PolicyUses(context.Background(), DependencyScript, "500"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	// And a different kind reuses the same sweep.
	if _, _, err := c.PolicyUses(context.Background(), DependencyPackage, "80"); err != nil {
		t.Fatal(err)
	}

	if src.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1", src.listCalls)
	}
	for id, n := range src.getCalls {
		if n != 1 {
			t.Errorf("policy %s read %d times, want 1", id, n)
		}
	}
	// The literal 5 is deliberate. Jamf's API scalability guidance is the fixed bound
	// this has to respect, so the assertion pins that number rather than whatever
	// dependencySweepConcurrency currently says — compared against the constant, the
	// two move together and raising it to 50 leaves the test green.
	const jamfConcurrentConnectionLimit = 5
	if src.maxConcurrent > jamfConcurrentConnectionLimit {
		t.Errorf("maxConcurrent = %d, exceeds the %d concurrent connections Jamf's scalability guidance asks for",
			src.maxConcurrent, jamfConcurrentConnectionLimit)
	}
	if src.maxConcurrent < 2 {
		t.Errorf("maxConcurrent = %d, sweep did not run concurrently at all", src.maxConcurrent)
	}
}

func TestPolicyUses_SurvivesUnreadablePolicyButNotUnreadableListing(t *testing.T) {
	t.Parallel()

	// One bad policy costs that policy's contribution, not the whole alert.
	src := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Good", withScripts(500)),
		},
		policyErrs: map[string]error{"11": errors.New("boom")},
	}
	c := depCache(twoGroupTenant(), src)
	uses, stats, err := c.PolicyUses(context.Background(), DependencyScript, "500")
	if err != nil {
		t.Fatalf("one unreadable policy must not fail the sweep: %v", err)
	}
	if len(uses) != 1 {
		t.Errorf("uses = %d, want 1", len(uses))
	}
	// Only the policy that was actually read counts as searched. Reporting 2 would let
	// the alert say "no policy uses this — searched 2 policies" while one of the two
	// was never opened, which is the confident-but-wrong denial this whole design is
	// built to avoid.
	if stats.Searched != 1 || stats.Unreadable != 1 {
		t.Errorf("stats = %+v, want 1 searched and 1 unreadable", stats)
	}
	if stats.Complete() || stats.Listed() != 2 {
		t.Errorf("stats = %+v, want an incomplete sweep over 2 listed policies", stats)
	}

	// An unreadable listing is fatal: an empty index would read as "nothing uses this".
	bad := depCache(twoGroupTenant(), &stubPolicySource{listErr: errors.New("nope")})
	if _, _, err := bad.PolicyUses(context.Background(), DependencyScript, "500"); err == nil {
		t.Error("unreadable listing must surface an error, not an empty index")
	}
}

func TestReportDependency_PartialSweepIsDisclosedNotDenied(t *testing.T) {
	t.Parallel()

	// The dangerous case: the one policy using the script is the one that could not
	// be read, so the index holds nothing for it. "No policy uses this script" would
	// be a flat denial of something the sweep never checked.
	unread := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Unrelated", withPackages(80)),
		},
		policyErrs: map[string]error{"11": errors.New("boom")},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), unread), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	if strings.Contains(summary, "no policy uses this") {
		t.Errorf("a partial sweep must not deny usage outright: %q", summary)
	}
	if !strings.Contains(summary, "the search was incomplete") {
		t.Errorf("the shortfall must reach the headline: %q", summary)
	}
	for _, want := range []string{"Searched 1 policy of 2", "1 could not be read"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q:\n%s", want, detail)
		}
	}

	// And when uses were found anyway, the figure still stands but the shortfall is
	// disclosed alongside it, since another policy may add to the audience.
	found := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1)),
		},
		policyErrs: map[string]error{"11": errors.New("boom")},
	}
	diags = ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), found), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail = diags[0].Summary(), diags[0].Detail()
	if !strings.Contains(detail, "Searched 1 policy of 2 (1 could not be read") {
		t.Errorf("detail must disclose the shortfall next to the count it qualifies:\n%s", detail)
	}
	if strings.Contains(detail, "Searched 2 polic") {
		t.Errorf("detail claims policies it never read:\n%s", detail)
	}
	// The shortfall is a direction, not just a sentence: an unread policy can only add
	// audience, so the figure is a lower bound and the qualifier has to say so.
	if !strings.Contains(summary, "2 or more of 4 computers") {
		t.Errorf("an incomplete sweep must render the figure as a lower bound: %q", summary)
	}
	if !strings.Contains(detail, "the true figure may be higher") ||
		!strings.Contains(detail, "unread policies (1)") {
		t.Errorf("the unread policies must appear among the caveats:\n%s", detail)
	}
	t.Logf("partial sweep with uses:\n--- summary ---\n%s\n--- detail ---\n%s", summary, detail)
}

func TestReportDependency_PartialSweepAndDroppedExclusionsBoundNeitherWay(t *testing.T) {
	t.Parallel()
	// Two directions at once: an unread policy can only add devices, a dropped exclusion
	// can only remove them, so the figure bounds the truth from neither side and reads
	// "an estimated". Correct rather than a defect — asserted here so nobody later
	// reads it as one and "fixes" it into a false one-sided claim.
	src := &stubPolicySource{
		ids: []string{"10", "11", "12"},
		policies: map[string]*proclassic.Policy{
			// Alpha excludes Group Two, Beta excludes nothing, so the exclusion is
			// carried by only one contributor and cannot be subtracted.
			"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1), withExcludedGroups(2)),
			"11": testPolicy(11, "Beta", withScripts(500), withGroupScope(2)),
		},
		policyErrs: map[string]error{"12": errors.New("boom")},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), src), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	if !strings.Contains(summary, "an estimated 3 of 4 computers") {
		t.Errorf("inputs pulling both ways must bound the figure from neither side: %q", summary)
	}
	for _, want := range []string{"unread policies (1)", "policy exclusions (1)"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q:\n%s", want, detail)
		}
	}
	t.Logf("both directions:\n--- summary ---\n%s\n--- detail ---\n%s", summary, detail)
}

func TestPolicyUses_MemoisesFailure(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{listErr: errors.New("nope")}
	c := depCache(twoGroupTenant(), src)
	for range 3 {
		if _, _, err := c.PolicyUses(context.Background(), DependencyScript, "500"); err == nil {
			t.Fatal("want error")
		}
	}
	if src.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1 — a failed sweep must not be retried per resource", src.listCalls)
	}
}

func TestPolicyUses_DisabledCacheAndEmptyIDReportNothing(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{ids: []string{"10"},
		policies: map[string]*proclassic.Policy{"10": testPolicy(10, "P", withScripts(500))}}

	// A cache without a policy source is the state of a provider whose impact
	// alerts are on but which was built without dependency support.
	noPolicies := NewCache(twoGroupTenant())
	if uses, _, err := noPolicies.PolicyUses(context.Background(), DependencyScript, "500"); err != nil || uses != nil {
		t.Errorf("cache without policy source: uses=%v err=%v, want nil/nil", uses, err)
	}
	var nilCache *Cache
	if uses, _, err := nilCache.PolicyUses(context.Background(), DependencyScript, "500"); err != nil || uses != nil {
		t.Errorf("nil cache: uses=%v err=%v, want nil/nil", uses, err)
	}
	// An empty id must not trigger the sweep at all.
	c := depCache(twoGroupTenant(), src)
	if _, _, err := c.PolicyUses(context.Background(), DependencyScript, ""); err != nil {
		t.Fatal(err)
	}
	if src.listCalls != 0 {
		t.Errorf("listCalls = %d, want 0 — an empty id must not sweep", src.listCalls)
	}
}

func TestReportDependency_UnionsOverlappingScopesExactlyOnce(t *testing.T) {
	t.Parallel()
	// Two policies use script 500. Group One is {a,b}, Group Two is {b,c}, so the
	// combined audience is {a,b,c} = 3, not 2+2 = 4.
	src := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1)),
			"11": testPolicy(11, "Beta", withScripts(500), withGroupScope(2)),
		},
	}
	c := depCache(twoGroupTenant(), src)

	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1: %v", len(diags), diags)
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	if !strings.Contains(summary, "3 of 4 computers") {
		t.Errorf("summary must report the deduplicated union of 3, got %q", summary)
	}
	if !strings.Contains(summary, "2 policies") {
		t.Errorf("summary must name how many policies carry the change, got %q", summary)
	}
	for _, want := range []string{"Alpha", "Beta", "counted once"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q:\n%s", want, detail)
		}
	}
	if !strings.Contains(detail, "Searched 2 policies") {
		t.Errorf("detail must say what was searched:\n%s", detail)
	}
}

func TestReportDependency_DisabledPoliciesReportedSeparately(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Live", withScripts(500), withGroupScope(1)),
			"11": testPolicy(11, "Staged", disabled(), withScripts(500), withAllComputers()),
		},
	}
	c := depCache(twoGroupTenant(), src)
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	// The disabled policy scopes all computers; if it were counted the figure
	// would be 4 rather than the 2 the enabled policy reaches.
	if !strings.Contains(summary, "2 of 4 computers") {
		t.Errorf("a disabled policy must not contribute devices, got %q", summary)
	}
	if !strings.Contains(summary, "1 policy") {
		t.Errorf("summary should count only enabled policies, got %q", summary)
	}
	if !strings.Contains(detail, "Staged") || !strings.Contains(detail, "disabled") {
		t.Errorf("detail must still disclose the disabled policy:\n%s", detail)
	}
}

func TestReportDependency_OnlyDisabledPolicies(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Staged", disabled(), withScripts(500), withGroupScope(1)),
		},
	}
	c := depCache(twoGroupTenant(), src)
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	if !strings.Contains(diags[0].Summary(), "only by disabled policies") {
		t.Errorf("summary = %q", diags[0].Summary())
	}
	if !strings.Contains(diags[0].Detail(), "reaches no computers yet") {
		t.Errorf("detail = %q", diags[0].Detail())
	}
}

func TestReportDependency_OnlyDisabledPoliciesNeverReadsTheTenant(t *testing.T) {
	t.Parallel()
	// The disabled-only alert renders no figure — the headline names the situation and
	// the detail says it reaches nothing yet — so it must not depend on the group list
	// or the device totals. Proven with a group source that fails: if the alert still
	// reads correctly, nothing asked the tenant for anything.
	groups := &stubSource{err: errors.New("group list unavailable")}
	src := &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Staged", disabled(), withScripts(500), withGroupScope(1)),
		},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(groups, src), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	if !strings.Contains(diags[0].Summary(), "only by disabled policies") {
		t.Errorf("a failing group read must not degrade this alert: %q", diags[0].Summary())
	}
	if !strings.Contains(diags[0].Detail(), "Staged") {
		t.Errorf("detail must still name the disabled policy:\n%s", diags[0].Detail())
	}
	groups.mu.Lock()
	calls := groups.calls
	groups.mu.Unlock()
	if calls != 0 {
		t.Errorf("group list read %d times, want 0 — this path renders no figure to count", calls)
	}
}

func TestReportDependency_UnusedDependency(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{
		ids:      []string{"10"},
		policies: map[string]*proclassic.Policy{"10": testPolicy(10, "Other", withScripts(999))},
	}
	c := depCache(twoGroupTenant(), src)
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	// Worth reporting rather than staying silent: "nothing uses this" is the
	// reassurance an administrator editing a script wants.
	if !strings.Contains(diags[0].Summary(), "no policy uses this script") {
		t.Errorf("summary = %q", diags[0].Summary())
	}
}

func TestReportDependency_UnusedPackageNamesPatchManagement(t *testing.T) {
	t.Parallel()
	// A package has a second delivery channel the sweep does not read: a patch software
	// title assigns packages per software version and a patch policy carries its own
	// scope, so a package can reach devices with no ordinary policy referencing it.
	// "No policy uses this package" would then read as an all-clear it has not earned.
	newSrc := func() *stubPolicySource {
		return &stubPolicySource{
			ids:      []string{"10"},
			policies: map[string]*proclassic.Policy{"10": testPolicy(10, "Other", withScripts(999))},
		}
	}
	report := func(kind DependencyKind, id string) string {
		diags := ReportDependency(context.Background(), DependencyRequest{
			Cache: depCache(twoGroupTenant(), newSrc()), Path: path.Empty(), Kind: kind,
			ID: id, Name: "my thing", Action: ActionUpdate, Changed: true,
		})
		if len(diags) != 1 {
			t.Fatalf("%s: diags = %d, want 1", kind, len(diags))
		}
		return diags[0].Detail()
	}

	pkg := report(DependencyPackage, "80")
	if !strings.Contains(pkg, "Patch Management") {
		t.Errorf("the package denial must name the channel it did not search:\n%s", pkg)
	}
	if strings.Contains(pkg, "reaches no computers through a policy") {
		t.Errorf("a patch policy is a policy, so this claim is not available here:\n%s", pkg)
	}

	// The other five kinds have no second consumer, so their wording must be exactly
	// what it was — pinned byte-for-byte, since this is the sentence an administrator
	// acts on.
	want := "No policy in this tenant references my thing, so this change reaches no computers " +
		"through a policy.\n\nSearched 1 policy.\n\n" + dependencyNote
	for _, kind := range []DependencyKind{
		DependencyScript, DependencyPrinter, DependencyDockItem,
		DependencyDirectoryBinding, DependencyDiskEncryptionConfiguration,
	} {
		if got := report(kind, "500"); got != want {
			t.Errorf("%s wording changed:\n got %q\nwant %q", kind, got, want)
		}
	}
}

func TestReportDependency_UnscopedPoliciesKeepTheDenominator(t *testing.T) {
	t.Parallel()
	// Two enabled policies, neither with a scope. They reach nobody, but the figure
	// must still carry its denominator: the guide tells CI authors to anchor the device
	// count on the "of N" clause, and an alert reading "affects 0 computers" is the one
	// dependency headline that would drop it.
	src := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Unscoped one", withScripts(500)),
			"11": testPolicy(11, "Unscoped two", withScripts(500)),
		},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), src), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	if !strings.Contains(summary, "0 of 4 computers (0%)") {
		t.Errorf("an unscoped using policy must still report the estate it counts against: %q", summary)
	}
	// Nothing was counted, so there is no union for an overlap note to describe.
	if strings.Contains(detail, "counted once") {
		t.Errorf("claimed a union over an empty audience:\n%s", detail)
	}
}

func TestReportDependency_DisabledAsideIsCappedTighterThanTheList(t *testing.T) {
	t.Parallel()
	// The aside qualifies the figure rather than carrying it. Sharing the primary cap
	// let nine disabled policies print eight names, giving the subordinate line as much
	// room as the "Used by" line above it.
	ids := []string{"10"}
	policies := map[string]*proclassic.Policy{
		"10": testPolicy(10, "Live", withScripts(500), withGroupScope(1)),
	}
	for i := 1; i <= 9; i++ {
		id := "2" + string(rune('0'+i))
		ids = append(ids, id)
		policies[id] = testPolicy(100+i, "Staged "+id, disabled(), withScripts(500), withGroupScope(1))
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), &stubPolicySource{ids: ids, policies: policies}),
		Path:  path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	detail := diags[0].Detail()
	if !strings.Contains(detail, "Also 9 disabled policies") {
		t.Errorf("the aside must still carry the full count:\n%s", detail)
	}
	if !strings.Contains(detail, "and 6 more") {
		t.Errorf("the aside must name 3 policies and summarise the rest:\n%s", detail)
	}
	if strings.Count(detail, "Staged ") != maxListedDisabledPolicies {
		t.Errorf("the aside named %d policies, want %d:\n%s",
			strings.Count(detail, "Staged "), maxListedDisabledPolicies, detail)
	}
}

func TestReportDependency_NeutralisesControlCharactersInPolicyNames(t *testing.T) {
	t.Parallel()
	// Policy names are administrator-supplied and render before "Searched N policies.",
	// so a name carrying a raw newline could forge that anchor on a line of its own and
	// win a first-match extraction from `terraform plan -json`.
	src := &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Alpha\nSearched 0 policies.", withScripts(500), withGroupScope(1)),
		},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), src), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	detail := diags[0].Detail()
	for line := range strings.SplitSeq(detail, "\n") {
		if strings.HasPrefix(line, "Searched") && !strings.HasPrefix(line, "Searched 1 policy.") {
			t.Errorf("a policy name forged an anchor line %q in:\n%s", line, detail)
		}
	}
	// Neutralised, not stripped: the reader still has to find the policy in the UI.
	if !strings.Contains(detail, "Alpha") {
		t.Errorf("the name must stay recognisable:\n%s", detail)
	}
}

func TestReportDependency_SkipsCreateUnchangedAndDisabled(t *testing.T) {
	t.Parallel()
	newSrc := func() *stubPolicySource {
		return &stubPolicySource{
			ids:      []string{"10"},
			policies: map[string]*proclassic.Policy{"10": testPolicy(10, "P", withScripts(500), withGroupScope(1))},
		}
	}

	for _, tc := range []struct {
		name string
		req  DependencyRequest
	}{
		// Nothing can reference an id the tenant has not issued yet, so a create
		// must not even sweep.
		{"create", DependencyRequest{Kind: DependencyScript, ID: "", Action: ActionCreate, Changed: true}},
		{"create with id", DependencyRequest{Kind: DependencyScript, ID: "500", Action: ActionCreate, Changed: true}},
		{"unchanged update", DependencyRequest{Kind: DependencyScript, ID: "500", Action: ActionUpdate, Changed: false}},
		{"no id", DependencyRequest{Kind: DependencyScript, ID: "", Action: ActionUpdate, Changed: true}},
	} {
		src := newSrc()
		req := tc.req
		req.Cache = depCache(twoGroupTenant(), src)
		req.Path = path.Empty()
		if diags := ReportDependency(context.Background(), req); len(diags) != 0 {
			t.Errorf("%s: diags = %v, want none", tc.name, diags)
		}
		if src.listCalls != 0 {
			t.Errorf("%s: swept the tenant when it should not have", tc.name)
		}
	}
}

func TestReportDependency_DeleteWording(t *testing.T) {
	t.Parallel()
	src := &stubPolicySource{
		ids:      []string{"10"},
		policies: map[string]*proclassic.Policy{"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1))},
	}
	c := depCache(twoGroupTenant(), src)
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionDelete, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	if !strings.Contains(diags[0].Summary(), "removing this script affects") {
		t.Errorf("summary = %q", diags[0].Summary())
	}
	// The detail carries no delete-specific lead sentence — the headline states it —
	// so the body just has to name the affected policy.
	if !strings.Contains(diags[0].Detail(), "Used by 1 enabled policy: Alpha") {
		t.Errorf("detail = %q", diags[0].Detail())
	}
}

func TestReportDependency_SweepFailureNotifiesOnce(t *testing.T) {
	t.Parallel()
	c := depCache(twoGroupTenant(), &stubPolicySource{listErr: errors.New("tenant unreachable")})
	req := DependencyRequest{
		Cache: c, Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "s", Action: ActionUpdate, Changed: true,
	}
	first := ReportDependency(context.Background(), req)
	if len(first) != 1 || !strings.Contains(first[0].Summary(), "Impact alert unavailable") {
		t.Fatalf("first = %v, want one unavailable notice", first)
	}
	// Advisory, and once per plan: a second dependency resource stays quiet.
	if second := ReportDependency(context.Background(), req); len(second) != 0 {
		t.Errorf("second = %v, want none", second)
	}
}

func TestPolicyWireScope_ClassifiesEverySection(t *testing.T) {
	t.Parallel()

	t.Run("nil scope is empty", func(t *testing.T) {
		t.Parallel()
		got := policyWireScope(nil)
		if !got.Empty() || got.DeviceType != DeviceTypeComputer {
			t.Errorf("got %+v, want an empty computer scope", got)
		}
	})

	t.Run("targets counted, limitations narrow, user targets broaden", func(t *testing.T) {
		t.Parallel()
		s := &proclassic.PolicyScope{
			AllJssUsers: new(true),
			Computers: &proclassic.PolicyScopeComputers{
				Computer: &[]proclassic.PolicyScopeComputersComputerItem{{ID: new(7)}},
			},
			ComputerGroups: &proclassic.PolicyScopeComputerGroups{
				ComputerGroup: &[]proclassic.IDName{{ID: new(1)}},
			},
			Buildings: &proclassic.PolicyScopeBuildings{
				Building: &[]proclassic.IDName{{ID: new(3)}},
			},
			Departments: &proclassic.PolicyScopeDepartments{
				Department: &[]proclassic.IDName{{ID: new(4)}},
			},
			Limitations: &proclassic.PolicyScopeLimitations{
				NetworkSegments: &proclassic.PolicyScopeLimitationsNetworkSegments{
					NetworkSegment: &[]proclassic.IDName{{ID: new(9)}},
				},
			},
			Exclusions: &proclassic.PolicyScopeExclusions{
				ComputerGroups: &proclassic.PolicyScopeExclusionsComputerGroups{
					ComputerGroup: &[]proclassic.IDName{{ID: new(2)}},
				},
			},
		}
		got := policyWireScope(s)

		if len(got.DeviceIDs) != 1 || got.DeviceIDs[0] != "7" {
			t.Errorf("DeviceIDs = %v, want [7]", got.DeviceIDs)
		}
		if len(got.ProGroups) != 1 || got.ProGroups[0].ID != "1" ||
			got.ProGroups[0].DeviceType != DeviceTypeComputer {
			t.Errorf("ProGroups = %+v, want computer group 1", got.ProGroups)
		}
		if len(got.BuildingIDs) != 1 || len(got.DepartmentIDs) != 1 {
			t.Errorf("buildings/departments = %v/%v, want one each", got.BuildingIDs, got.DepartmentIDs)
		}
		if len(got.ExcludedProGroups) != 1 || got.ExcludedProGroups[0].ID != "2" {
			t.Errorf("ExcludedProGroups = %+v, want computer group 2", got.ExcludedProGroups)
		}

		var narrows, broadens int
		for _, u := range got.Unresolvable {
			switch u.Effect {
			case Narrows:
				narrows++
			case Broadens:
				broadens++
			}
		}
		// A network segment limitation narrows; all_jss_users broadens. Getting the
		// direction wrong would tell the reader the figure errs the wrong way.
		if narrows != 1 {
			t.Errorf("narrowing inputs = %d, want 1 (the network segment limitation)", narrows)
		}
		if broadens != 1 {
			t.Errorf("broadening inputs = %d, want 1 (all_jss_users)", broadens)
		}
	})
}

func TestCombineScopes_DropsPerPolicyExclusionsAsNarrowing(t *testing.T) {
	t.Parallel()
	// An exclusion belongs to the policy that declares it. A computer excluded
	// from policy A but targeted by policy B still receives the dependency, so
	// subtracting A's exclusion from the combined audience would undercount.
	a := Scope{
		DeviceType:        DeviceTypeComputer,
		ProGroups:         []ProGroupRef{{DeviceTypeComputer, "1"}},
		ExcludedDeviceIDs: []string{"x"},
	}
	b := Scope{
		DeviceType: DeviceTypeComputer,
		ProGroups:  []ProGroupRef{{DeviceTypeComputer, "2"}},
	}

	// One contributing policy keeps its exclusions, since they apply to the whole
	// (single-policy) audience.
	if only := combineScopes([]Scope{a}); len(only.ExcludedDeviceIDs) != 1 {
		t.Errorf("a single policy must keep its exclusions, got %+v", only.ExcludedDeviceIDs)
	}

	got := combineScopes([]Scope{a, b})
	if len(got.ExcludedDeviceIDs) != 0 {
		t.Errorf("ExcludedDeviceIDs = %v, want dropped once two policies combine", got.ExcludedDeviceIDs)
	}
	if len(got.ProGroups) != 2 {
		t.Errorf("ProGroups = %+v, want both groups unioned", got.ProGroups)
	}
	var found bool
	for _, u := range got.Unresolvable {
		if u.Effect == Narrows && strings.Contains(u.Reason, "per policy") {
			found = true
		}
	}
	if !found {
		t.Errorf("dropping exclusions must be disclosed as narrowing, got %+v", got.Unresolvable)
	}
}

func TestCombineScopes_KeepsExclusionsEveryContributorShares(t *testing.T) {
	t.Parallel()
	// A device excluded by every contributing policy receives the object from none of
	// them, so subtracting it removes something that was never in the union. Dropping
	// these too made the bound jump at the 1→2 boundary: two policies both targeting
	// every computer and both excluding one large kiosk group went from an exact
	// "400 of 1000" to "up to 1000 of 1000" purely because a second policy joined.
	kiosk := ProGroupRef{DeviceTypeComputer, "9"}
	both := []Scope{
		{DeviceType: DeviceTypeComputer, All: true, ExcludedProGroups: []ProGroupRef{kiosk}},
		{DeviceType: DeviceTypeComputer, All: true, ExcludedProGroups: []ProGroupRef{kiosk}},
	}
	got := combineScopes(both)
	if len(got.ExcludedProGroups) != 1 || got.ExcludedProGroups[0] != kiosk {
		t.Errorf("ExcludedProGroups = %+v, want the shared exclusion kept", got.ExcludedProGroups)
	}
	for _, u := range got.Unresolvable {
		if strings.Contains(u.Reason, "exclusions apply per policy") {
			t.Errorf("a fully shared exclusion must not degrade the bound: %+v", u)
		}
	}

	// Partly shared is not shared: the kiosk group is carried by both, the individually
	// excluded device by only one, so only the device becomes a caveat.
	mixed := []Scope{
		{DeviceType: DeviceTypeComputer, All: true,
			ExcludedProGroups: []ProGroupRef{kiosk}, ExcludedDeviceIDs: []string{"d1"}},
		{DeviceType: DeviceTypeComputer, All: true, ExcludedProGroups: []ProGroupRef{kiosk}},
	}
	got = combineScopes(mixed)
	if len(got.ExcludedProGroups) != 1 {
		t.Errorf("ExcludedProGroups = %+v, want the shared group still kept", got.ExcludedProGroups)
	}
	if len(got.ExcludedDeviceIDs) != 0 {
		t.Errorf("ExcludedDeviceIDs = %v, want dropped — only one policy excludes it", got.ExcludedDeviceIDs)
	}
	var dropped int
	for _, u := range got.Unresolvable {
		if strings.Contains(u.Reason, "exclusions apply per policy") {
			dropped = u.Values
		}
	}
	if dropped != 1 {
		t.Errorf("dropped exclusion count = %d, want 1 (the device only one policy excludes)", dropped)
	}

	// A group repeated inside one policy's own exclusions must not pass for two
	// policies agreeing.
	repeated := []Scope{
		{DeviceType: DeviceTypeComputer, All: true, ExcludedProGroups: []ProGroupRef{kiosk, kiosk}},
		{DeviceType: DeviceTypeComputer, All: true},
	}
	if got = combineScopes(repeated); len(got.ExcludedProGroups) != 0 {
		t.Errorf("ExcludedProGroups = %+v, want none — the second policy excludes nothing",
			got.ExcludedProGroups)
	}
}

func TestReportDependency_SharedExclusionKeepsTheFigureExact(t *testing.T) {
	t.Parallel()
	// End to end: two policies scope every computer and both exclude Group Two, which
	// holds two of the four managed computers. The audience is the other two, exactly —
	// no "up to", because nothing was left unsubtracted.
	src := &stubPolicySource{
		ids: []string{"10", "11"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Alpha", withScripts(500), withAllComputers(), withExcludedGroups(2)),
			"11": testPolicy(11, "Beta", withScripts(500), withAllComputers(), withExcludedGroups(2)),
		},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), src), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	summary, detail := diags[0].Summary(), diags[0].Detail()
	if !strings.Contains(summary, "2 of 4 computers") || strings.Contains(summary, "up to") {
		t.Errorf("a shared exclusion must keep the figure exact: %q", summary)
	}
	if !strings.Contains(detail, "Less 1 group excluded: Group Two") {
		t.Errorf("the subtraction must be shown, or a figure below the targets reads as a contradiction:\n%s", detail)
	}
	if strings.Contains(detail, "exclusions apply per policy") {
		t.Errorf("nothing was left unsubtracted, so there is no caveat to make:\n%s", detail)
	}
}

// TestDependencyAlert_IsMachineReadable pins the wording against the regexes
// documented in the CI section of docs/guides/impact-alerts.md.
//
// Alerts are consumed from `terraform plan -json` and matched by regex, so the
// phrasing is a contract, not prose. Compacting the detail is fine; moving a count
// out of its anchor phrase silently breaks every pipeline gating on it.
func TestDependencyAlert_IsMachineReadable(t *testing.T) {
	t.Parallel()

	// The shared figure pattern from the guide, which must match a dependency
	// headline exactly as it matches a policy or profile one.
	figure := regexp.MustCompile(`(scoped to|affects) (up to |at least |an estimated )?([0-9]+)( of ([0-9]+))?`)
	// Dependency-specific anchors.
	via := regexp.MustCompile(`via ([0-9]+) (policy|policies)$`)
	// Anchored to a line start, and that is the point rather than tidiness. Every
	// detail sentence below renders on its own line, while a policy name only ever
	// appears mid-line inside one of them — so a policy named "Searched 0 policies."
	// cannot impersonate the genuine sentence. Names come from the tenant, so anyone
	// with policy-write access chooses that text; an unanchored first-match read of
	// the detail is theirs to pre-empt.
	usedBy := regexp.MustCompile(`(?m)^Used by ([0-9]+) enabled (policy|policies):`)
	alsoDisabled := regexp.MustCompile(`(?m)^Also ([0-9]+) disabled (policy|policies), delivering nothing until enabled:`)
	searched := regexp.MustCompile(`(?m)^Searched ([0-9]+) (policy|policies)\.`)

	src := &stubPolicySource{
		ids: []string{"10", "11", "12"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1)),
			"11": testPolicy(11, "Beta", withScripts(500), withGroupScope(2)),
			"12": testPolicy(12, "Staged", disabled(), withScripts(500), withGroupScope(1)),
		},
	}

	for _, action := range []Action{ActionUpdate, ActionDelete} {
		c := depCache(twoGroupTenant(), src)
		diags := ReportDependency(context.Background(), DependencyRequest{
			Cache: c, Path: path.Empty(), Kind: DependencyScript,
			ID: "500", Name: "my script", Action: action, Changed: true,
		})
		if len(diags) != 1 {
			t.Fatalf("action %v: diags = %d, want 1", action, len(diags))
		}
		summary, detail := diags[0].Summary(), diags[0].Detail()

		m := figure.FindStringSubmatch(summary)
		if m == nil {
			t.Fatalf("action %v: summary does not match the documented figure regex: %q", action, summary)
		}
		// Group 3 is the count, group 5 the denominator: 3 of 4 computers.
		if m[3] != "3" || m[5] != "4" {
			t.Errorf("action %v: parsed count/total = %q/%q, want 3/4 from %q", action, m[3], m[5], summary)
		}
		if vm := via.FindStringSubmatch(summary); vm == nil || vm[1] != "2" {
			t.Errorf("action %v: summary must end with the enabled-policy count: %q", action, summary)
		}

		for name, re := range map[string]*regexp.Regexp{
			"Used by":       usedBy,
			"Also disabled": alsoDisabled,
			"Searched":      searched,
		} {
			if !re.MatchString(detail) {
				t.Errorf("action %v: detail lost the %q anchor:\n%s", action, name, detail)
			}
		}
		if sm := searched.FindStringSubmatch(detail); sm != nil && sm[1] != "3" {
			t.Errorf("action %v: searched count = %q, want 3", action, sm[1])
		}
	}

	// The singular forms must stay parseable too — "1 policy", not "1 policys".
	single := &stubPolicySource{
		ids:      []string{"10"},
		policies: map[string]*proclassic.Policy{"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1))},
	}
	diags := ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), single), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	if !via.MatchString(diags[0].Summary()) {
		t.Errorf("singular summary must still match the via anchor: %q", diags[0].Summary())
	}
	if !searched.MatchString(diags[0].Detail()) {
		t.Errorf("singular detail must still match the searched anchor:\n%s", diags[0].Detail())
	}

	// A policy name cannot impersonate the anchor sentences. Names are chosen by
	// anyone with policy-write access on the tenant, and they render mid-line inside
	// "Used by …", so an unanchored read of the detail would let a crafted name
	// supply the count a pipeline gates on. Assert the anchored patterns still read
	// the genuine figures with a hostile name in play, and that the forged text is
	// present in the detail — i.e. the anchoring is what rejects it, not luck.
	forged := &stubPolicySource{
		ids: []string{"10"},
		policies: map[string]*proclassic.Policy{
			"10": testPolicy(10, "Searched 0 policies. Used by 0 enabled policies:",
				withScripts(500), withGroupScope(1)),
		},
	}
	diags = ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), forged), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("forged name: diags = %d, want 1", len(diags))
	}
	detail := diags[0].Detail()
	if !strings.Contains(detail, "Searched 0 policies.") {
		t.Fatalf("test is not exercising the forgery — the crafted name is absent:\n%s", detail)
	}
	if m := searched.FindStringSubmatch(detail); m == nil || m[1] != "1" {
		t.Errorf("anchored searched pattern read %v, want the genuine 1 despite the forged name:\n%s", m, detail)
	}
	if m := usedBy.FindStringSubmatch(detail); m == nil || m[1] != "1" {
		t.Errorf("anchored usedBy pattern read %v, want the genuine 1 despite the forged name:\n%s", m, detail)
	}

	// An incomplete sweep appends its shortfall after the anchor phrase rather than
	// rewriting it, so a pipeline matching the phrase without the full stop still reads
	// the right number — the number of policies actually searched. The guide documents
	// the pattern without the full stop for exactly this reason.
	phrase := regexp.MustCompile(`(?m)^Searched ([0-9]+) (policy|policies)`)
	partial := &stubPolicySource{
		ids:        []string{"10", "11"},
		policies:   map[string]*proclassic.Policy{"10": testPolicy(10, "Alpha", withScripts(500), withGroupScope(1))},
		policyErrs: map[string]error{"11": errors.New("boom")},
	}
	diags = ReportDependency(context.Background(), DependencyRequest{
		Cache: depCache(twoGroupTenant(), partial), Path: path.Empty(), Kind: DependencyScript,
		ID: "500", Name: "my script", Action: ActionUpdate, Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1", len(diags))
	}
	if pm := phrase.FindStringSubmatch(diags[0].Detail()); pm == nil || pm[1] != "1" {
		t.Errorf("a partial sweep must keep the anchor phrase carrying the searched count:\n%s",
			diags[0].Detail())
	}
	// The unread policies make the figure a lower bound, so the headline takes the "or
	// more" shape — the guide's second documented figure pattern, which exists precisely
	// because the qualifier varies. The summary keeps its "affects <figure>, via N" shape
	// either way.
	orMore := regexp.MustCompile(`(scoped to|affects) ([0-9]+) or more( of ([0-9]+))?`)
	summary := diags[0].Summary()
	om := orMore.FindStringSubmatch(summary)
	if om == nil {
		t.Fatalf("a partial sweep's summary must match the documented or-more pattern: %q", summary)
	}
	if om[2] != "2" || om[4] != "4" {
		t.Errorf("parsed count/total = %q/%q, want 2/4 from %q", om[2], om[4], summary)
	}
	if vm := via.FindStringSubmatch(summary); vm == nil || vm[1] != "1" {
		t.Errorf("summary must still end with the enabled-policy count: %q", summary)
	}
}

func TestCombineScopes_AggregatesRepeatedCaveatsIntoOneLine(t *testing.T) {
	t.Parallel()
	// Live tenants hit this hard: a script used by 56 policies, most of them with
	// exclusions and some limited by a network segment, produced one identical
	// caveat line per policy — dozens of copies of the same sentence — which
	// buried the figure the alert exists to convey.
	scopes := make([]Scope, 0, 20)
	for i := range 20 {
		scopes = append(scopes, Scope{
			DeviceType: DeviceTypeComputer,
			ProGroups:  []ProGroupRef{{DeviceTypeComputer, "1"}},
			// A distinct device per policy, so every exclusion is one only that policy
			// carries and all 20 are genuinely undroppable — the case the caveat exists
			// for.
			ExcludedDeviceIDs: []string{fmt.Sprintf("x%d", i)},
			Unresolvable: []Unresolvable{{
				Path:   "scope.limitations.network_segments",
				Reason: ReasonNetworkSegment,
				Effect: Narrows,
				Values: 1,
			}},
		})
	}

	got := combineScopes(scopes)

	counts := map[string]int{}
	values := map[string]int{}
	for _, u := range got.Unresolvable {
		counts[u.Path]++
		values[u.Path] += u.Values
	}
	for path, n := range counts {
		if n != 1 {
			t.Errorf("%s appears %d times, want exactly 1 aggregated entry", path, n)
		}
	}
	// Aggregating must not lose the total: 20 policies each excluding one device.
	if values["policy exclusions"] != 20 {
		t.Errorf("policy exclusions total = %d, want 20", values["policy exclusions"])
	}
	if values["scope.limitations.network_segments"] != 20 {
		t.Errorf("network segment total = %d, want 20", values["scope.limitations.network_segments"])
	}

	// And the rendered caveats must contain each line once.
	res := Resolution{DeviceType: DeviceTypeComputer, Unresolvable: got.Unresolvable}
	rendered := strings.Join(caveats(res), "\n")
	if n := strings.Count(rendered, exclusionsAreNotCombinable); n != 1 {
		t.Errorf("exclusion caveat rendered %d times, want 1:\n%s", n, rendered)
	}
	if n := strings.Count(rendered, ReasonNetworkSegment); n != 1 {
		t.Errorf("network segment caveat rendered %d times, want 1:\n%s", n, rendered)
	}
}

func TestCombineScopes_DeduplicatesAndPropagatesAll(t *testing.T) {
	t.Parallel()
	a := Scope{DeviceType: DeviceTypeComputer,
		ProGroups: []ProGroupRef{{DeviceTypeComputer, "1"}}, DeviceIDs: []string{"d1"}}
	b := Scope{DeviceType: DeviceTypeComputer,
		ProGroups: []ProGroupRef{{DeviceTypeComputer, "1"}}, DeviceIDs: []string{"d1", "d2"}, All: true}

	got := combineScopes([]Scope{a, b})
	if len(got.ProGroups) != 1 {
		t.Errorf("ProGroups = %+v, want deduplicated to one", got.ProGroups)
	}
	if len(got.DeviceIDs) != 2 {
		t.Errorf("DeviceIDs = %v, want [d1 d2]", got.DeviceIDs)
	}
	if !got.All {
		t.Error("All must propagate: one policy scoping every computer makes the union tenant-wide")
	}
}
