// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildEbookInput projects a plan EbookResourceModel into an SDK
// *proclassic.EbookPost suitable for Create / Update. The full plan is sent on
// every write (classic PUT is a partial-merge, so omission never clears — see
// crud.go header). Scope categories follow the granular-ownership omission
// rules in STYLE_GUIDE.md §Scope helper: a null (unmanaged) category is
// omitted from the body; a declared category — including `[]`, via
// BuildIDSlice/BuildNameSlice returning a non-nil empty slice — is emitted
// explicitly, empty wrapper and all, because the scope subtree replaces
// wholesale once any category element is present. On Update the caller passes
// a plan whose Scope is the read-merge-write merged model, so every category
// is non-null and the full explicit skeleton is emitted.
func buildEbookInput(ctx context.Context, plan EbookResourceModel) (*proclassic.EbookPost, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &proclassic.EbookPost{}

	if plan.General != nil {
		out.General = buildEbookGeneral(plan.General)
	}

	if plan.Scope != nil {
		s, d := buildEbookScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	if plan.SelfService != nil {
		out.SelfService = buildEbookSelfService(plan.SelfService)

		// The Self Service icon is stored under BOTH <general> and
		// <self_service> on the wire (wire-probed — setting it in <general>
		// propagates to both). Mirror that by stamping the icon id into general
		// too so a repeated GET round-trips identically.
		if iconID := helpers.StringIDPtr(plan.SelfService.IconID); iconID != nil {
			if out.General == nil {
				out.General = &proclassic.EbookPostGeneral{}
			}
			out.General.SelfServiceIcon = &proclassic.EbookGeneralSelfServiceIcon{ID: iconID}
		}
	}

	return out, diags
}

// ebookScopeClassesCleared returns the first of the two requests an ebook
// whose scope carries class members needs, or nil when one request will do.
//
// Jamf Pro stores <classes> only while the stored category is empty. A
// populated-to-populated transition is refused and the category is cleared
// instead, whatever the body carries — re-sending the identical <classes>
// cleared it 5/5, as did omitting it (wire-probed 2026-09-12 on 11.31.1,
// /proclassic/ebooks). No other scope category behaves this way, and because a
// scope PUT replaces the whole subtree there is no single body that preserves a
// stored class across another scope change. Two bodies do: the first empties
// the category while applying every other change, the second sets the classes
// against the now-empty category (verified 5/5 on the same build).
//
// The returned payload is the caller's own body with an explicitly empty
// <classes>, so the first request already carries the real change and an apply
// interrupted between the two leaves exactly what a single request leaves — the
// change applied, the classes empty, restored by the next apply.
//
// The gate is the category rather than the resource: a scope with no class
// members takes the single-request path, so the extra write happens only where
// it is the difference between keeping and losing the stored classes, and a
// resource that gains the category later needs no further change here.
func ebookScopeClassesCleared(p *proclassic.EbookPost) *proclassic.EbookPost {
	if p == nil || p.Scope == nil || p.Scope.Classes == nil || p.Scope.Classes.Class == nil || len(*p.Scope.Classes.Class) == 0 {
		return nil
	}
	cleared := *p
	clearedScope := *p.Scope
	clearedScope.Classes = &proclassic.EbookScopeClasses{Class: &[]proclassic.IDName{}}
	cleared.Scope = &clearedScope
	return &cleared
}

func buildEbookGeneral(m *EbookGeneralModel) *proclassic.EbookPostGeneral {
	g := &proclassic.EbookPostGeneral{
		Name:            helpers.OptionalStringPointer(m.Name),
		Author:          helpers.OptionalStringPointer(m.Author),
		URL:             helpers.OptionalStringPointer(m.URL),
		DeploymentType:  helpers.OptionalStringPointer(m.DeploymentType),
		DeployAsManaged: helpers.OptionalBoolPointer(m.DeployAsManaged),
		Free:            helpers.OptionalBoolPointer(m.Free),
		FileType:        helpers.OptionalStringPointer(m.FileType),
		Version:         helpers.OptionalStringPointer(m.Version),
	}
	if catID := helpers.StringIDPtr(m.CategoryID); catID != nil {
		g.Category = &proclassic.CategoryObject{ID: catID}
	}
	if siteID := helpers.StringIDPtr(m.SiteID); siteID != nil {
		g.Site = &proclassic.SiteObject{ID: siteID}
	}
	return g
}

func buildEbookScope(ctx context.Context, m *EbookScopeModel) (*proclassic.EbookPostScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.EbookPostScope{
		AllComputers:     helpers.OptionalBoolPointer(t.AllComputers),
		AllMobileDevices: helpers.OptionalBoolPointer(t.AllMobileDevices),
		AllJssUsers:      helpers.OptionalBoolPointer(t.AllJssUsers),
	}

	computers, d := scope.BuildIDSlice(ctx, t.ComputerIDs, func(id int) proclassic.EbookScopeComputersComputerItem {
		return proclassic.EbookScopeComputersComputerItem{ID: &id}
	})
	diags.Append(d...)
	if computers != nil {
		s.Computers = &proclassic.EbookScopeComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, t.ComputerGroupIDs, idNameFromInt)
	diags.Append(d...)
	if computerGroups != nil {
		s.ComputerGroups = &proclassic.EbookScopeComputerGroups{ComputerGroup: computerGroups}
	}

	mobileDevices, d := scope.BuildIDSlice(ctx, t.MobileDeviceIDs, func(id int) proclassic.EbookScopeMobileDevicesMobileDeviceItem {
		return proclassic.EbookScopeMobileDevicesMobileDeviceItem{ID: &id}
	})
	diags.Append(d...)
	if mobileDevices != nil {
		s.MobileDevices = &proclassic.EbookScopeMobileDevices{MobileDevice: mobileDevices}
	}

	mobileDeviceGroups, d := scope.BuildIDSlice(ctx, t.MobileDeviceGroupIDs, idNameFromInt)
	diags.Append(d...)
	if mobileDeviceGroups != nil {
		s.MobileDeviceGroups = &proclassic.EbookScopeMobileDeviceGroups{MobileDeviceGroup: mobileDeviceGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, t.BuildingIDs, idNameFromInt)
	diags.Append(d...)
	if buildings != nil {
		s.Buildings = &proclassic.EbookScopeBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, t.DepartmentIDs, idNameFromInt)
	diags.Append(d...)
	if departments != nil {
		s.Departments = &proclassic.EbookScopeDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, t.UserIDs, idNameFromInt)
	diags.Append(d...)
	if jssUsers != nil {
		s.JssUsers = &proclassic.EbookScopeJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, t.UserGroupIDs, idNameFromInt)
	diags.Append(d...)
	if jssUserGroups != nil {
		s.JssUserGroups = &proclassic.EbookScopeJssUserGroups{UserGroup: jssUserGroups}
	}

	classes, d := scope.BuildIDSlice(ctx, t.ClassIDs, idNameFromInt)
	diags.Append(d...)
	if classes != nil {
		s.Classes = &proclassic.EbookScopeClasses{Class: classes}
	}

	if m.Limitations != nil {
		l, ld := buildEbookScopeLimitations(ctx, m.Limitations)
		diags.Append(ld...)
		s.Limitations = l
	}

	if m.Exclusions != nil {
		e, ed := buildEbookScopeExclusions(ctx, m.Exclusions)
		diags.Append(ed...)
		s.Exclusions = e
	}

	// Omission semantics: collapse to nil when every child is nil so the payload
	// omits <scope> entirely.
	if s.AllComputers == nil && s.AllMobileDevices == nil && s.AllJssUsers == nil &&
		s.Computers == nil && s.ComputerGroups == nil && s.MobileDevices == nil &&
		s.MobileDeviceGroups == nil && s.Buildings == nil && s.Departments == nil &&
		s.JssUsers == nil && s.JssUserGroups == nil && s.Classes == nil &&
		s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

func buildEbookScopeLimitations(ctx context.Context, m *EbookScopeLimitationsModel) (*proclassic.EbookScopeLimitations, diag.Diagnostics) {
	var diags diag.Diagnostics
	l := &proclassic.EbookScopeLimitations{}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, idNameFromInt)
	diags.Append(d...)
	if segs != nil {
		l.NetworkSegments = &proclassic.EbookScopeLimitationsNetworkSegments{NetworkSegment: segs}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, idNameFromName)
	diags.Append(d...)
	if users != nil {
		l.Users = &proclassic.EbookScopeLimitationsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, idNameFromName)
	diags.Append(d...)
	if userGroups != nil {
		l.UserGroups = &proclassic.EbookScopeLimitationsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when the user declared `limitations` (the caller's
	// gate): a scope PUT replaces the whole subtree once any category element is
	// present (wire-probed 2026-07-08), so a declared-but-empty block must land
	// as an explicit <limitations></limitations> — the clear gesture for `[]`.
	// Undeclared (null) categories inside a declared block are handled by the
	// Update merge, which re-emits the live members.
	return l, diags
}

func buildEbookScopeExclusions(ctx context.Context, m *EbookScopeExclusionsModel) (*proclassic.EbookScopeExclusions, diag.Diagnostics) {
	var diags diag.Diagnostics
	e := &proclassic.EbookScopeExclusions{}

	computers, d := scope.BuildIDSlice(ctx, m.ComputerIDs, func(id int) proclassic.EbookScopeExclusionsComputersComputerItem {
		return proclassic.EbookScopeExclusionsComputersComputerItem{ID: &id}
	})
	diags.Append(d...)
	if computers != nil {
		e.Computers = &proclassic.EbookScopeExclusionsComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, m.ComputerGroupIDs, idNameFromInt)
	diags.Append(d...)
	if computerGroups != nil {
		e.ComputerGroups = &proclassic.EbookScopeExclusionsComputerGroups{ComputerGroup: computerGroups}
	}

	mobileDevices, d := scope.BuildIDSlice(ctx, m.MobileDeviceIDs, func(id int) proclassic.EbookScopeExclusionsMobileDevicesMobileDeviceItem {
		return proclassic.EbookScopeExclusionsMobileDevicesMobileDeviceItem{ID: &id}
	})
	diags.Append(d...)
	if mobileDevices != nil {
		e.MobileDevices = &proclassic.EbookScopeExclusionsMobileDevices{MobileDevice: mobileDevices}
	}

	mobileDeviceGroups, d := scope.BuildIDSlice(ctx, m.MobileDeviceGroupIDs, idNameFromInt)
	diags.Append(d...)
	if mobileDeviceGroups != nil {
		e.MobileDeviceGroups = &proclassic.EbookScopeExclusionsMobileDeviceGroups{MobileDeviceGroup: mobileDeviceGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, m.BuildingIDs, idNameFromInt)
	diags.Append(d...)
	if buildings != nil {
		e.Buildings = &proclassic.EbookScopeExclusionsBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, m.DepartmentIDs, idNameFromInt)
	diags.Append(d...)
	if departments != nil {
		e.Departments = &proclassic.EbookScopeExclusionsDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, m.UserIDs, idNameFromInt)
	diags.Append(d...)
	if jssUsers != nil {
		e.JssUsers = &proclassic.EbookScopeExclusionsJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, m.UserGroupIDs, idNameFromInt)
	diags.Append(d...)
	if jssUserGroups != nil {
		e.JssUserGroups = &proclassic.EbookScopeExclusionsJssUserGroups{UserGroup: jssUserGroups}
	}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.EbookScopeExclusionsNetworkSegmentsNetworkSegmentItem {
		return proclassic.EbookScopeExclusionsNetworkSegmentsNetworkSegmentItem{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		e.NetworkSegments = &proclassic.EbookScopeExclusionsNetworkSegments{NetworkSegment: segs}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.EbookScopeExclusionsUsersUserItem {
		n := name
		return proclassic.EbookScopeExclusionsUsersUserItem{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		e.Users = &proclassic.EbookScopeExclusionsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, idNameFromName)
	diags.Append(d...)
	if userGroups != nil {
		e.UserGroups = &proclassic.EbookScopeExclusionsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when declared — see buildEbookScopeLimitations.
	return e, diags
}

func buildEbookSelfService(m *EbookSelfServiceModel) *proclassic.EbookPostSelfService {
	ss := &proclassic.EbookPostSelfService{
		SelfServiceDisplayName:      helpers.OptionalStringPointer(m.DisplayName),
		InstallButtonText:           helpers.OptionalStringPointer(m.InstallButtonText),
		SelfServiceDescription:      helpers.OptionalStringPointer(m.SelfServiceDescription),
		ForceUsersToViewDescription: helpers.OptionalBoolPointer(m.ForceUsersToViewDescription),
		FeatureOnMainPage:           helpers.OptionalBoolPointer(m.FeatureOnMainPage),
		Notification:                buildEbookNotification(m.NotificationEnabled, m.NotificationMethod),
		NotificationSubject:         helpers.OptionalStringPointer(m.NotificationSubject),
		NotificationMessage:         helpers.OptionalStringPointer(m.NotificationMessage),
	}

	if iconID := helpers.StringIDPtr(m.IconID); iconID != nil {
		ss.SelfServiceIcon = &proclassic.EbookSelfServiceSelfServiceIcon{ID: iconID}
	}

	if len(m.Categories) > 0 {
		cats := make([]proclassic.EbookSelfServiceSelfServiceCategoriesCategoryItem, 0, len(m.Categories))
		for _, c := range m.Categories {
			cats = append(cats, proclassic.EbookSelfServiceSelfServiceCategoriesCategoryItem{
				ID:        helpers.StringIDPtr(c.ID),
				Name:      helpers.OptionalStringPointer(c.Name),
				DisplayIn: helpers.SelfServiceCategoryDisplayIn(c.DisplayIn),
				FeatureIn: helpers.OptionalBoolPointer(c.FeatureIn),
			})
		}
		ss.SelfServiceCategories = &proclassic.EbookSelfServiceSelfServiceCategories{Category: &cats}
	}

	return ss
}

// idNameFromInt wraps a numeric ID in a proclassic.IDName for ID-keyed scope
// sub-blocks (computer groups, mobile device groups, buildings, departments,
// jss users, jss user groups, classes, network segments).
func idNameFromInt(id int) proclassic.IDName {
	return proclassic.IDName{ID: &id}
}

// idNameFromName wraps a name in a proclassic.IDName for name-keyed scope
// sub-blocks (directory-service users / user groups in limitations and
// exclusion user groups).
func idNameFromName(name string) proclassic.IDName {
	n := name
	return proclassic.IDName{Name: &n}
}
