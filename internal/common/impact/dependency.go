// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// A policy dependency is an object a policy uses to do its work rather than to
// choose its audience: a script, package, printer, Dock item, directory binding
// or disk encryption configuration. It has no scope of its own, so its blast
// radius is the combined audience of the policies referencing it — which neither
// the resource's diff nor Jamf Pro's save-time alert shows.
//
// That costs a whole-tenant policy read, and both cheaper routes are closed:
//
//   - No reverse lookup exists. Jamf Pro has a `data-dependency` endpoint for
//     extension attributes but no equivalent for these, so the only way to
//     answer "which policies use this script" is to read every policy.
//   - Subsets cannot trim the payload. `subset/PackageConfiguration` returns 200
//     with an empty body even when the policy has packages (as do `Maintenance`
//     and `FilesProcesses`, and inside multi-subset requests), so subsetting
//     would silently report that nothing uses a package. It would also save
//     only bandwidth — not the constraint — and measured 7% of wall clock.

// DependencyKind identifies a category of policy dependency. The values are the
// admin UI's own terms, and are used directly in diagnostics.
type DependencyKind string

const (
	// DependencyScript is a script a policy runs.
	DependencyScript DependencyKind = "script"
	// DependencyPackage is a package a policy installs or caches.
	DependencyPackage DependencyKind = "package"
	// DependencyPrinter is a printer a policy maps or removes.
	DependencyPrinter DependencyKind = "printer"
	// DependencyDockItem is a Dock item a policy adds or removes.
	DependencyDockItem DependencyKind = "dock item"
	// DependencyDirectoryBinding is a directory binding a policy applies.
	DependencyDirectoryBinding DependencyKind = "directory binding"
	// DependencyDiskEncryptionConfiguration is a disk encryption configuration a
	// policy applies or remediates against.
	DependencyDiskEncryptionConfiguration DependencyKind = "disk encryption configuration"
)

// dependencySweepConcurrency bounds the policy sweep's parallel reads.
//
// Five, on two agreeing grounds: Jamf's API scalability guidance asks for no more
// than five concurrent connections, and five is where measured throughput flattens
// (53 req/s at five workers, no faster at ten). A local worker bound is used rather
// than the provider's global request interval because the interval gates request
// starts, and so would throttle every resource in the plan to pace one sweep.
const dependencySweepConcurrency = 5

// PolicyUse is one policy that references a dependency, carrying the scope its
// audience is counted from.
type PolicyUse struct {
	// ID is the numeric Jamf Pro policy id.
	ID string
	// Name is the policy name, as shown in the admin UI.
	Name string
	// Enabled reports whether the policy is enabled. A disabled policy still
	// references the dependency but reaches nothing, so it is counted separately.
	Enabled bool
	// Scope is the policy's scope, reduced to the shape Resolve counts.
	Scope Scope
}

// dependencyKey identifies one dependency object.
type dependencyKey struct {
	kind DependencyKind
	id   string
}

// PolicySource supplies the policy sweep. Split from Source so the index can be
// tested without the group and inventory reads, and so a plan that touches no
// dependency resource never needs it.
type PolicySource interface {
	// PolicyIDs returns every policy id in the tenant.
	PolicyIDs(ctx context.Context) ([]string, error)
	// Policy returns one policy in full, including its dependency sections and
	// its scope.
	Policy(ctx context.Context, id string) (*proclassic.Policy, error)
}

// SweepStats reports how much of the tenant the policy sweep actually covered.
//
// The two counts are kept apart because a partial sweep must never be rendered as
// a complete one. "No policy uses this script — searched 295 policies" is a
// confident denial, and it is false if any of those 295 went unread: the answer the
// alert exists to give is exactly the one an unread policy can invalidate.
type SweepStats struct {
	// Searched is how many policies were read and folded into the index.
	Searched int
	// Unreadable is how many were listed but could not be read, so their references
	// are absent from the index and every negative answer is provisional.
	Unreadable int
}

// Listed is how many policies the tenant listed, read or not.
func (s SweepStats) Listed() int { return s.Searched + s.Unreadable }

// Complete reports whether every listed policy was read.
func (s SweepStats) Complete() bool { return s.Unreadable == 0 }

// dependencyIndex is the reverse map from dependency object to the policies
// using it, built once per Cache from a single whole-tenant sweep.
type dependencyIndex struct {
	uses map[dependencyKey][]PolicyUse
	// stats is what the sweep covered, so a diagnostic can say what was searched
	// rather than only what was found — and can hedge when it searched less than
	// the tenant holds.
	stats SweepStats
}

