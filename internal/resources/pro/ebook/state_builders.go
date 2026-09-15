// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignEbookResourceModel populates a resource model from the SDK Ebook
// response. general is always refreshed (required block). The optional sections
// (scope / self_service) are only refreshed when the caller (plan or current
// state) already manages them: the classic server echoes every section on GET
// with default values, so populating an unmanaged section would violate the
// framework's "produced inconsistent result after apply" check (plan said null,
// we'd return a populated object). See feedback_server_derived_echo_attrs.
//
// includeUnmanaged inverts those section gates for the list resource's
// config-generation path (terraform query -generate-config-out): there is no
// plan to stay consistent with, so every wire-present optional section is
// allocated and hydrated, yielding a complete exported config rather than a
// general-only one. CRUD callers pass false. The ebook flatteners use the
// wire-authoritative reads (helpers.ReconcileOptionalStringPointer /
// helpers.BoolPointerValueOrNull), which adopt the wire value whatever state
// holds, so allocating an empty section is sufficient for it to fully hydrate.
func assignEbookResourceModel(ctx context.Context, state *EbookResourceModel, e *proclassic.Ebook, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if e == nil {
		return diags
	}

	if id := extractEbookID(e); id != "" {
		state.ID = types.StringValue(id)
	}

	if state.General == nil {
		state.General = &EbookGeneralModel{}
	}
	flattenEbookGeneral(e.General, state.General)

	if includeUnmanaged && state.Scope == nil && e.Scope != nil {
		state.Scope = &EbookScopeModel{}
	}
	if state.Scope != nil && e.Scope != nil {
		flattenEbookScope(ctx, e.Scope, state.Scope, includeUnmanaged)
	}
	if includeUnmanaged && state.SelfService == nil && e.SelfService != nil {
		state.SelfService = &EbookSelfServiceModel{}
	}
	if state.SelfService != nil && e.SelfService != nil {
		flattenEbookSelfService(e.SelfService, state.SelfService)
	}

	return diags
}

