// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignResourceModel populates a ResourceModel from the SDK response.
// Optional sub-blocks are only refreshed when the caller (plan or prior
// state) already manages them — otherwise a server-populated block would
// trip the framework's "produced inconsistent result after apply" check.
//
// includeUnmanaged inverts that gate for the list resource's
// config-generation path (terraform query -generate-config-out): there is no
// plan to stay consistent with, so every wire-present optional section is
// allocated and hydrated, yielding a complete exported config rather than a
// general-only one. CRUD callers pass false.
func assignResourceModel(ctx context.Context, state *ResourceModel, p *proclassic.OsXConfigurationProfile, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if p == nil {
		return diags
	}

	if p.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(p.ID)
	} else if p.General != nil && p.General.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(p.General.ID)
	}

	if state.General == nil {
		state.General = &GeneralModel{}
	}
	flattenGeneral(p.General, state.General)

	if includeUnmanaged && state.Scope == nil && p.Scope != nil {
		state.Scope = &scope.ComputerScopeModel{}
	}
	if state.Scope != nil && p.Scope != nil {
		diags.Append(flattenScope(ctx, p.Scope, state.Scope, includeUnmanaged)...)
	}
	if includeUnmanaged && state.SelfService == nil && p.SelfService != nil {
		state.SelfService = &SelfServiceModel{}
	}
	if state.SelfService != nil && p.SelfService != nil {
		flattenSelfService(p.SelfService, state.SelfService, includeUnmanaged)
	}
	return diags
}

