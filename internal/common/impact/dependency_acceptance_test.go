// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package impact

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/accrequire"
)

// The dependency index is the one part of impact reporting whose correctness
// depends on wire shapes the unit tests can only assume: which policy sections
// the API actually populates, and whether the ids in them line up with the ids
// the dependency resources carry in state. These tests run the production code
// path — NewTenantCache, the real sweep, the real reverse index — against a live
// tenant, and assert the invariants rather than any particular tenant's numbers,
// so they pass on any estate.

// liveCache builds a Cache against the tenant named by the environment.
//
// It builds its client directly rather than through testhelpers.NewAcceptanceClient
// because it needs WithMinRequestInterval(0) — the provider's own default, and
// the configuration the sweep's concurrency bound was chosen against — which the
// shared client does not carry. The unset-credential branch still routes through
// accrequire.SkipOrFailUnset, so bypassing the shared client does not also
// bypass the require gate: a bare t.Skip here would skip these three tests green
// in the pro lane while everything around them failed loudly.
// internal/conformance/acc_lanes_test.go allow-lists them by name for the direct
// construction, which records the exemption; this is what makes it safe.
//
// It reaches the gate through internal/testhelpers/accrequire rather than
// testhelpers, because testhelpers imports internal/provider, which reaches this
// package — importing it here would be an import cycle in the test binary. That
// is exactly why the gate is a leaf package.
func liveCache(t *testing.T) *Cache {
	t.Helper()
	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	clientID := os.Getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
	environmentID := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID")
	tenantID := os.Getenv("JAMFPLATFORM_TENANT_ID")
	if baseURL == "" || clientID == "" || clientSecret == "" || (environmentID == "" && tenantID == "") {
		accrequire.SkipOrFailUnset(t, "AccPreCheck", "JAMFPLATFORM_BASE_URL, _CLIENT_ID, _CLIENT_SECRET and one of _ENVIRONMENT_ID / _TENANT_ID are not all set")
	}
	scope := jamfplatform.WithEnvironmentID(environmentID)
	if environmentID == "" {
		scope = jamfplatform.WithTenantID(tenantID)
	}
	client := jamfplatform.NewClient(baseURL, clientID, clientSecret,
		scope,
		// Matches the provider's default, and is the configuration the sweep's
		// concurrency bound was chosen against.
		jamfplatform.WithMinRequestInterval(0),
	)
	if err := client.ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("credentials: %v", err)
	}
	return NewTenantCache(client)
}

// TestAcceptance_DependencyIndex_SweepsLiveTenant asserts the sweep completes,
// finds real references, and reuses its one result.
func TestAcceptance_DependencyIndex_SweepsLiveTenant(t *testing.T) {
	c := liveCache(t)
	ctx := context.Background()

	// Probe every kind. A tenant need not use all of them, but the sweep must
	// answer for each without error, and the whole thing must happen once.
	kinds := []DependencyKind{
		DependencyScript, DependencyPackage, DependencyPrinter,
		DependencyDockItem, DependencyDirectoryBinding,
		DependencyDiskEncryptionConfiguration,
	}

	start := time.Now()
	idx, err := c.policyIndex(ctx)
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	elapsed := time.Since(start)
	if idx.stats.Listed() == 0 {
		t.Skip("tenant has no policies; nothing to assert")
	}
	t.Logf("swept %d of %d policies in %s at %d-way concurrency (%.1f policies/sec)",
		idx.stats.Searched, idx.stats.Listed(), elapsed.Round(time.Millisecond), dependencySweepConcurrency,
		float64(idx.stats.Searched)/elapsed.Seconds())
	// An unreadable policy is survivable by design, but it means every "nothing uses
	// this" the index supports is provisional, so it is worth seeing in the log.
	if !idx.stats.Complete() {
		t.Logf("%d policies could not be read; the index is incomplete", idx.stats.Unreadable)
	}

	// Every reference the index holds must be well-formed: a real id, and a use
	// carrying the policy that declared it.
	byKind := map[DependencyKind]int{}
	for key, uses := range idx.uses {
		if key.id == "" {
			t.Errorf("index holds a %s reference with an empty id", key.kind)
		}
		byKind[key.kind] += len(uses)
		for _, u := range uses {
			if u.ID == "" {
				t.Errorf("%s %s: use with no policy id: %+v", key.kind, key.id, u)
			}
			if u.Name == "" {
				t.Errorf("%s %s: use with no policy name: %+v", key.kind, key.id, u)
			}
			if u.Scope.DeviceType != DeviceTypeComputer {
				t.Errorf("%s %s: policy %q scope device type = %q, want COMPUTER — policies are computers-only",
					key.kind, key.id, u.Name, u.Scope.DeviceType)
			}
		}
	}
	for _, k := range kinds {
		t.Logf("  %-32s %d reference(s) across the tenant", k, byKind[k])
	}

	// A second lookup, of any kind, must not re-sweep.
	for _, k := range kinds {
		if _, stats, err := c.PolicyUses(ctx, k, "1"); err != nil || stats != idx.stats {
			t.Errorf("%s: PolicyUses after sweep: stats=%+v err=%v, want %+v/nil", k, stats, err, idx.stats)
		}
	}
}