// PolicyUses returns the policies referencing one dependency object, and how much
// of the tenant was searched to find them.
//
// The sweep runs at most once per Cache — in practice once per plan — shared across
// every dependency resource in it, and is lazy: a plan changing none never sweeps.
// A nil Cache, or one without a policy source, reports nothing.
func (c *Cache) PolicyUses(ctx context.Context, kind DependencyKind, id string) (uses []PolicyUse, stats SweepStats, err error) {
	if c == nil || c.policySrc == nil || id == "" {
		return nil, SweepStats{}, nil
	}
	idx, err := c.policyIndex(ctx)
	if err != nil {
		return nil, SweepStats{}, err
	}
	return idx.uses[dependencyKey{kind, id}], idx.stats, nil
}

// policyIndex builds the reverse dependency index once, memoising result and
// failure alike. Failures are memoised for the same reason the group load does it:
// reporting is advisory, and retrying per resource would turn one transient
// failure into N slow resources and N duplicate notices.
func (c *Cache) policyIndex(ctx context.Context) (*dependencyIndex, error) {
	c.policyOnce.Do(func() {
		c.policies, c.policyErr = buildDependencyIndex(ctx, c.policySrc)
	})
	return c.policies, c.policyErr
}

// buildDependencyIndex sweeps every policy and inverts its dependency references
// into the index.
//
// An unreadable policy costs only its own contribution rather than the whole alert,
// but it is counted rather than quietly dropped: its references are missing from the
// index, so every "nothing uses this" the index supports afterwards is provisional
// and has to say so. Only the listing is fatal: without it the index would be
// silently empty, which reads as "nothing uses this".
func buildDependencyIndex(ctx context.Context, src PolicySource) (*dependencyIndex, error) {
	ids, err := src.PolicyIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing policies: %w", err)
	}

	var (
		mu    sync.Mutex
		index = &dependencyIndex{uses: make(map[dependencyKey][]PolicyUse)}
	)

	work := make(chan string)
	var wg sync.WaitGroup
	workers := min(dependencySweepConcurrency, len(ids))
	for range workers {
		wg.Go(func() {
			for id := range work {
				pol, err := src.Policy(ctx, id)
				if err != nil || pol == nil {
					mu.Lock()
					index.stats.Unreadable++
					mu.Unlock()
					continue
				}
				use, refs := policyDependencies(pol)
				// A policy referencing nothing still takes the lock: the searched
				// count is what the diagnostic reports, so it has to include the
				// policies that were read and found to use nothing.
				mu.Lock()
				index.stats.Searched++
				for _, key := range refs {
					index.uses[key] = append(index.uses[key], use)
				}
				mu.Unlock()
			}
		})
	}
	for _, id := range ids {
		select {
		case work <- id:
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(work)
	wg.Wait()

	// Sorted so a diagnostic listing policy names is stable across plans; the
	// sweep completes them in whatever order the workers finish.
	for key := range index.uses {
		sort.Slice(index.uses[key], func(i, j int) bool {
			return index.uses[key][i].Name < index.uses[key][j].Name
		})
	}
	return index, nil
}

// policyDependencies reduces one policy to its identity plus every dependency it
// references.
//
// Absent fields fail open in one direction throughout: a policy whose enabled flag
// cannot be read is treated as enabled, so it contributes devices and the figure
// errs high. Bucketing it as disabled instead would drop its audience from the
// figure entirely while still listing it, which is the one error mode this alert
// cannot afford.
func policyDependencies(p *proclassic.Policy) (PolicyUse, []dependencyKey) {
	use := PolicyUse{Scope: policyWireScope(p.Scope), Enabled: true}
	if p.General != nil {
		if p.General.ID != nil {
			use.ID = strconv.Itoa(*p.General.ID)
		}
		if p.General.Name != nil {
			use.Name = *p.General.Name
		}
		use.Enabled = p.General.Enabled == nil || *p.General.Enabled
	}
	switch {
	case use.Name != "":
	case use.ID != "":
		use.Name = "policy " + use.ID
	default:
		// Neither name nor id: "policy " alone would render as a dangling prefix in
		// the middle of a comma-separated list.
		use.Name = "an unidentified policy"
	}

	// De-duplicated per policy, because one policy counts once however many of its
	// fields name the same object. Disk encryption is the ordinary case: apply and
	// remediate routinely point at the same configuration, and a doubled PolicyUse
	// would read as "via 2 policies", list the name twice, and push combineScopes
	// off its exact single-policy path — turning an exact figure into an inflated
	// "up to".
	var refs []dependencyKey
	seen := make(map[dependencyKey]struct{})
	add := func(kind DependencyKind, id *int) {
		// A zero id is the Classic API's way of saying the field is unset, which the
		// disk-encryption fields emit rather than omitting them.
		if id == nil || *id <= 0 {
			return
		}
		key := dependencyKey{kind, strconv.Itoa(*id)}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		refs = append(refs, key)
	}

	if p.Scripts != nil && p.Scripts.Script != nil {
		for _, s := range *p.Scripts.Script {
			add(DependencyScript, s.ID)
		}
	}
	if p.PackageConfiguration != nil && p.PackageConfiguration.Packages != nil &&
		p.PackageConfiguration.Packages.Package != nil {
		for _, pkg := range *p.PackageConfiguration.Packages.Package {
			add(DependencyPackage, pkg.ID)
		}
	}
	if p.Printers != nil && p.Printers.Printer != nil {
		for _, pr := range *p.Printers.Printer {
			add(DependencyPrinter, pr.ID)
		}
	}
	if p.DockItems != nil && p.DockItems.DockItem != nil {
		for _, d := range *p.DockItems.DockItem {
			add(DependencyDockItem, d.ID)
		}
	}
	if p.AccountMaintenance != nil && p.AccountMaintenance.DirectoryBindings != nil &&
		p.AccountMaintenance.DirectoryBindings.Binding != nil {
		for _, b := range *p.AccountMaintenance.DirectoryBindings.Binding {
			add(DependencyDirectoryBinding, b.ID)
		}
	}
	// Apply and remediate name the configuration in different fields; either
	// makes the policy depend on it.
	if de := p.DiskEncryption; de != nil {
		add(DependencyDiskEncryptionConfiguration, de.DiskEncryptionConfigurationID)
		add(DependencyDiskEncryptionConfiguration, de.RemediateDiskEncryptionConfigurationID)
	}
	return use, refs
}

// policyWireScope converts a policy's scope as the API returns it into the shape
// Resolve counts.
//
// The wire-side counterpart to scope.BuildImpactScope, which converts the same
// scope from configuration. Both exist because the inputs differ — configuration
// carries types.Set values that may be unknown, the wire carries resolved ids that
// never pend — but each section's classification is kept identical, since a
// policy's scope means the same thing whichever direction it arrived from.
//
// Policies are computers-only, so every group reference is a computer group.
func policyWireScope(s *proclassic.PolicyScope) Scope {
	out := Scope{DeviceType: DeviceTypeComputer}
	if s == nil {
		return out
	}

	if s.AllComputers != nil && *s.AllComputers {
		out.All = true
	}

	// Targets that resolve to devices.
	if s.Computers != nil && s.Computers.Computer != nil {
		for _, c := range *s.Computers.Computer {
			out.DeviceIDs = appendID(out.DeviceIDs, c.ID)
		}
	}
	if s.ComputerGroups != nil && s.ComputerGroups.ComputerGroup != nil {
		for _, g := range *s.ComputerGroups.ComputerGroup {
			if g.ID != nil {
				out.ProGroups = append(out.ProGroups, ProGroupRef{
					DeviceType: DeviceTypeComputer, ID: strconv.Itoa(*g.ID),
				})
			}
		}
	}
	if s.Buildings != nil && s.Buildings.Building != nil {
		for _, b := range *s.Buildings.Building {
			out.BuildingIDs = appendID(out.BuildingIDs, b.ID)
		}
	}
	if s.Departments != nil && s.Departments.Department != nil {
		for _, d := range *s.Departments.Department {
			out.DepartmentIDs = appendID(out.DepartmentIDs, d.ID)
		}
	}

	// User targets reach devices through user assignment, so they broaden unquantifiably.
	if s.AllJssUsers != nil && *s.AllJssUsers {
		out.Unresolvable = append(out.Unresolvable, Unresolvable{
			Path: "scope.all_jss_users", Reason: ReasonUserTarget, Effect: Broadens, Values: 1,
		})
	}
	if s.JssUsers != nil && s.JssUsers.User != nil {
		out.Unresolvable = appendUnresolvable(out.Unresolvable,
			"scope.jss_users", ReasonUserTarget, Broadens, len(*s.JssUsers.User))
	}
	if s.JssUserGroups != nil && s.JssUserGroups.UserGroup != nil {
		out.Unresolvable = appendUnresolvable(out.Unresolvable,
			"scope.jss_user_groups", ReasonUserTarget, Broadens, len(*s.JssUserGroups.UserGroup))
	}

	// Limitations narrow, and none can be evaluated ahead of time.
	if l := s.Limitations; l != nil {
		if l.NetworkSegments != nil && l.NetworkSegments.NetworkSegment != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.limitations.network_segments", ReasonNetworkSegment, Narrows,
				len(*l.NetworkSegments.NetworkSegment))
		}
		if l.Ibeacons != nil && l.Ibeacons.Ibeacon != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.limitations.ibeacons", ReasonIbeacon, Narrows, len(*l.Ibeacons.Ibeacon))
		}
		if l.Users != nil && l.Users.User != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.limitations.users", ReasonUserName, Narrows, len(*l.Users.User))
		}
		if l.UserGroups != nil && l.UserGroups.UserGroup != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.limitations.user_groups", ReasonDirectoryServiceGroup, Narrows,
				len(*l.UserGroups.UserGroup))
		}
	}

	// Exclusions naming devices are carried as data so membership can be subtracted
	// exactly; the rest narrow by an unknown amount.
	if e := s.Exclusions; e != nil {
		if e.Computers != nil && e.Computers.Computer != nil {
			for _, c := range *e.Computers.Computer {
				out.ExcludedDeviceIDs = appendID(out.ExcludedDeviceIDs, c.ID)
			}
		}
		if e.ComputerGroups != nil && e.ComputerGroups.ComputerGroup != nil {
			for _, g := range *e.ComputerGroups.ComputerGroup {
				if g.ID != nil {
					out.ExcludedProGroups = append(out.ExcludedProGroups, ProGroupRef{
						DeviceType: DeviceTypeComputer, ID: strconv.Itoa(*g.ID),
					})
				}
			}
		}
		if e.Buildings != nil && e.Buildings.Building != nil {
			for _, b := range *e.Buildings.Building {
				out.ExcludedBuildingIDs = appendID(out.ExcludedBuildingIDs, b.ID)
			}
		}
		if e.Departments != nil && e.Departments.Department != nil {
			for _, d := range *e.Departments.Department {
				out.ExcludedDepartmentIDs = appendID(out.ExcludedDepartmentIDs, d.ID)
			}
		}
		if e.NetworkSegments != nil && e.NetworkSegments.NetworkSegment != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.exclusions.network_segments", ReasonNetworkSegment, Narrows,
				len(*e.NetworkSegments.NetworkSegment))
		}
		if e.Ibeacons != nil && e.Ibeacons.Ibeacon != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.exclusions.ibeacons", ReasonIbeacon, Narrows, len(*e.Ibeacons.Ibeacon))
		}
		if e.Users != nil && e.Users.User != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.exclusions.users", ReasonUserName, Narrows, len(*e.Users.User))
		}
		if e.UserGroups != nil && e.UserGroups.UserGroup != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.exclusions.user_groups", ReasonDirectoryServiceGroup, Narrows,
				len(*e.UserGroups.UserGroup))
		}
		if e.JssUsers != nil && e.JssUsers.User != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.exclusions.jss_users", ReasonUserTarget, Narrows, len(*e.JssUsers.User))
		}
		if e.JssUserGroups != nil && e.JssUserGroups.UserGroup != nil {
			out.Unresolvable = appendUnresolvable(out.Unresolvable,
				"scope.exclusions.jss_user_groups", ReasonUserTarget, Narrows,
				len(*e.JssUserGroups.UserGroup))
		}
	}

	return out
}

// appendID appends a numeric wire id as a string, skipping absent ones.
func appendID(out []string, id *int) []string {
	if id == nil {
		return out
	}
	return append(out, strconv.Itoa(*id))
}

// appendUnresolvable records a non-empty unresolvable input.
func appendUnresolvable(out []Unresolvable, path, reason string, e Effect, n int) []Unresolvable {
	if n == 0 {
		return out
	}
	return append(out, Unresolvable{Path: path, Reason: reason, Effect: e, Values: n})
}

// policyTenantSource reads policies from the live tenant.
type policyTenantSource struct {
	classic *proclassic.Client
}

func (s policyTenantSource) PolicyIDs(ctx context.Context) ([]string, error) {
	list, err := s.classic.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, nil
	}
	out := make([]string, 0, len(list.Policies))
	for _, p := range list.Policies {
		if p.ID != nil {
			out = append(out, strconv.Itoa(*p.ID))
		}
	}
	return out, nil
}

// Policy reads one policy in full — deliberately not GetPolicyByIDSubset, which
// silently omits PackageConfiguration. See the note at the top of this file.
func (s policyTenantSource) Policy(ctx context.Context, id string) (*proclassic.Policy, error) {
	return s.classic.GetPolicyByID(ctx, id)
}