func flattenGeneral(g *proclassic.OsXConfigurationProfileGeneral, state *GeneralModel) {
	if g == nil {
		return
	}
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.ReconcileOptionalStringPointer(g.Name, state.Name)
	state.Description = helpers.ReconcileOptionalStringPointer(g.Description, state.Description)
	state.UserRemovable = helpers.ReconcileOptionalBoolPointer(g.UserRemovable, state.UserRemovable)
	// RedeployOnUpdate: the Classic API always returns "Newly Assigned" on
	// read regardless of what was sent on Update — the field is effectively
	// write-only on the wire (the API accepts "All" on PUT but never echoes
	// it back on GET). Same wire bug deploymenttheory PR #888 addresses.
	//
	// Adapted for Optional+Computed: write from wire only when state is
	// null/unknown (Create with no user input, or import). Otherwise keep
	// the user-authored value so a subsequent plan after
	// `redeploy_on_update = "All"` does not snap back to "Newly Assigned"
	// on every refresh. The Optional+Computed shape requires a known value
	// after apply, hence the wire fallback when the user omitted the
	// attribute.
	if state.RedeployOnUpdate.IsNull() || state.RedeployOnUpdate.IsUnknown() {
		state.RedeployOnUpdate = helpers.StringPointerValueOrNull(g.RedeployOnUpdate)
	}
	state.UUID = helpers.StringPointerValueOrNull(g.UUID)
	// Self-healing payload assignment. The framework requires
	// plan.Payloads == final state.Payloads for the Required `payloads`
	// attribute, but the server re-serialises the plist and re-assigns
	// top-level UUIDs — so the byte-form changes on every write. Keep
	// the user-authored bytes when the server's canonical form is
	// semantically equivalent; only overwrite with the server form when
	// genuine out-of-band drift is detected.
	if g.Payloads != nil {
		server := string(*g.Payloads)
		keep := false
		if !state.Payloads.IsNull() && !state.Payloads.IsUnknown() {
			if eq, err := payloadhelpers.PayloadsSemanticallyEqual([]byte(state.Payloads.ValueString()), []byte(server)); err == nil && eq {
				keep = true
			}
		}
		if !keep {
			state.Payloads = types.StringValue(string(plisthelpers.CanonicalisePlistXML([]byte(server))))
		}
	}
	state.DistributionMethod = helpers.ReconcileOptionalStringPointer(g.DistributionMethod, state.DistributionMethod)

	// Level wire-read translation. The wire returns System for Computer Level
	// and User for User Level — translate back to the UI-canonical strings.
	if g.Level != nil {
		state.Level = types.StringValue(levelFromWireRead(*g.Level))
	} else if !helpers.IsConfiguredValue(state.Level) {
		state.Level = types.StringNull()
	}

	// Category — emit the assigned ID; "no category" sentinel "-1" stays
	// visible so users see Jamf's default rather than null.
	if g.Category != nil {
		state.CategoryID = helpers.StringValueFromIntPtr(g.Category.ID)
		state.CategoryName = helpers.DerivedRefName(g.Category.ID, g.Category.Name)
	}
	if g.Site != nil {
		state.SiteID = helpers.StringValueFromIntPtr(g.Site.ID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	}
}

// flattenScope refreshes the scope sub-blocks the caller already manages,
// gating each category independently (see the in-body comment). When
// includeUnmanaged is set (import / config generation / the Update merge base)
// every wire-present sub-block is first allocated so the from-scratch read
// hydrates the full scope rather than leaving unmanaged
// targets/limitations/exclusions null.
func flattenScope(ctx context.Context, s *proclassic.OsXConfigurationProfileScope, state *scope.ComputerScopeModel, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &scope.ComputerScopeTargetsModel{}
		}
		if state.Limitations == nil && s.Limitations != nil {
			state.Limitations = &scope.ComputerScopeLimitationsModel{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &scope.ComputerScopeExclusionsModel{}
		}
	}

	// Sub-blocks are gated on caller management (typed-pointer models cannot
	// hold categories without the block struct); within a managed sub-block
	// each category refreshes independently via RefreshManagedSet — a category
	// the caller did not declare (null) stays null, so members maintained in
	// the admin UI never enter state. includeUnmanaged bypasses both gates for
	// import / config-generation hydration and for building the server-side
	// merge base in Update.
	if state.Targets != nil {
		t := state.Targets
		t.AllComputers = scope.RefreshManagedBool(t.AllComputers, s.AllComputers, includeUnmanaged)
		t.AllJssUsers = scope.RefreshManagedBool(t.AllJssUsers, s.AllJssUsers, includeUnmanaged)

		t.ComputerIDs = scope.RefreshManagedSet(t.ComputerIDs, flattenScopeComputerItemSet(ctx, s.Computers), includeUnmanaged)
		t.ComputerGroupIDs = scope.RefreshManagedSet(t.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromComputerGroups(s.ComputerGroups)), includeUnmanaged)
		t.BuildingIDs = scope.RefreshManagedSet(t.BuildingIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromBuildings(s.Buildings)), includeUnmanaged)
		t.DepartmentIDs = scope.RefreshManagedSet(t.DepartmentIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromDepartments(s.Departments)), includeUnmanaged)
		t.UserIDs = scope.RefreshManagedSet(t.UserIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromJssUsers(s.JssUsers)), includeUnmanaged)
		t.UserGroupIDs = scope.RefreshManagedSet(t.UserGroupIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromJssUserGroups(s.JssUserGroups)), includeUnmanaged)
	}

	if state.Limitations != nil && s.Limitations != nil {
		flattenScopeLimitations(ctx, s.Limitations, state.Limitations, includeUnmanaged)
	}
	if state.Exclusions != nil && s.Exclusions != nil {
		flattenScopeExclusions(ctx, s.Exclusions, state.Exclusions, includeUnmanaged)
	}
	return diags
}

func flattenScopeLimitations(ctx context.Context, l *proclassic.OsXConfigurationProfileScopeLimitations, state *scope.ComputerScopeLimitationsModel, includeUnmanaged bool) {
	if l == nil {
		return
	}
	state.NetworkSegmentIDs = scope.RefreshManagedSet(state.NetworkSegmentIDs, flattenLimNetworkSegmentSet(ctx, l.NetworkSegments), includeUnmanaged)
	state.IbeaconIDs = scope.RefreshManagedSet(state.IbeaconIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromLimIbeacons(l.Ibeacons)), includeUnmanaged)
	state.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(state.DirectoryServiceOrLocalUserNames, scope.FlattenNameSet(ctx, idNameSliceFromLimUsers(l.Users)), includeUnmanaged)
	state.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(state.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, idNameSliceFromLimUserGroups(l.UserGroups)), includeUnmanaged)
}