// flattenEbookGeneral maps the wire <general> block onto the model. Two fields
// keep a sticky read. deploy_as_managed does not persist: a PUT sending false,
// in isolation, left the GET reading true. file_type is canonicalised by the
// server — a PUT sending `ePub` reads back `EPUB` while `EPUB` round-trips
// unchanged, so reading the wire would rewrite the caller's spelling and never
// converge (the schema already documents that no strict validation applies
// here). Everything else, category_id and site_id included, is echoed
// faithfully and reads from the wire. Wire-probed against Jamf Pro 11.31.1 on
// 2026-09-06; see issue #387.
func flattenEbookGeneral(g *proclassic.EbookGeneral, state *EbookGeneralModel) {
	if g == nil {
		return
	}
	state.ID = helpers.StringValueFromIntPtr(g.ID)
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.URL = helpers.StringPointerValueOrNull(g.URL)
	state.Author = helpers.ReconcileOptionalStringPointer(g.Author, state.Author)
	state.DeploymentType = helpers.ReconcileOptionalStringPointer(g.DeploymentType, state.DeploymentType)
	state.DeployAsManaged = helpers.StickyIgnoringDriftBool(g.DeployAsManaged, state.DeployAsManaged)
	state.Free = helpers.BoolPointerValueOrNull(g.Free)
	state.FileType = helpers.StickyIgnoringDriftString(g.FileType, state.FileType)
	state.Version = helpers.ReconcileOptionalStringPointer(g.Version, state.Version)

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

// flattenEbookScope refreshes the scope sub-blocks the caller already manages.
// When includeUnmanaged is set (config generation) every wire-present sub-block
// is first allocated so the from-scratch read hydrates the full scope rather
// than leaving unmanaged targets/limitations/exclusions null.
func flattenEbookScope(ctx context.Context, s *proclassic.EbookScope, state *EbookScopeModel, includeUnmanaged bool) {
	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &EbookScopeTargetsModel{}
		}
		if state.Limitations == nil && s.Limitations != nil {
			state.Limitations = &EbookScopeLimitationsModel{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &EbookScopeExclusionsModel{}
		}
	}

	// Sub-blocks are gated on caller management (typed-pointer models cannot
	// hold categories without the block struct); within a managed sub-block
	// each category refreshes independently via RefreshManagedSet — a category
	// the caller did not declare (null) stays null, so members maintained in
	// the admin UI never enter state. includeUnmanaged bypasses both gates for
	// import / config-generation hydration and for building the live-side
	// merge base in Update.
	if state.Targets != nil {
		t := state.Targets
		t.AllComputers = scope.RefreshManagedBool(t.AllComputers, s.AllComputers, includeUnmanaged)
		t.AllMobileDevices = scope.RefreshManagedBool(t.AllMobileDevices, s.AllMobileDevices, includeUnmanaged)
		t.AllJssUsers = scope.RefreshManagedBool(t.AllJssUsers, s.AllJssUsers, includeUnmanaged)

		t.ComputerIDs = scope.RefreshManagedSet(t.ComputerIDs, flattenComputerItemSet(ctx, s.Computers), includeUnmanaged)
		t.ComputerGroupIDs = scope.RefreshManagedSet(t.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, computerGroupSlice(s.ComputerGroups)), includeUnmanaged)
		t.MobileDeviceIDs = scope.RefreshManagedSet(t.MobileDeviceIDs, flattenMobileDeviceItemSet(ctx, s.MobileDevices), includeUnmanaged)
		t.MobileDeviceGroupIDs = scope.RefreshManagedSet(t.MobileDeviceGroupIDs, scope.FlattenIDNameSet(ctx, mobileDeviceGroupSlice(s.MobileDeviceGroups)), includeUnmanaged)
		t.BuildingIDs = scope.RefreshManagedSet(t.BuildingIDs, scope.FlattenIDNameSet(ctx, buildingSlice(s.Buildings)), includeUnmanaged)
		t.DepartmentIDs = scope.RefreshManagedSet(t.DepartmentIDs, scope.FlattenIDNameSet(ctx, departmentSlice(s.Departments)), includeUnmanaged)
		t.UserIDs = scope.RefreshManagedSet(t.UserIDs, scope.FlattenIDNameSet(ctx, jssUserSlice(s.JssUsers)), includeUnmanaged)
		t.UserGroupIDs = scope.RefreshManagedSet(t.UserGroupIDs, scope.FlattenIDNameSet(ctx, jssUserGroupSlice(s.JssUserGroups)), includeUnmanaged)
		t.ClassIDs = scope.RefreshManagedSet(t.ClassIDs, scope.FlattenIDNameSet(ctx, classSlice(s.Classes)), includeUnmanaged)
	}

	if state.Limitations != nil && s.Limitations != nil {
		l, sl := state.Limitations, s.Limitations
		l.NetworkSegmentIDs = scope.RefreshManagedSet(l.NetworkSegmentIDs, scope.FlattenIDNameSet(ctx, limitationsSegmentSlice(sl.NetworkSegments)), includeUnmanaged)
		l.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(l.DirectoryServiceOrLocalUserNames, scope.FlattenNameSet(ctx, limitationsUserSlice(sl.Users)), includeUnmanaged)
		l.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(l.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, limitationsUserGroupSlice(sl.UserGroups)), includeUnmanaged)
	}

	if state.Exclusions != nil && s.Exclusions != nil {
		x, e := state.Exclusions, s.Exclusions
		x.ComputerIDs = scope.RefreshManagedSet(x.ComputerIDs, flattenExclComputerItemSet(ctx, e.Computers), includeUnmanaged)
		x.ComputerGroupIDs = scope.RefreshManagedSet(x.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, exclComputerGroupSlice(e.ComputerGroups)), includeUnmanaged)
		x.MobileDeviceIDs = scope.RefreshManagedSet(x.MobileDeviceIDs, flattenExclMobileDeviceItemSet(ctx, e.MobileDevices), includeUnmanaged)
		x.MobileDeviceGroupIDs = scope.RefreshManagedSet(x.MobileDeviceGroupIDs, scope.FlattenIDNameSet(ctx, exclMobileDeviceGroupSlice(e.MobileDeviceGroups)), includeUnmanaged)
		x.BuildingIDs = scope.RefreshManagedSet(x.BuildingIDs, scope.FlattenIDNameSet(ctx, exclBuildingSlice(e.Buildings)), includeUnmanaged)
		x.DepartmentIDs = scope.RefreshManagedSet(x.DepartmentIDs, scope.FlattenIDNameSet(ctx, exclDepartmentSlice(e.Departments)), includeUnmanaged)
		x.UserIDs = scope.RefreshManagedSet(x.UserIDs, scope.FlattenIDNameSet(ctx, exclJssUserSlice(e.JssUsers)), includeUnmanaged)
		x.UserGroupIDs = scope.RefreshManagedSet(x.UserGroupIDs, scope.FlattenIDNameSet(ctx, exclJssUserGroupSlice(e.JssUserGroups)), includeUnmanaged)
		x.NetworkSegmentIDs = scope.RefreshManagedSet(x.NetworkSegmentIDs, flattenExclNetworkSegmentSet(ctx, e.NetworkSegments), includeUnmanaged)
		x.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(x.DirectoryServiceOrLocalUserNames, flattenExclUsersNameSet(ctx, e.Users), includeUnmanaged)
		x.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(x.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, exclUserGroupSlice(e.UserGroups)), includeUnmanaged)
	}
}