// TestAcceptance_DependencyIndex_PackagesAreFound reports what the tenant's own
// policies reference, and asserts the index agrees with the wire.
//
// It deliberately asserts nothing when the tenant installs no packages. The
// regression this once tried to catch — a sweep reading Classic subsets, which
// answer 200 with an empty PackageConfiguration — is indistinguishable from an
// estate whose policies simply install nothing, so inferring it from
// "scripts referenced, packages not" fails on any such tenant. That guard now
// lives where the distinction is observable, in
// TestDependencyIndex_ReadsWholePoliciesNotSubsets, which drives the real
// PolicySource against a server reproducing the empty-subset behaviour.
func TestAcceptance_DependencyIndex_PackagesAreFound(t *testing.T) {
	c := liveCache(t)
	ctx := context.Background()

	idx, err := c.policyIndex(ctx)
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	var packageRefs, scriptRefs int
	for key := range idx.uses {
		switch key.kind {
		case DependencyPackage:
			packageRefs++
		case DependencyScript:
			scriptRefs++
		}
	}
	t.Logf("distinct packages referenced: %d; distinct scripts referenced: %d", packageRefs, scriptRefs)

	// The wire, read independently of the index: how many packages the tenant's
	// policies actually install. Zero is a legitimate estate, and the only honest
	// response to it is to assert nothing.
	ids, err := c.policySrc.PolicyIDs(ctx)
	if err != nil {
		t.Fatalf("listing policies: %v", err)
	}
	wirePackages := map[string]struct{}{}
	for _, id := range ids {
		pol, err := c.policySrc.Policy(ctx, id)
		if err != nil || pol == nil {
			continue
		}
		if pol.PackageConfiguration == nil || pol.PackageConfiguration.Packages == nil ||
			pol.PackageConfiguration.Packages.Package == nil {
			continue
		}
		for _, pkg := range *pol.PackageConfiguration.Packages.Package {
			if pkg.ID != nil && *pkg.ID > 0 {
				wirePackages[strconv.Itoa(*pkg.ID)] = struct{}{}
			}
		}
	}
	if len(wirePackages) == 0 {
		t.Skip("no policy in this tenant installs a package; nothing to assert")
	}
	if packageRefs != len(wirePackages) {
		t.Errorf("index holds %d distinct package(s), the wire names %d — the index and the "+
			"tenant disagree about which packages policies install", packageRefs, len(wirePackages))
	}
	for id := range wirePackages {
		if len(idx.uses[dependencyKey{DependencyPackage, id}]) == 0 {
			t.Errorf("package %s is installed by a policy on the wire but absent from the index", id)
		}
	}
}

// TestAcceptance_DependencyReport_RendersForRealDependency renders the alert for
// whichever dependency the tenant uses most, which is the case most likely to
// expose bad arithmetic or unreadable wording.
func TestAcceptance_DependencyReport_RendersForRealDependency(t *testing.T) {
	c := liveCache(t)
	ctx := context.Background()

	idx, err := c.policyIndex(ctx)
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	var best dependencyKey
	var bestN int
	for key, uses := range idx.uses {
		if len(uses) > bestN {
			best, bestN = key, len(uses)
		}
	}
	if bestN == 0 {
		t.Skip("no policy in this tenant references any dependency")
	}
	t.Logf("most-used dependency: %s id %s, referenced by %d policies", best.kind, best.id, bestN)

	diags := ReportDependency(ctx, DependencyRequest{
		Cache:   c,
		Path:    path.Empty(),
		Kind:    best.kind,
		ID:      best.id,
		Name:    "the " + string(best.kind) + " under test",
		Action:  ActionUpdate,
		Changed: true,
	})
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want exactly 1: %v", len(diags), diags)
	}
	d := diags[0]
	if d.Severity().String() != "Warning" {
		t.Errorf("severity = %s, want Warning — impact alerts must never block a plan", d.Severity())
	}
	t.Logf("\n--- summary ---\n%s\n--- detail ---\n%s", d.Summary(), d.Detail())

	// The union must never exceed the managed estate: that would mean devices were
	// double-counted across overlapping policy scopes, which is the specific error
	// unioning member sets exists to avoid.
	//
	// Both invariants below run under BoundAtMost as well as BoundExact, because on a
	// real tenant BoundExact is the rare case: two contributing policies with any
	// exclusion between them make combineScopes emit a narrowing caveat — 111 of them
	// on the tenant this was developed against — and gating on BoundExact skipped the
	// assertions outright, exactly where the arithmetic is hardest. AtMost is safe for
	// both: it means something could only reduce the true figure, so no uncounted
	// devices were added, which is what each invariant relies on.
	uses, stats, err := c.PolicyUses(ctx, best.kind, best.id)
	if err != nil {
		t.Fatal(err)
	}
	res, err := resolveDependency(ctx, c, uses, stats)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	totals, err := c.DeviceTotals(ctx)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	// boundedAbove reports whether the figure can be trusted as a ceiling: nothing
	// went uncounted upwards, so the true audience is the figure or smaller.
	boundedAbove := res.Determinable && (res.Bound == BoundExact || res.Bound == BoundAtMost)
	t.Logf("invariants running under bound=%v (checked=%t)", res.Bound, boundedAbove)
	if boundedAbove && res.Count > totals.ManagedComputers {
		t.Errorf("counted %d computers but the tenant manages %d — overlapping policy scopes were double-counted",
			res.Count, totals.ManagedComputers)
	}
	t.Logf("union: %d computers of %d managed, across %d enabled and %d disabled policies (bound=%v)",
		res.Count, totals.ManagedComputers, len(res.Enabled), len(res.Disabled), res.Bound)

	// And the union must be at least as large as the largest single contributing
	// policy: a union that shrank below one of its inputs would mean exclusions
	// were subtracted from the wrong audience. Dropping a policy's exclusions can only
	// make the union larger than that policy's own exclusion-subtracted figure, so this
	// holds under AtMost too.
	for _, u := range res.Enabled {
		one, err := Resolve(ctx, c, u.Scope)
		if err != nil || !one.Determinable || one.Bound != BoundExact {
			continue
		}
		if boundedAbove && res.Count < one.Count {
			t.Errorf("union counted %d but policy %q alone reaches %d", res.Count, u.Name, one.Count)
		}
	}
}