func flattenScopeExclusions(ctx context.Context, e *proclassic.OsXConfigurationProfileScopeExclusions, state *scope.ComputerScopeExclusionsModel, includeUnmanaged bool) {
	if e == nil {
		return
	}
	state.ComputerIDs = scope.RefreshManagedSet(state.ComputerIDs, flattenExclComputerItemSet(ctx, e.Computers), includeUnmanaged)
	state.ComputerGroupIDs = scope.RefreshManagedSet(state.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromExclComputerGroups(e.ComputerGroups)), includeUnmanaged)
	state.BuildingIDs = scope.RefreshManagedSet(state.BuildingIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromExclBuildings(e.Buildings)), includeUnmanaged)
	state.DepartmentIDs = scope.RefreshManagedSet(state.DepartmentIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromExclDepartments(e.Departments)), includeUnmanaged)
	state.UserIDs = scope.RefreshManagedSet(state.UserIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromExclJssUsers(e.JssUsers)), includeUnmanaged)
	state.UserGroupIDs = scope.RefreshManagedSet(state.UserGroupIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromExclJssUserGroups(e.JssUserGroups)), includeUnmanaged)
	state.NetworkSegmentIDs = scope.RefreshManagedSet(state.NetworkSegmentIDs, flattenExclNetworkSegmentSet(ctx, e.NetworkSegments), includeUnmanaged)
	state.IbeaconIDs = scope.RefreshManagedSet(state.IbeaconIDs, scope.FlattenIDNameSet(ctx, idNameSliceFromExclIbeacons(e.Ibeacons)), includeUnmanaged)
	state.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(state.DirectoryServiceOrLocalUserNames, flattenExclUsersNameSet(ctx, e.Users), includeUnmanaged)
	state.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(state.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, idNameSliceFromExclUserGroups(e.UserGroups)), includeUnmanaged)
}

// ---- scope sub-slice accessors -------------------------------------------------

