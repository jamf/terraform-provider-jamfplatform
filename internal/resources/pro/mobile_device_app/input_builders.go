// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildMobileAppInput projects a plan MobileAppResourceModel into an SDK
// *proclassic.MobileDeviceApplication suitable for Create / Update. Each section
// follows the scope omission rules in STYLE_GUIDE.md §Scope helper: nil-pointer
// sub-blocks suppress wire emission entirely; empty child collections collapse
// all the way up to a nil parent.
func buildMobileAppInput(ctx context.Context, plan MobileAppResourceModel) (*proclassic.MobileDeviceApplication, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &proclassic.MobileDeviceApplication{}

	if plan.General != nil {
		out.General = buildMobileAppGeneral(plan.General)
	}

	if plan.Scope != nil {
		s, d := buildMobileAppScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	if plan.SelfService != nil {
		out.SelfService = buildMobileAppSelfService(plan.SelfService)
	}

	if plan.Vpp != nil {
		out.Vpp = buildMobileAppVpp(plan.Vpp)
	}

	if plan.AppConfiguration != nil {
		out.AppConfiguration = buildMobileAppAppConfiguration(plan.AppConfiguration)
	}

	return out, diags
}

// buildMobileAppGeneral maps the general block onto the wire payload.
//
// DeploymentType is never sent: Jamf Pro derives it from DeployAutomatically and
// discards a write to it, on create and on update alike. See mirrorString in
// resource.go.
func buildMobileAppGeneral(m *MobileAppGeneralModel) *proclassic.MobileDeviceApplicationGeneral {
	g := &proclassic.MobileDeviceApplicationGeneral{
		Name:                             helpers.OptionalStringPointer(m.Name),
		Version:                          helpers.OptionalStringPointer(m.Version),
		BundleID:                         helpers.OptionalStringPointer(m.BundleID),
		OsType:                           helpers.OptionalStringPointer(m.OsType),
		Free:                             helpers.OptionalBoolPointer(m.IsFree),
		ExternalURL:                      helpers.OptionalStringPointer(m.ExternalURL),
		ItunesStoreURL:                   helpers.OptionalStringPointer(m.ItunesStoreURL),
		ItunesCountryRegion:              helpers.OptionalStringPointer(m.ItunesCountryRegion),
		ItunesSyncTime:                   optionalIntPointer(m.ItunesSyncTime),
		MakeAvailableAfterInstall:        helpers.OptionalBoolPointer(m.MakeAvailableAfterInstall),
		KeepDescriptionAndIconUpToDate:   helpers.OptionalBoolPointer(m.KeepDescriptionAndIconUpToDate),
		KeepAppUpdatedOnDevices:          helpers.OptionalBoolPointer(m.KeepAppUpdatedOnDevices),
		DeployAsManagedApp:               helpers.OptionalBoolPointer(m.DeployAsManagedApp),
		TakeOverManagement:               helpers.OptionalBoolPointer(m.TakeOverManagement),
		DeployAutomatically:              helpers.OptionalBoolPointer(m.DeployAutomatically),
		RemoveAppWhenMDMProfileIsRemoved: helpers.OptionalBoolPointer(m.RemoveAppWhenMDMProfileIsRemoved),
		PreventBackupOfAppData:           helpers.OptionalBoolPointer(m.PreventBackupOfAppData),
		AllowUserToDelete:                helpers.OptionalBoolPointer(m.AllowUserToDelete),
		RequireNetworkTethered:           helpers.OptionalBoolPointer(m.RequireNetworkTethered),
		HostExternally:                   helpers.OptionalBoolPointer(m.HostExternally),
	}
	if catID := helpers.StringIDPtr(m.CategoryID); catID != nil {
		g.Category = &proclassic.CategoryObject{ID: catID}
	}
	if siteID := helpers.StringIDPtr(m.SiteID); siteID != nil {
		g.Site = &proclassic.SiteObject{ID: siteID}
	}
	return g
}

func buildMobileAppScope(ctx context.Context, m *scope.MobileScopeModelNoIbeacons) (*proclassic.MobileDeviceApplicationScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.MobileDeviceApplicationScope{
		AllMobileDevices: helpers.OptionalBoolPointer(t.AllMobileDevices),
		AllJssUsers:      helpers.OptionalBoolPointer(t.AllJssUsers),
	}

	mds, d := scope.BuildIDSlice(ctx, t.MobileDeviceIDs, func(id int) proclassic.MobileDeviceApplicationScopeMobileDevicesMobileDeviceItem {
		return proclassic.MobileDeviceApplicationScopeMobileDevicesMobileDeviceItem{ID: &id}
	})
	diags.Append(d...)
	if mds != nil {
		s.MobileDevices = &proclassic.MobileDeviceApplicationScopeMobileDevices{MobileDevice: mds}
	}

	mdgs, d := scope.BuildIDSlice(ctx, t.MobileDeviceGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if mdgs != nil {
		s.MobileDeviceGroups = &proclassic.MobileDeviceApplicationScopeMobileDeviceGroups{MobileDeviceGroup: mdgs}
	}

	buildings, d := scope.BuildIDSlice(ctx, t.BuildingIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if buildings != nil {
		s.Buildings = &proclassic.MobileDeviceApplicationScopeBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, t.DepartmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if departments != nil {
		s.Departments = &proclassic.MobileDeviceApplicationScopeDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, t.UserIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUsers != nil {
		s.JssUsers = &proclassic.MobileDeviceApplicationScopeJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, t.UserGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUserGroups != nil {
		s.JssUserGroups = &proclassic.MobileDeviceApplicationScopeJssUserGroups{UserGroup: jssUserGroups}
	}

	if m.Limitations != nil {
		l, ld := buildMobileAppScopeLimitations(ctx, m.Limitations)
		diags.Append(ld...)
		s.Limitations = l
	}

	if m.Exclusions != nil {
		e, ed := buildMobileAppScopeExclusions(ctx, m.Exclusions)
		diags.Append(ed...)
		s.Exclusions = e
	}

	// Omission semantics (STYLE_GUIDE.md §Scope helper): collapse to nil when
	// every child pointer is nil so the payload omits <scope> entirely.
	if s.AllMobileDevices == nil && s.AllJssUsers == nil && s.MobileDevices == nil &&
		s.MobileDeviceGroups == nil && s.Buildings == nil && s.Departments == nil &&
		s.JssUsers == nil && s.JssUserGroups == nil &&
		s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

func buildMobileAppScopeLimitations(ctx context.Context, m *scope.MobileScopeLimitationsModelNoIbeacons) (*proclassic.MobileDeviceApplicationScopeLimitations, diag.Diagnostics) {
	var diags diag.Diagnostics
	l := &proclassic.MobileDeviceApplicationScopeLimitations{}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		l.NetworkSegments = &proclassic.MobileDeviceApplicationScopeLimitationsNetworkSegments{NetworkSegment: segs}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		l.Users = &proclassic.MobileDeviceApplicationScopeLimitationsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if userGroups != nil {
		l.UserGroups = &proclassic.MobileDeviceApplicationScopeLimitationsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when the caller's model declares `limitations`. A
	// scope PUT replaces the whole subtree, so an explicit empty element is the
	// clear gesture for a declared-empty category; undeclared (null) categories
	// are preserved upstream by the read-merge-write update, which hands this
	// builder a fully non-null merged model.
	return l, diags
}

func buildMobileAppScopeExclusions(ctx context.Context, m *scope.MobileScopeExclusionsModelNoIbeacons) (*proclassic.MobileDeviceApplicationScopeExclusions, diag.Diagnostics) {
	var diags diag.Diagnostics
	e := &proclassic.MobileDeviceApplicationScopeExclusions{}

	mds, d := scope.BuildIDSlice(ctx, m.MobileDeviceIDs, func(id int) proclassic.MobileDeviceApplicationScopeExclusionsMobileDevicesMobileDeviceItem {
		return proclassic.MobileDeviceApplicationScopeExclusionsMobileDevicesMobileDeviceItem{ID: &id}
	})
	diags.Append(d...)
	if mds != nil {
		e.MobileDevices = &proclassic.MobileDeviceApplicationScopeExclusionsMobileDevices{MobileDevice: mds}
	}

	mdgs, d := scope.BuildIDSlice(ctx, m.MobileDeviceGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if mdgs != nil {
		e.MobileDeviceGroups = &proclassic.MobileDeviceApplicationScopeExclusionsMobileDeviceGroups{MobileDeviceGroup: mdgs}
	}

	buildings, d := scope.BuildIDSlice(ctx, m.BuildingIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if buildings != nil {
		e.Buildings = &proclassic.MobileDeviceApplicationScopeExclusionsBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, m.DepartmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if departments != nil {
		e.Departments = &proclassic.MobileDeviceApplicationScopeExclusionsDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, m.UserIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUsers != nil {
		e.JssUsers = &proclassic.MobileDeviceApplicationScopeExclusionsJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, m.UserGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUserGroups != nil {
		e.JssUserGroups = &proclassic.MobileDeviceApplicationScopeExclusionsJssUserGroups{UserGroup: jssUserGroups}
	}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.MobileDeviceApplicationScopeExclusionsNetworkSegmentsNetworkSegmentItem {
		return proclassic.MobileDeviceApplicationScopeExclusionsNetworkSegmentsNetworkSegmentItem{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		e.NetworkSegments = &proclassic.MobileDeviceApplicationScopeExclusionsNetworkSegments{NetworkSegment: segs}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.MobileDeviceApplicationScopeExclusionsUsersUserItem {
		n := name
		return proclassic.MobileDeviceApplicationScopeExclusionsUsersUserItem{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		e.Users = &proclassic.MobileDeviceApplicationScopeExclusionsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if userGroups != nil {
		e.UserGroups = &proclassic.MobileDeviceApplicationScopeExclusionsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when declared — see buildMobileAppScopeLimitations.
	return e, diags
}

func buildMobileAppSelfService(m *MobileAppSelfServiceModel) *proclassic.MobileDeviceApplicationSelfService {
	ss := &proclassic.MobileDeviceApplicationSelfService{
		SelfServiceInstallButtonText:      helpers.OptionalStringPointer(m.InstallButtonText),
		SelfServiceAfterInstallButtonText: helpers.OptionalStringPointer(m.AfterInstallButtonText),
		SelfServiceDescription:            helpers.OptionalStringPointer(m.SelfServiceDescription),
		FeatureOnMainPage:                 helpers.OptionalBoolPointer(m.FeatureOnMainPage),
		Notification:                      buildMobileNotification(m.NotificationEnabled),
		NotificationSubject:               helpers.OptionalStringPointer(m.NotificationSubject),
		NotificationMessage:               helpers.OptionalStringPointer(m.NotificationMessage),
	}

	if m.SelfServiceIcon != nil {
		if id := helpers.StringIDPtr(m.SelfServiceIcon.ID); id != nil {
			ss.SelfServiceIcon = &proclassic.MobileDeviceApplicationSelfServiceSelfServiceIcon{ID: id}
		}
	}

	if len(m.SelfServiceCategories) > 0 {
		cats := make([]proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategoriesCategoryItem, 0, len(m.SelfServiceCategories))
		for _, c := range m.SelfServiceCategories {
			cats = append(cats, proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategoriesCategoryItem{
				ID:        helpers.StringIDPtr(c.ID),
				Name:      helpers.OptionalStringPointer(c.Name),
				DisplayIn: helpers.SelfServiceCategoryDisplayIn(c.DisplayIn),
			})
		}
		ss.SelfServiceCategories = &proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategories{Category: &cats}
	}

	return ss
}

func buildMobileAppVpp(m *MobileAppVppModel) *proclassic.MobileDeviceApplicationVpp {
	v := &proclassic.MobileDeviceApplicationVpp{
		AssignVppDeviceBasedLicenses: helpers.OptionalBoolPointer(m.AssignVppDeviceBasedLicenses),
		VppAdminAccountID:            helpers.StringIDPtr(m.VppAdminAccountID),
	}
	if v.AssignVppDeviceBasedLicenses == nil && v.VppAdminAccountID == nil {
		return nil
	}
	return v
}

func buildMobileAppAppConfiguration(m *MobileAppAppConfigurationModel) *proclassic.MobileDeviceApplicationAppConfiguration {
	prefs := helpers.OptionalStringPointer(m.Preferences)
	if prefs == nil {
		return nil
	}
	return &proclassic.MobileDeviceApplicationAppConfiguration{Preferences: prefs}
}