// flattenEbookSelfService maps the wire <self_service> block onto the model.
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
func flattenEbookSelfService(ss *proclassic.EbookSelfService, state *EbookSelfServiceModel) {
	state.DisplayName = helpers.ReconcileOptionalStringPointer(ss.SelfServiceDisplayName, state.DisplayName)
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

	if ss.SelfServiceIcon != nil {
		state.IconID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(ss.SelfServiceIcon.ID), state.IconID)
		state.IconURI = helpers.StringPointerValueOrNull(ss.SelfServiceIcon.URI)
	} else {
		state.IconID = helpers.ReconcileOptionalStringPointer(nil, state.IconID)
		state.IconURI = types.StringNull()
	}

	if state.Categories != nil && ss.SelfServiceCategories != nil && ss.SelfServiceCategories.Category != nil {
		flattenEbookSelfServiceCategories(*ss.SelfServiceCategories.Category, state)
	}
}

// flattenEbookSelfServiceCategories refreshes the managed category set, matching
// server items to existing state items by ID so caller-authored values
// (display_in / feature_in) stick across refreshes. The set is keyed by category ID.
func flattenEbookSelfServiceCategories(api []proclassic.EbookSelfServiceSelfServiceCategoriesCategoryItem, state *EbookSelfServiceModel) {
	byID := make(map[string]EbookSelfServiceCategoryModel, len(state.Categories))
	for _, c := range state.Categories {
		byID[c.ID.ValueString()] = c
	}

	out := make([]EbookSelfServiceCategoryModel, 0, len(api))
	for _, c := range api {
		idStr := ""
		if s := helpers.StringFromIntPtr(c.ID); s != nil {
			idStr = *s
		}
		current := byID[idStr]
		out = append(out, EbookSelfServiceCategoryModel{
			ID:        types.StringValue(idStr),
			Name:      helpers.ReconcileOptionalStringPointer(c.Name, current.Name),
			DisplayIn: helpers.BoolPointerValueOrNull(c.DisplayIn),
			FeatureIn: helpers.BoolPointerValueOrNull(c.FeatureIn),
		})
	}
	state.Categories = out
}

// ---- scope sub-slice accessors -------------------------------------------------

func computerGroupSlice(g *proclassic.EbookScopeComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func mobileDeviceGroupSlice(g *proclassic.EbookScopeMobileDeviceGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.MobileDeviceGroup
}

func buildingSlice(b *proclassic.EbookScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func departmentSlice(d *proclassic.EbookScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func jssUserSlice(u *proclassic.EbookScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func jssUserGroupSlice(u *proclassic.EbookScopeJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func classSlice(c *proclassic.EbookScopeClasses) *[]proclassic.IDName {
	if c == nil {
		return nil
	}
	return c.Class
}

func limitationsSegmentSlice(s *proclassic.EbookScopeLimitationsNetworkSegments) *[]proclassic.IDName {
	if s == nil {
		return nil
	}
	return s.NetworkSegment
}

func limitationsUserSlice(u *proclassic.EbookScopeLimitationsUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func limitationsUserGroupSlice(u *proclassic.EbookScopeLimitationsUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclComputerGroupSlice(g *proclassic.EbookScopeExclusionsComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func exclMobileDeviceGroupSlice(g *proclassic.EbookScopeExclusionsMobileDeviceGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.MobileDeviceGroup
}

func exclBuildingSlice(b *proclassic.EbookScopeExclusionsBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func exclDepartmentSlice(d *proclassic.EbookScopeExclusionsDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func exclJssUserSlice(u *proclassic.EbookScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func exclJssUserGroupSlice(u *proclassic.EbookScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclUserGroupSlice(u *proclassic.EbookScopeExclusionsUserGroups) *[]proclassic.IDName {
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

func flattenComputerItemSet(ctx context.Context, c *proclassic.EbookScopeComputers) types.Set {
	if c == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, c.Computer, func(i proclassic.EbookScopeComputersComputerItem) *int { return i.ID })
	return out
}

func flattenMobileDeviceItemSet(ctx context.Context, m *proclassic.EbookScopeMobileDevices) types.Set {
	if m == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, m.MobileDevice, func(i proclassic.EbookScopeMobileDevicesMobileDeviceItem) *int { return i.ID })
	return out
}

func flattenExclComputerItemSet(ctx context.Context, c *proclassic.EbookScopeExclusionsComputers) types.Set {
	if c == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, c.Computer, func(i proclassic.EbookScopeExclusionsComputersComputerItem) *int { return i.ID })
	return out
}

func flattenExclMobileDeviceItemSet(ctx context.Context, m *proclassic.EbookScopeExclusionsMobileDevices) types.Set {
	if m == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, m.MobileDevice, func(i proclassic.EbookScopeExclusionsMobileDevicesMobileDeviceItem) *int { return i.ID })
	return out
}

func flattenExclNetworkSegmentSet(ctx context.Context, n *proclassic.EbookScopeExclusionsNetworkSegments) types.Set {
	if n == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, n.NetworkSegment, func(i proclassic.EbookScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int { return i.ID })
	return out
}

func flattenExclUsersNameSet(ctx context.Context, u *proclassic.EbookScopeExclusionsUsers) types.Set {
	if u == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenNameSlice(ctx, u.User, func(i proclassic.EbookScopeExclusionsUsersUserItem) *string { return i.Name })
	return out
}
