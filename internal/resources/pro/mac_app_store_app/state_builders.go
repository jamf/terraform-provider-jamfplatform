// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignMacAppResourceModel populates a resource model from the SDK
// MacApplication response. general is always refreshed (required block). The
// optional sections (scope / self_service / vpp) are only refreshed when the
// caller (plan or current state) already manages them: the classic server
// echoes every section on GET with default values, so populating an unmanaged
// section would violate the framework's "produced inconsistent result after
// apply" check (plan said null, we'd return a populated object). See
// feedback_server_derived_echo_attrs at block granularity.
//
// includeUnmanaged inverts those section gates for the list resource's
// config-generation path (terraform query -generate-config-out): there is no
// plan to stay consistent with, so every wire-present optional section is
// allocated and hydrated, yielding a complete exported config rather than a
// general-only one. CRUD callers pass false. The mac-app flatteners use the
// wire-authoritative reads (helpers.ReconcileOptionalStringPointer /
// helpers.BoolPointerValueOrNull), which adopt the wire value whatever state
// holds, so allocating an empty section is sufficient for it to fully hydrate.
func assignMacAppResourceModel(ctx context.Context, state *MacAppResourceModel, a *proclassic.MacApplication, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if a == nil {
		return diags
	}

	if id := extractMacAppID(a); id != "" {
		state.ID = types.StringValue(id)
	}

	if state.General == nil {
		state.General = &MacAppGeneralModel{}
	}
	flattenMacAppGeneral(a.General, state.General)

	if includeUnmanaged && state.Scope == nil && a.Scope != nil {
		state.Scope = &scope.ComputerScopeModelNoIbeacons{}
	}
	if state.Scope != nil && a.Scope != nil {
		flattenMacAppScope(ctx, a.Scope, state.Scope, includeUnmanaged)
	}
	if includeUnmanaged && state.SelfService == nil && a.SelfService != nil {
		state.SelfService = &MacAppSelfServiceModel{}
	}
	if state.SelfService != nil && a.SelfService != nil {
		flattenMacAppSelfService(a.SelfService, state.SelfService)
	}
	if includeUnmanaged && state.Vpp == nil && a.Vpp != nil {
		state.Vpp = &MacAppVppModel{}
	}
	if state.Vpp != nil && a.Vpp != nil {
		flattenMacAppVpp(a.Vpp, state.Vpp)
	}

	return diags
}

// flattenMacAppGeneral maps the wire <general> block onto the model. is_free
// keeps a sticky read: Jamf Pro resolves it from the App Store listing, so a
// PUT sending false, in isolation, left the GET reading true. Everything else
// is echoed faithfully and reads from the wire. Wire-probed against Jamf Pro
// 11.31.1 on 2026-09-06; see issue #387.
func flattenMacAppGeneral(g *proclassic.MacApplicationGeneral, state *MacAppGeneralModel) {
	if g == nil {
		return
	}
	state.ID = helpers.StringValueFromIntPtr(g.ID)
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.Version = helpers.StringPointerValueOrNull(g.Version)
	state.BundleID = helpers.StringPointerValueOrNull(g.BundleID)
	state.URL = helpers.StringPointerValueOrNull(g.URL)
	state.IsFree = helpers.StickyIgnoringDriftBool(g.IsFree, state.IsFree)
	state.DeploymentType = helpers.ReconcileOptionalStringPointer(g.DeploymentType, state.DeploymentType)

	if g.Category != nil {
		state.CategoryID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(g.Category.ID), state.CategoryID)
		state.CategoryName = helpers.DerivedRefName(g.Category.ID, g.Category.Name)
	} else {
		state.CategoryID = helpers.ReconcileOptionalStringPointer(nil, state.CategoryID)
		state.CategoryName = types.StringNull()
	}

	if g.Site != nil {
		state.SiteID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(g.Site.ID), state.SiteID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	} else {
		state.SiteID = helpers.ReconcileOptionalStringPointer(nil, state.SiteID)
		state.SiteName = types.StringNull()
	}
}

