// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildMacAppInput projects a plan MacAppResourceModel into an SDK
// *proclassic.MacApplication suitable for Create / Update. Each section follows
// the scope omission rules in STYLE_GUIDE.md §Scope helper: nil-pointer
// sub-blocks suppress wire emission entirely; empty child collections collapse
// all the way up to a nil parent.
func buildMacAppInput(ctx context.Context, plan MacAppResourceModel) (*proclassic.MacApplication, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &proclassic.MacApplication{}

	if plan.General != nil {
		out.General = buildMacAppGeneral(plan.General)
	}

	if plan.Scope != nil {
		s, d := buildMacAppScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	if plan.SelfService != nil {
		out.SelfService = buildMacAppSelfService(plan.SelfService)
	}

	if plan.Vpp != nil {
		out.Vpp = buildMacAppVpp(plan.Vpp)
	}

	return out, diags
}

func buildMacAppGeneral(m *MacAppGeneralModel) *proclassic.MacApplicationGeneral {
	g := &proclassic.MacApplicationGeneral{
		Name:           helpers.OptionalStringPointer(m.Name),
		Version:        helpers.OptionalStringPointer(m.Version),
		BundleID:       helpers.OptionalStringPointer(m.BundleID),
		URL:            helpers.OptionalStringPointer(m.URL),
		IsFree:         helpers.OptionalBoolPointer(m.IsFree),
		DeploymentType: helpers.OptionalStringPointer(m.DeploymentType),
	}
	if catID := helpers.StringIDPtr(m.CategoryID); catID != nil {
		g.Category = &proclassic.CategoryObject{ID: catID}
	}
	if siteID := helpers.StringIDPtr(m.SiteID); siteID != nil {
		g.Site = &proclassic.SiteObject{ID: siteID}
	}
	return g
}

func buildMacAppScope(ctx context.Context, m *scope.ComputerScopeModelNoIbeacons) (*proclassic.MacApplicationScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.MacApplicationScope{
		AllComputers: helpers.OptionalBoolPointer(t.AllComputers),
		AllJssUsers:  helpers.OptionalBoolPointer(t.AllJssUsers),
	}

	computers, d := scope.BuildIDSlice(ctx, t.ComputerIDs, func(id int) proclassic.MacApplicationScopeComputersComputerItem {
		return proclassic.MacApplicationScopeComputersComputerItem{ID: &id}
	})
	diags.Append(d...)
	if computers != nil {
		s.Computers = &proclassic.MacApplicationScopeComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, t.ComputerGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if computerGroups != nil {
		s.ComputerGroups = &proclassic.MacApplicationScopeComputerGroups{ComputerGroup: computerGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, t.BuildingIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if buildings != nil {
		s.Buildings = &proclassic.MacApplicationScopeBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, t.DepartmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if departments != nil {
		s.Departments = &proclassic.MacApplicationScopeDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, t.UserIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUsers != nil {
		s.JssUsers = &proclassic.MacApplicationScopeJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, t.UserGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUserGroups != nil {
		s.JssUserGroups = &proclassic.MacApplicationScopeJssUserGroups{UserGroup: jssUserGroups}
	}

	if m.Limitations != nil {
		l, ld := buildMacAppScopeLimitations(ctx, m.Limitations)
		diags.Append(ld...)
		s.Limitations = l
	}

	if m.Exclusions != nil {
		e, ed := buildMacAppScopeExclusions(ctx, m.Exclusions)
		diags.Append(ed...)
		s.Exclusions = e
	}

	// Omission semantics (STYLE_GUIDE.md §Scope helper): collapse to nil when
	// every child pointer is nil so the payload omits <scope> entirely.
	if s.AllComputers == nil && s.AllJssUsers == nil && s.Computers == nil &&
		s.ComputerGroups == nil && s.Buildings == nil && s.Departments == nil &&
		s.JssUsers == nil && s.JssUserGroups == nil &&
		s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

func buildMacAppScopeLimitations(ctx context.Context, m *scope.ComputerScopeLimitationsModelNoIbeacons) (*proclassic.MacApplicationScopeLimitations, diag.Diagnostics) {
	var diags diag.Diagnostics
	l := &proclassic.MacApplicationScopeLimitations{}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		l.NetworkSegments = &proclassic.MacApplicationScopeLimitationsNetworkSegments{NetworkSegment: segs}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		l.Users = &proclassic.MacApplicationScopeLimitationsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if userGroups != nil {
		l.UserGroups = &proclassic.MacApplicationScopeLimitationsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when the caller's model declares `limitations`. A
	// scope PUT replaces the whole subtree, so an explicit empty element is the
	// clear gesture for a declared-empty category; undeclared (null) categories
	// are preserved upstream by the read-merge-write update, which hands this
	// builder a fully non-null merged model.
	return l, diags
}

func buildMacAppScopeExclusions(ctx context.Context, m *scope.ComputerScopeExclusionsModelNoIbeacons) (*proclassic.MacApplicationScopeExclusions, diag.Diagnostics) {
	var diags diag.Diagnostics
	e := &proclassic.MacApplicationScopeExclusions{}

	computers, d := scope.BuildIDSlice(ctx, m.ComputerIDs, func(id int) proclassic.MacApplicationScopeExclusionsComputersComputerItem {
		return proclassic.MacApplicationScopeExclusionsComputersComputerItem{ID: &id}
	})
	diags.Append(d...)
	if computers != nil {
		e.Computers = &proclassic.MacApplicationScopeExclusionsComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, m.ComputerGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if computerGroups != nil {
		e.ComputerGroups = &proclassic.MacApplicationScopeExclusionsComputerGroups{ComputerGroup: computerGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, m.BuildingIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if buildings != nil {
		e.Buildings = &proclassic.MacApplicationScopeExclusionsBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, m.DepartmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if departments != nil {
		e.Departments = &proclassic.MacApplicationScopeExclusionsDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, m.UserIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUsers != nil {
		e.JssUsers = &proclassic.MacApplicationScopeExclusionsJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, m.UserGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUserGroups != nil {
		e.JssUserGroups = &proclassic.MacApplicationScopeExclusionsJssUserGroups{UserGroup: jssUserGroups}
	}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.MacApplicationScopeExclusionsNetworkSegmentsNetworkSegmentItem {
		return proclassic.MacApplicationScopeExclusionsNetworkSegmentsNetworkSegmentItem{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		e.NetworkSegments = &proclassic.MacApplicationScopeExclusionsNetworkSegments{NetworkSegment: segs}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.MacApplicationScopeExclusionsUsersUserItem {
		n := name
		return proclassic.MacApplicationScopeExclusionsUsersUserItem{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		e.Users = &proclassic.MacApplicationScopeExclusionsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if userGroups != nil {
		e.UserGroups = &proclassic.MacApplicationScopeExclusionsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when declared — see buildMacAppScopeLimitations.
	return e, diags
}

func buildMacAppSelfService(m *MacAppSelfServiceModel) *proclassic.MacApplicationSelfService {
	ss := &proclassic.MacApplicationSelfService{
		InstallButtonText:           helpers.OptionalStringPointer(m.InstallButtonText),
		SelfServiceDescription:      helpers.OptionalStringPointer(m.SelfServiceDescription),
		ForceUsersToViewDescription: helpers.OptionalBoolPointer(m.ForceUsersToViewDescription),
		FeatureOnMainPage:           helpers.OptionalBoolPointer(m.FeatureOnMainPage),
		Notification:                buildMacAppNotification(m.NotificationEnabled, m.NotificationMethod),
		NotificationSubject:         helpers.OptionalStringPointer(m.NotificationSubject),
		NotificationMessage:         helpers.OptionalStringPointer(m.NotificationMessage),
	}

	if m.SelfServiceIcon != nil {
		icon := &proclassic.MacApplicationSelfServiceSelfServiceIcon{}
		if id := helpers.StringIDPtr(m.SelfServiceIcon.ID); id != nil {
			icon.ID = id
		}
		if icon.ID != nil {
			ss.SelfServiceIcon = icon
		}
	}

	if len(m.SelfServiceCategories) > 0 {
		cats := make([]proclassic.MacApplicationSelfServiceSelfServiceCategoriesCategoryItem, 0, len(m.SelfServiceCategories))
		for _, c := range m.SelfServiceCategories {
			item := proclassic.MacApplicationSelfServiceSelfServiceCategoriesCategoryItem{
				ID:        helpers.StringIDPtr(c.ID),
				Name:      helpers.OptionalStringPointer(c.Name),
				DisplayIn: helpers.SelfServiceCategoryDisplayIn(c.DisplayIn),
				FeatureIn: helpers.OptionalBoolPointer(c.FeatureIn),
			}
			cats = append(cats, item)
		}
		ss.SelfServiceCategories = &proclassic.MacApplicationSelfServiceSelfServiceCategories{Category: &cats}
	}

	return ss
}

func buildMacAppVpp(m *MacAppVppModel) *proclassic.MacApplicationVpp {
	v := &proclassic.MacApplicationVpp{
		AssignVppDeviceBasedLicenses: helpers.OptionalBoolPointer(m.AssignVppDeviceBasedLicenses),
		VppAdminAccountID:            helpers.StringIDPtr(m.VppAdminAccountID),
	}
	// License counts are server-computed; never written.
	if v.AssignVppDeviceBasedLicenses == nil && v.VppAdminAccountID == nil {
		return nil
	}
	return v
}
