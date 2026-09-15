// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package impact computes plan-time scope impact for scopeable and deployable
// objects, mirroring Jamf Pro's impact alert notifications.
//
// Jamf Pro shows an impact alert on Save, summarising how many devices a change
// to a deployable object (policies, configuration profiles, apps) or a scopeable
// object (smart and static groups, classes) will affect. This package produces
// the equivalent signal during `terraform plan`, as advisory warning
// diagnostics. It never blocks a plan and never writes to the tenant.
//
// The counting inputs are group membership counts and the tenant device totals.
// Both are read once per provider instance and shared across every resource in
// the plan; see Cache.
package impact

import (
	"context"
	"fmt"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// DeviceType distinguishes the two membership namespaces Jamf Pro groups live
// in. Group rows carry it so a computer-scoped resource never resolves a mobile
// device group id, and so percentages are taken against the right denominator.
type DeviceType string

const (
	// DeviceTypeComputer is a computer group or computer-scoped resource.
	DeviceTypeComputer DeviceType = "COMPUTER"
	// DeviceTypeMobile is a mobile device group or mobile-device-scoped resource.
	DeviceTypeMobile DeviceType = "MOBILE"
	// DeviceTypeAny is a resource that can target either kind in one set —
	// blueprints and compliance benchmarks address device groups without
	// distinguishing computers from mobile devices.
	DeviceTypeAny DeviceType = ""
)

// Noun returns the user-facing plural for a device type, matching the Jamf Pro
// admin UI ("computers", "mobile devices", "devices").
func (d DeviceType) Noun() string {
	switch d {
	case DeviceTypeMobile:
		return "mobile devices"
	case DeviceTypeComputer:
		return "computers"
	default:
		return "devices"
	}
}

// accepts reports whether a group of type other belongs in a scope of this type.
func (d DeviceType) accepts(other DeviceType) bool {
	return d == DeviceTypeAny || d == other
}

// Group is one row of the tenant's group list: a single group carrying both of
// its identifiers plus its current membership count.
//
// Jamf Pro groups are addressed by two different identifiers depending on which
// surface refers to them. Jamf Pro resources (policies, profiles, restricted
// software) scope by the numeric Jamf Pro id; Jamf Platform resources
// (blueprints, compliance benchmarks) target the group's Platform UUID. Both
// identifiers arrive on the same row, so one read serves every caller.
type Group struct {
	PlatformID      string
	JamfProID       string
	Name            string
	DeviceType      DeviceType
	Smart           bool
	MembershipCount int64
}

// Totals is the tenant's device inventory summary, used as the denominator when
// expressing impact as a proportion.
type Totals struct {
	ManagedComputers     int64
	ManagedMobileDevices int64
}

// For returns the managed device total for a device type. DeviceTypeAny spans
// both, so it returns the whole managed estate.
func (t Totals) For(d DeviceType) int64 {
	switch d {
	case DeviceTypeMobile:
		return t.ManagedMobileDevices
	case DeviceTypeComputer:
		return t.ManagedComputers
	default:
		return t.ManagedComputers + t.ManagedMobileDevices
	}
}

// Source supplies the reads the impact calculation depends on. The production
// implementation is backed by the Jamf clients; tests substitute a stub so the
// resolution logic is exercised without HTTP.
type Source interface {
	// Groups returns every group in the tenant with its membership count and both
	// of its identifiers. One read serves the whole plan.
	Groups(ctx context.Context) ([]Group, error)
	// Totals returns the tenant's managed device counts.
	Totals(ctx context.Context) (Totals, error)
	// Members returns the device identifiers belonging to one group, addressed by
	// its Platform identifier. Read per group, only for groups a changing scope
	// actually names.
	Members(ctx context.Context, platformID string) ([]string, error)
	// ComputerManagementIDs returns the management identifiers of the computers
	// matching an inventory filter. Used to turn the Jamf Pro numeric identifiers a
	// scope block carries — individual computers, buildings, departments — into the
	// identifier space group membership uses.
	ComputerManagementIDs(ctx context.Context, filter string) ([]string, error)
	// MobileManagementIDs is the mobile device equivalent.
	MobileManagementIDs(ctx context.Context, filter string) ([]string, error)
	// PlaceNames maps building and department identifiers to their names. The mobile
	// inventory filters on the name where the computer inventory filters on the id,
	// so a scope block's ids have to be translated before they can be used.
	PlaceNames(ctx context.Context) (Places, error)
}

// Places maps building and department identifiers to names.
type Places struct {
	Buildings   map[string]string
	Departments map[string]string
}

// Cache holds the tenant group list and device totals for the lifetime of one
// provider instance — in practice, one terraform plan.
//
// Loading semantics:
//   - Both reads happen once, on the first resource that needs them. Terraform
//     evaluates resources concurrently, so concurrent callers block on the same
//     load rather than issuing duplicate reads.
//   - Failures are memoised. Impact reporting is advisory: a tenant that cannot
//     be read produces one "impact unavailable" notice and then stays quiet for
//     the rest of the plan. Retrying per resource would turn a transient blip
//     into N slow resources and N duplicate notices, and could make a plan that
//     currently issues no reads at all (a plan with refresh disabled) depend on
//     tenant availability.
//
// A nil *Cache is valid and reports nothing, which is how the disabled state
// (impact_alerts unset) is represented — callers need no flag check of their own.
type Cache struct {
	src Source

	once sync.Once
	err  error
	// byPro is keyed by device type as well as id, because Jamf Pro's numeric
	// group ids are only unique within an estate: id 1 is "All Managed Clients"
	// among computer groups and "All Managed iPads" among mobile device groups.
	// Keying on the id alone silently loses one of every colliding pair.
	byPro  map[proKey]Group
	byUUID map[string]Group
	totals Totals

	// memberMu guards the per-group membership map. Membership is read lazily,
	// one group at a time, because a plan only needs it for the groups a changing
	// scope names — not for every group in the tenant.
	memberMu sync.Mutex
	members  map[string]*memberSet

	// deviceMu guards the per-filter device lookups, cached by filter so two
	// resources naming the same building read it once.
	deviceMu sync.Mutex
	devices  map[string]*memberSet

	// placeOnce guards the one-time building and department name lookup, needed
	// only when a mobile-device scope names one of them.
	placeOnce sync.Once
	places    Places
	placeErr  error

	// noticeMu guards the one-shot latch for the "impact unavailable" notice, so
	// a tenant that cannot be read produces one notice for the plan rather than
	// one per scoped resource.
	noticeMu    sync.Mutex
	noticeFired bool

	// policySrc supplies the whole-tenant policy sweep behind dependency alerts.
	// Held separately from src because the sweep is far more expensive than the
	// group reads and is loaded only when a plan actually changes a policy
	// dependency; a nil policySrc reports no dependency impact at all.
	policySrc PolicySource
	// policyOnce guards the sweep, which happens at most once per Cache however
	// many dependency resources a plan changes. Its failure is memoised for the
	// same reason the group load's is.
	policyOnce sync.Once
	policies   *dependencyIndex
	policyErr  error
}

// memberSet is one group's membership, fetched at most once per Cache.
type memberSet struct {
	once sync.Once
	ids  []string
	err  error
}

// Members returns the device identifiers in a group, addressed by its Platform
// identifier. Results are cached for the lifetime of the Cache and fetched at
// most once per group even under concurrent callers.
//
// Membership is what makes exact arithmetic possible: two groups can share
// devices, so their counts cannot simply be added, and an exclusion cannot be
// subtracted from a sum that may already double-count. Device identifiers are
// Platform UUIDs and are therefore comparable across the computer and mobile
// device estates without translation.
func (c *Cache) Members(ctx context.Context, platformID string) ([]string, error) {
	if !c.Enabled() || platformID == "" {
		return nil, nil
	}
	c.memberMu.Lock()
	if c.members == nil {
		c.members = make(map[string]*memberSet)
	}
	entry, ok := c.members[platformID]
	if !ok {
		entry = &memberSet{}
		c.members[platformID] = entry
	}
	c.memberMu.Unlock()

	entry.once.Do(func() {
		entry.ids, entry.err = c.src.Members(ctx, platformID)
	})
	return entry.ids, entry.err
}

// noticeOnce reports whether the caller should emit the "impact unavailable"
// notice. It returns true exactly once per Cache.
func (c *Cache) noticeOnce() bool {
	if c == nil {
		return false
	}
	c.noticeMu.Lock()
	defer c.noticeMu.Unlock()
	if c.noticeFired {
		return false
	}
	c.noticeFired = true
	return true
}

// NewCache returns a Cache backed by src, without dependency reporting. Tests
// that exercise the dependency index use NewCacheWithPolicies.
func NewCache(src Source) *Cache {
	return &Cache{src: src}
}

// NewCacheWithPolicies returns a Cache that can also report policy dependency
// impact, backed by a policy sweep source.
func NewCacheWithPolicies(src Source, policies PolicySource) *Cache {
	return &Cache{src: src, policySrc: policies}
}

// NewTenantCache returns a Cache backed by a configured Jamf client.
//
// Two namespaces are involved and both are needed. Group counts and the pairing
// of Jamf Pro ids to Platform ids come from Jamf Pro; group membership comes
// from the Platform device groups service, which serves every kind of group —
// smart or static, computer or mobile device — through one call keyed by
// Platform identifier.
// Policy dependency alerts additionally need the Classic policy surface, since
// the reverse "which policies use this script" question has no endpoint of its
// own and can only be answered by sweeping policies.
func NewTenantCache(client *jamfplatform.Client) *Cache {
	return NewCacheWithPolicies(
		tenantSource{
			pro:    pro.New(client),
			groups: devicegroups.New(client),
		},
		policyTenantSource{classic: proclassic.New(client)},
	)
}

// Enabled reports whether this cache will report anything. A nil Cache is
// disabled.
func (c *Cache) Enabled() bool { return c != nil && c.src != nil }

// load performs the one-time read. The returned error is the memoised load
// failure; callers treat it as "impact unavailable", never as a plan error.
func (c *Cache) load(ctx context.Context) error {
	c.once.Do(func() {
		groups, err := c.src.Groups(ctx)
		if err != nil {
			c.err = err
			return
		}
		totals, err := c.src.Totals(ctx)
		if err != nil {
			c.err = err
			return
		}
		c.byPro = make(map[proKey]Group, len(groups))
		c.byUUID = make(map[string]Group, len(groups))
		for _, g := range groups {
			if g.JamfProID != "" {
				c.byPro[proKey{g.DeviceType, g.JamfProID}] = g
			}
			if g.PlatformID != "" {
				c.byUUID[g.PlatformID] = g
			}
		}
		c.totals = totals
	})
	return c.err
}

// proKey identifies a group by estate and numeric id, the only combination that
// is unique on the Jamf Pro side.
type proKey struct {
	deviceType DeviceType
	id         string
}

// GroupByJamfProID looks up a group by its numeric Jamf Pro identifier — the
// form Jamf Pro resources use in scope — within one estate.
//
// The device type is required rather than optional: numeric group ids repeat
// across the computer and mobile device estates, so an id on its own does not
// identify a group. Callers that genuinely do not know the estate should address
// groups by Platform identifier instead, which is unique tenant-wide.
func (c *Cache) GroupByJamfProID(ctx context.Context, dt DeviceType, id string) (Group, bool, error) {
	if err := c.load(ctx); err != nil {
		return Group{}, false, err
	}
	if dt == DeviceTypeAny {
		return Group{}, false, nil
	}
	g, ok := c.byPro[proKey{dt, id}]
	return g, ok, nil
}

// GroupByPlatformID looks up a group by its Platform UUID — the form blueprints
// and compliance benchmarks use to target device groups.
func (c *Cache) GroupByPlatformID(ctx context.Context, id string) (Group, bool, error) {
	if err := c.load(ctx); err != nil {
		return Group{}, false, err
	}
	g, ok := c.byUUID[id]
	return g, ok, nil
}

// DeviceTotals returns the tenant's managed device counts.
func (c *Cache) DeviceTotals(ctx context.Context) (Totals, error) {
	if err := c.load(ctx); err != nil {
		return Totals{}, err
	}
	return c.totals, nil
}

// tenantSource reads the live tenant.
type tenantSource struct {
	pro    *pro.Client
	groups *devicegroups.Client
}

// Members reads one group's membership. The endpoint is not paginated — its
// specification declares no page parameters, unlike its sibling group list — so
// a single call returns the complete set.
func (s tenantSource) Members(ctx context.Context, platformID string) ([]string, error) {
	ids, err := s.groups.ListDeviceGroupMembers(ctx, platformID)
	if err != nil {
		return nil, fmt.Errorf("reading membership of group %s: %w", platformID, err)
	}
	return ids, nil
}

// ComputerManagementIDs reads the management identifiers of the computers an
// inventory filter matches. Only the general section is requested, since the
// management identifier is the single field needed.
func (s tenantSource) ComputerManagementIDs(ctx context.Context, filter string) ([]string, error) {
	rows, err := s.pro.ListComputersInventoryV4(ctx, []string{pro.ComputerSectionV4General}, nil, filter)
	if err != nil {
		return nil, fmt.Errorf("reading computers matching the scope: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.General != nil && r.General.ManagementID != "" {
			out = append(out, r.General.ManagementID)
		}
	}
	return out, nil
}

// MobileManagementIDs reads the management identifiers of the mobile devices an
// inventory filter matches.
//
// The response is a union discriminated by operating system, and only one variant
// is populated per record — so the identifier has to be read from whichever it is.
func (s tenantSource) MobileManagementIDs(ctx context.Context, filter string) ([]string, error) {
	rows, err := s.pro.ListMobileDevicesDetailV2(ctx, []string{pro.MobileDeviceSectionGeneral}, nil, filter)
	if err != nil {
		return nil, fmt.Errorf("reading mobile devices matching the scope: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		switch {
		case r.IOS != nil && r.IOS.General != nil:
			out = appendIfSet(out, r.IOS.General.ManagementID)
		case r.TvOS != nil && r.TvOS.General != nil:
			out = appendIfSet(out, r.TvOS.General.ManagementID)
		case r.VisionOS != nil && r.VisionOS.General != nil:
			out = appendIfSet(out, r.VisionOS.General.ManagementID)
		case r.WatchOS != nil && r.WatchOS.General != nil:
			out = appendIfSet(out, r.WatchOS.General.ManagementID)
		}
	}
	return out, nil
}

// appendIfSet appends a non-empty identifier.
func appendIfSet(out []string, id string) []string {
	if id == "" {
		return out
	}
	return append(out, id)
}

// PlaceNames reads the building and department names, for the mobile inventory
// filters that match on name rather than id.
func (s tenantSource) PlaceNames(ctx context.Context) (Places, error) {
	buildings, err := s.pro.ListBuildingsV1(ctx, nil, "")
	if err != nil {
		return Places{}, fmt.Errorf("reading buildings: %w", err)
	}
	departments, err := s.pro.ListDepartmentsV1(ctx, nil, "")
	if err != nil {
		return Places{}, fmt.Errorf("reading departments: %w", err)
	}
	out := Places{
		Buildings:   make(map[string]string, len(buildings)),
		Departments: make(map[string]string, len(departments)),
	}
	for _, b := range buildings {
		if b.ID != nil {
			out.Buildings[*b.ID] = b.Name
		}
	}
	for _, d := range departments {
		if d.ID != nil {
			out.Departments[*d.ID] = d.Name
		}
	}
	return out, nil
}

func (s tenantSource) Groups(ctx context.Context) ([]Group, error) {
	rows, err := s.pro.ListGroupsV2(ctx, nil, "")
	if err != nil {
		return nil, fmt.Errorf("reading group membership counts: %w", err)
	}
	out := make([]Group, 0, len(rows))
	for _, r := range rows {
		dt := DeviceTypeComputer
		if r.GroupType == pro.GroupDtoV1GroupTypeMobile {
			dt = DeviceTypeMobile
		}
		out = append(out, Group{
			PlatformID:      r.GroupPlatformID,
			JamfProID:       r.GroupJamfProID,
			Name:            r.GroupName,
			DeviceType:      dt,
			Smart:           r.Smart,
			MembershipCount: int64(r.MembershipCount),
		})
	}
	return out, nil
}

func (s tenantSource) Totals(ctx context.Context) (Totals, error) {
	info, err := s.pro.GetInventoryInformationV1(ctx)
	if err != nil {
		return Totals{}, fmt.Errorf("reading device totals: %w", err)
	}
	if info == nil {
		return Totals{}, nil
	}
	return Totals{
		ManagedComputers:     int64(info.ManagedComputers),
		ManagedMobileDevices: int64(info.ManagedDevices),
	}, nil
}