// flattenMacAppScope refreshes the scope sub-blocks the caller already manages.
// When includeUnmanaged is set (config generation) every wire-present sub-block
// is first allocated so the from-scratch read hydrates the full scope rather
// than leaving unmanaged targets/limitations/exclusions null.
func flattenMacAppScope(ctx context.Context, s *proclassic.MacApplicationScope, state *scope.ComputerScopeModelNoIbeacons, includeUnmanaged bool) {
	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &scope.ComputerScopeTargetsModel{}
		}
		if state.Limitations == nil && s.Limitations != nil {
			state.Limitations = &scope.ComputerScopeLimitationsModelNoIbeacons{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &scope.ComputerScopeExclusionsModelNoIbeacons{}
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

		t.ComputerIDs = scope.RefreshManagedSet(t.ComputerIDs, flattenComputerItemSet(ctx, s.Computers), includeUnmanaged)
		t.ComputerGroupIDs = scope.RefreshManagedSet(t.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, computerGroupSlice(s.ComputerGroups)), includeUnmanaged)
		t.BuildingIDs = scope.RefreshManagedSet(t.BuildingIDs, scope.FlattenIDNameSet(ctx, buildingSlice(s.Buildings)), includeUnmanaged)
		t.DepartmentIDs = scope.RefreshManagedSet(t.DepartmentIDs, scope.FlattenIDNameSet(ctx, departmentSlice(s.Departments)), includeUnmanaged)
		t.UserIDs = scope.RefreshManagedSet(t.UserIDs, scope.FlattenIDNameSet(ctx, jssUserSlice(s.JssUsers)), includeUnmanaged)
		t.UserGroupIDs = scope.RefreshManagedSet(t.UserGroupIDs, scope.FlattenIDNameSet(ctx, jssUserGroupSlice(s.JssUserGroups)), includeUnmanaged)
	}

	if state.Limitations != nil && s.Limitations != nil {
		l, sl := state.Limitations, s.Limitations
		l.NetworkSegmentIDs = scope.RefreshManagedSet(l.NetworkSegmentIDs, scope.FlattenIDNameSet(ctx, limitationsSegmentSlice(sl.NetworkSegments)), includeUnmanaged)
		l.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(l.DirectoryServiceOrLocalUserNames, scope.FlattenNameSet(ctx, limitationsUserSlice(sl.Users)), includeUnmanaged)
		l.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(l.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, limitationsUserGroupSlice(sl.UserGroups)), includeUnmanaged)
	}

	if state.Exclusions != nil && s.Exclusions != nil {
		x, se := state.Exclusions, s.Exclusions
		x.ComputerIDs = scope.RefreshManagedSet(x.ComputerIDs, flattenExclComputerItemSet(ctx, se.Computers), includeUnmanaged)
		x.ComputerGroupIDs = scope.RefreshManagedSet(x.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, exclComputerGroupSlice(se.ComputerGroups)), includeUnmanaged)
		x.BuildingIDs = scope.RefreshManagedSet(x.BuildingIDs, scope.FlattenIDNameSet(ctx, exclBuildingSlice(se.Buildings)), includeUnmanaged)
		x.DepartmentIDs = scope.RefreshManagedSet(x.DepartmentIDs, scope.FlattenIDNameSet(ctx, exclDepartmentSlice(se.Departments)), includeUnmanaged)
		x.UserIDs = scope.RefreshManagedSet(x.UserIDs, scope.FlattenIDNameSet(ctx, exclJssUserSlice(se.JssUsers)), includeUnmanaged)
		x.UserGroupIDs = scope.RefreshManagedSet(x.UserGroupIDs, scope.FlattenIDNameSet(ctx, exclJssUserGroupSlice(se.JssUserGroups)), includeUnmanaged)
		x.NetworkSegmentIDs = scope.RefreshManagedSet(x.NetworkSegmentIDs, flattenExclNetworkSegmentSet(ctx, se.NetworkSegments), includeUnmanaged)
		x.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(x.DirectoryServiceOrLocalUserNames, flattenExclUsersNameSet(ctx, se.Users), includeUnmanaged)
		x.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(x.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, exclUserGroupSlice(se.UserGroups)), includeUnmanaged)
	}
}

// flattenMacAppSelfService maps the wire <self_service> block onto the model.
//
// The Self Service notification fields are echoed only while the tenant-level
// Settings -> Self Service -> macOS -> "Enable Self Service notifications"
// toggle is on. With it off the write is accepted and the GET omits the whole
// <notification> family, so they read through helpers.WireWhenPresent*: the
// wire wins whenever it carries a value (drift reported), and state is kept
// when it does not (no "inconsistent result after apply" on a tenant that has
// notifications disabled). That gate is a tenant setting this resource does not
// own, which is why neither a plain wire read nor a sticky read is right.
//
// The rest of the block, the icon included, is echoed unconditionally and reads
// from the wire. Wire-probed against Jamf Pro 11.31.1 on 2026-09-06; see issue
// #387.
func flattenMacAppSelfService(ss *proclassic.MacApplicationSelfService, state *MacAppSelfServiceModel) {
	state.InstallButtonText = helpers.ReconcileOptionalStringPointer(ss.InstallButtonText, state.InstallButtonText)
	state.SelfServiceDescription = helpers.PreserveStringWhenWireEmpty(ss.SelfServiceDescription, state.SelfServiceDescription)
	state.ForceUsersToViewDescription = helpers.BoolPointerValueOrNull(ss.ForceUsersToViewDescription)
	state.FeatureOnMainPage = helpers.BoolPointerValueOrNull(ss.FeatureOnMainPage)

	var apiEnabled *bool
	var apiMethod *string
	if ss.Notification != nil {
		apiEnabled = ss.Notification.Enabled
		apiMethod = ss.Notification.Method
	}
	state.NotificationEnabled = helpers.WireWhenPresentBool(apiEnabled, state.NotificationEnabled)
	state.NotificationMethod = helpers.WireWhenPresentString(apiMethod, state.NotificationMethod)
	state.NotificationSubject = helpers.WireWhenPresentString(ss.NotificationSubject, state.NotificationSubject)
	state.NotificationMessage = helpers.WireWhenPresentString(ss.NotificationMessage, state.NotificationMessage)

	if state.SelfServiceIcon != nil && ss.SelfServiceIcon != nil {
		state.SelfServiceIcon.ID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(ss.SelfServiceIcon.ID), state.SelfServiceIcon.ID)
		state.SelfServiceIcon.URI = helpers.ReconcileOptionalStringPointer(ss.SelfServiceIcon.URI, state.SelfServiceIcon.URI)
	}

	if state.SelfServiceCategories != nil && ss.SelfServiceCategories != nil && ss.SelfServiceCategories.Category != nil {
		flattenMacAppSelfServiceCategories(*ss.SelfServiceCategories.Category, state)
	}
}