func idNameSliceFromComputerGroups(g *proclassic.OsXConfigurationProfileScopeComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func idNameSliceFromBuildings(b *proclassic.OsXConfigurationProfileScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func idNameSliceFromDepartments(d *proclassic.OsXConfigurationProfileScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func idNameSliceFromJssUsers(u *proclassic.OsXConfigurationProfileScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func idNameSliceFromJssUserGroups(u *proclassic.OsXConfigurationProfileScopeJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func idNameSliceFromLimIbeacons(i *proclassic.OsXConfigurationProfileScopeLimitationsIbeacons) *[]proclassic.IDName {
	if i == nil {
		return nil
	}
	return i.Ibeacon
}

func idNameSliceFromLimUsers(u *proclassic.OsXConfigurationProfileScopeLimitationsUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func idNameSliceFromLimUserGroups(u *proclassic.OsXConfigurationProfileScopeLimitationsUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func idNameSliceFromExclComputerGroups(g *proclassic.OsXConfigurationProfileScopeExclusionsComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func idNameSliceFromExclBuildings(b *proclassic.OsXConfigurationProfileScopeExclusionsBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func idNameSliceFromExclDepartments(d *proclassic.OsXConfigurationProfileScopeExclusionsDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func idNameSliceFromExclJssUsers(u *proclassic.OsXConfigurationProfileScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func idNameSliceFromExclJssUserGroups(u *proclassic.OsXConfigurationProfileScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func idNameSliceFromExclIbeacons(i *proclassic.OsXConfigurationProfileScopeExclusionsIbeacons) *[]proclassic.IDName {
	if i == nil {
		return nil
	}
	return i.Ibeacon
}

func idNameSliceFromExclUserGroups(u *proclassic.OsXConfigurationProfileScopeExclusionsUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

// ---- set flatteners ------------------------------------------------------------

// The wire flatteners below return EmptyStringSet (never null) for an absent
// element: a null return would flow through RefreshManagedSet and null out a
// managed category, tripping the post-apply consistency check. Empty is the
// canonical "no members" value for a managed category; unmanaged categories
// are kept null by the RefreshManagedSet gate itself.

func flattenScopeComputerItemSet(ctx context.Context, c *proclassic.OsXConfigurationProfileScopeComputers) types.Set {
	if c == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, c.Computer, func(i proclassic.OsXConfigurationProfileScopeComputersComputerItem) *int { return i.ID })
	return out
}

func flattenLimNetworkSegmentSet(ctx context.Context, n *proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegments) types.Set {
	if n == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, n.NetworkSegment, func(i proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegmentsNetworkSegmentItem) *int {
		return i.ID
	})
	return out
}

func flattenExclComputerItemSet(ctx context.Context, c *proclassic.OsXConfigurationProfileScopeExclusionsComputers) types.Set {
	if c == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, c.Computer, func(i proclassic.OsXConfigurationProfileScopeExclusionsComputersComputerItem) *int { return i.ID })
	return out
}

func flattenExclNetworkSegmentSet(ctx context.Context, n *proclassic.OsXConfigurationProfileScopeExclusionsNetworkSegments) types.Set {
	if n == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, n.NetworkSegment, func(i proclassic.OsXConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int {
		return i.ID
	})
	return out
}

func flattenExclUsersNameSet(ctx context.Context, u *proclassic.OsXConfigurationProfileScopeExclusionsUsers) types.Set {
	if u == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenNameSlice(ctx, u.User, func(i proclassic.OsXConfigurationProfileScopeExclusionsUsersUserItem) *string { return i.Name })
	return out
}

func flattenSelfService(ss *proclassic.OsXConfigurationProfileSelfService, state *SelfServiceModel, includeUnmanaged bool) {
	if ss == nil {
		return
	}
	state.SelfServiceDisplayName = helpers.ReconcileOptionalStringPointer(ss.SelfServiceDisplayName, state.SelfServiceDisplayName)
	state.InstallButtonText = helpers.ReconcileOptionalStringPointer(ss.InstallButtonText, state.InstallButtonText)
	state.SelfServiceDescription = helpers.ReconcileOptionalStringPointer(ss.SelfServiceDescription, state.SelfServiceDescription)
	state.EnsureUsersViewDescription = helpers.ReconcileOptionalBoolPointer(ss.ForceUsersToViewDescription, state.EnsureUsersViewDescription)
	state.FeatureOnMainPage = helpers.ReconcileOptionalBoolPointer(ss.FeatureOnMainPage, state.FeatureOnMainPage)
	state.NotificationSubject = helpers.PreserveStringWhenWireEmpty(ss.NotificationSubject, state.NotificationSubject)
	state.NotificationMessage = helpers.PreserveStringWhenWireEmpty(ss.NotificationMessage, state.NotificationMessage)

	if ss.Notification != nil {
		state.DisplayNotifications = helpers.ReconcileOptionalBoolPointer(ss.Notification.Enabled, state.DisplayNotifications)
		state.NotificationLocation = helpers.ReconcileOptionalStringPointer(ss.Notification.Method, state.NotificationLocation)
	}

	if ss.Security != nil {
		state.RemovalDisallowed = helpers.ReconcileOptionalStringPointer(ss.Security.RemovalDisallowed, state.RemovalDisallowed)
	}

	// categories is ownership-gated like every sibling above it. It used not to
	// be: the hydration tested only what the wire carried, so dropping
	// categories from a self_service block you otherwise keep failed the apply
	// with "Provider produced inconsistent result after apply" — the plan said
	// null and Read handed back a populated list. The server retains them
	// correctly; this was ours (issue #392).
	//
	// Optional-only, so a nil slice means the configuration does not manage the
	// attribute and an empty non-nil one means it declares `[]`. Managed, the
	// wire is authoritative including when it is empty, which is what lets a
	// declared `[]` converge. Unmanaged, only the first hydration populates it
	// — import, config generation and the Update merge base — and even then
	// only if the server has any, so an ordinary refresh never fabricates a
	// list the practitioner never wrote.
	switch {
	case state.Categories != nil:
		state.Categories = flattenSelfServiceCategories(ss.SelfServiceCategories)
	case includeUnmanaged:
		if cats := flattenSelfServiceCategories(ss.SelfServiceCategories); len(cats) > 0 {
			state.Categories = cats
		}
	}
}

// flattenSelfServiceCategories maps the wire category list onto the model,
// returning an empty (non-nil) slice when the server carries none so a managed
// attribute can converge on a declared `[]`.
func flattenSelfServiceCategories(c *proclassic.OsXConfigurationProfileSelfServiceSelfServiceCategories) []SelfServiceCategoryItem {
	if c == nil || c.Category == nil {
		return []SelfServiceCategoryItem{}
	}
	cats := *c.Category
	items := make([]SelfServiceCategoryItem, 0, len(cats))
	for _, cat := range cats {
		items = append(items, SelfServiceCategoryItem{
			ID:        idPointerToString(cat.ID),
			Name:      helpers.StringPointerValueOrNull(cat.Name),
			DisplayIn: helpers.BoolPointerValueOrNull(cat.DisplayIn),
			FeatureIn: helpers.BoolPointerValueOrNull(cat.FeatureIn),
		})
	}
	return items
}

func idPointerToString(id *int) types.String {
	if id == nil {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(*id))
}