// flattenMacAppSelfServiceCategories refreshes the managed category set,
// matching server items to existing state items by ID so caller-authored
// values (display_in / feature_in) stick across refreshes. Server items not in
// state are appended; the set is keyed by category ID.
func flattenMacAppSelfServiceCategories(api []proclassic.MacApplicationSelfServiceSelfServiceCategoriesCategoryItem, state *MacAppSelfServiceModel) {
	byID := make(map[string]MacAppSelfServiceCategoryModel, len(state.SelfServiceCategories))
	for _, c := range state.SelfServiceCategories {
		byID[c.ID.ValueString()] = c
	}

	out := make([]MacAppSelfServiceCategoryModel, 0, len(api))
	for _, c := range api {
		idStr := ""
		if s := helpers.StringFromIntPtr(c.ID); s != nil {
			idStr = *s
		}
		current := byID[idStr]
		out = append(out, MacAppSelfServiceCategoryModel{
			ID:        types.StringValue(idStr),
			Name:      helpers.ReconcileOptionalStringPointer(c.Name, current.Name),
			DisplayIn: helpers.BoolPointerValueOrNull(c.DisplayIn),
			FeatureIn: helpers.BoolPointerValueOrNull(c.FeatureIn),
		})
	}
	state.SelfServiceCategories = out
}

func flattenMacAppVpp(v *proclassic.MacApplicationVpp, state *MacAppVppModel) {
	state.AssignVppDeviceBasedLicenses = helpers.BoolPointerValueOrNull(v.AssignVppDeviceBasedLicenses)
	state.VppAdminAccountID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(v.VppAdminAccountID), state.VppAdminAccountID)
	state.TotalVppLicenses = helpers.Int64FromIntPtr(v.TotalVppLicenses)
	state.RemainingVppLicenses = helpers.Int64FromIntPtr(v.RemainingVppLicenses)
	state.UsedVppLicenses = helpers.Int64FromIntPtr(v.UsedVppLicenses)
}

// ---- scope sub-slice accessors -------------------------------------------------

func computerGroupSlice(g *proclassic.MacApplicationScopeComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func buildingSlice(b *proclassic.MacApplicationScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func departmentSlice(d *proclassic.MacApplicationScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func jssUserSlice(u *proclassic.MacApplicationScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func jssUserGroupSlice(u *proclassic.MacApplicationScopeJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func limitationsSegmentSlice(s *proclassic.MacApplicationScopeLimitationsNetworkSegments) *[]proclassic.IDName {
	if s == nil {
		return nil
	}
	return s.NetworkSegment
}

func limitationsUserSlice(u *proclassic.MacApplicationScopeLimitationsUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func limitationsUserGroupSlice(u *proclassic.MacApplicationScopeLimitationsUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclComputerGroupSlice(g *proclassic.MacApplicationScopeExclusionsComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func exclBuildingSlice(b *proclassic.MacApplicationScopeExclusionsBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func exclDepartmentSlice(d *proclassic.MacApplicationScopeExclusionsDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func exclJssUserSlice(u *proclassic.MacApplicationScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func exclJssUserGroupSlice(u *proclassic.MacApplicationScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclUserGroupSlice(u *proclassic.MacApplicationScopeExclusionsUserGroups) *[]proclassic.IDName {
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

func flattenComputerItemSet(ctx context.Context, c *proclassic.MacApplicationScopeComputers) types.Set {
	if c == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, c.Computer, func(i proclassic.MacApplicationScopeComputersComputerItem) *int { return i.ID })
	return out
}

func flattenExclComputerItemSet(ctx context.Context, c *proclassic.MacApplicationScopeExclusionsComputers) types.Set {
	if c == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, c.Computer, func(i proclassic.MacApplicationScopeExclusionsComputersComputerItem) *int { return i.ID })
	return out
}

func flattenExclNetworkSegmentSet(ctx context.Context, n *proclassic.MacApplicationScopeExclusionsNetworkSegments) types.Set {
	if n == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, n.NetworkSegment, func(i proclassic.MacApplicationScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int { return i.ID })
	return out
}

func flattenExclUsersNameSet(ctx context.Context, u *proclassic.MacApplicationScopeExclusionsUsers) types.Set {
	if u == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenNameSlice(ctx, u.User, func(i proclassic.MacApplicationScopeExclusionsUsersUserItem) *string { return i.Name })
	return out
}
