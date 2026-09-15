// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignMobileAppResourceModel populates a resource model from the SDK
// MobileDeviceApplication response. general is always refreshed (required block).
// The optional sections (scope / self_service / vpp / app_configuration) are only
// refreshed when the caller (plan or current state) already manages them: the
// classic server echoes every section on GET with default values, so populating
// an unmanaged section would violate the framework's "produced inconsistent
// result after apply" check. See feedback_server_derived_echo_attrs at block
// granularity.
//
// includeUnmanaged inverts those section gates for the list resource's
// config-generation path (terraform query -generate-config-out): there is no
// plan to stay consistent with, so every wire-present optional section is
// allocated and hydrated, yielding a complete exported config rather than a
// general-only one. CRUD callers pass false. The mobile-app flatteners use the
// wire-authoritative reads (helpers.ReconcileOptionalStringPointer /
// helpers.BoolPointerValueOrNull), which adopt the wire value whatever state
// holds, so allocating an empty section is sufficient for it to fully hydrate.
func assignMobileAppResourceModel(ctx context.Context, state *MobileAppResourceModel, a *proclassic.MobileDeviceApplication, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if a == nil {
		return diags
	}

	if id := extractMobileAppID(a); id != "" {
		state.ID = types.StringValue(id)
	}

	if state.General == nil {
		state.General = &MobileAppGeneralModel{}
	}
	flattenMobileAppGeneral(a.General, state.General)

	if includeUnmanaged && state.Scope == nil && a.Scope != nil {
		state.Scope = &scope.MobileScopeModelNoIbeacons{}
	}
	if state.Scope != nil && a.Scope != nil {
		flattenMobileAppScope(ctx, a.Scope, state.Scope, includeUnmanaged)
	}
	if includeUnmanaged && state.SelfService == nil && a.SelfService != nil {
		state.SelfService = &MobileAppSelfServiceModel{}
	}
	if state.SelfService != nil && a.SelfService != nil {
		flattenMobileAppSelfService(a.SelfService, state.SelfService, includeUnmanaged)
	}
	if includeUnmanaged && state.Vpp == nil && a.Vpp != nil {
		state.Vpp = &MobileAppVppModel{}
	}
	if state.Vpp != nil && a.Vpp != nil {
		flattenMobileAppVpp(a.Vpp, state.Vpp)
	}
	if includeUnmanaged && state.AppConfiguration == nil && a.AppConfiguration != nil {
		state.AppConfiguration = &MobileAppAppConfigurationModel{}
	}
	if state.AppConfiguration != nil && a.AppConfiguration != nil {
		// Preserve the configured value when the server differs only by newline
		// style / a stripped trailing newline (server strips it on round-trip);
		// otherwise reflect the server value as drift.
		state.AppConfiguration.Preferences = preservePreferences(a.AppConfiguration.Preferences, state.AppConfiguration.Preferences)
	}

	return diags
}

// flattenMobileAppGeneral maps the wire <general> block onto the model.
// host_externally keeps a sticky read: while external_url is set the server
// forces it true, and a PUT sending false, in isolation, left the GET reading
// true. os_type has its own asymmetric rule, below. Everything else is echoed
// faithfully and reads from the wire. Wire-probed against Jamf Pro 11.31.1 on
// 2026-09-06; see issue #387.
//
// description and deployment_type are read-only mirrors of a sibling this
// resource manages, and read straight from the wire like the rest.
// (display_name and internal_app are not modeled — see MobileAppGeneralModel.)
// Wire-proven on 2026-09-07:
//
//   - description mirrors self_service.self_service_description. A create
//     sending only the Self Service description made BOTH echo it, a write to
//     general.description alone was accepted and discarded, and a later write
//     to the Self Service description moved both.
//   - deployment_type mirrors general.deploy_automatically: false echoes "Make
//     Available in Self Service", true echoes "Install Automatically/Prompt
//     Users to Install". A write to deployment_type itself is discarded on
//     create and on update, so it never worked — the previous sticky read hid
//     that.
//
// Neither carries UseStateForUnknown, for the reason in mirrorString: pinning a
// mirror's prior value makes the plan disagree with the apply.
func flattenMobileAppGeneral(g *proclassic.MobileDeviceApplicationGeneral, state *MobileAppGeneralModel) {
	if g == nil {
		return
	}
	state.ID = helpers.StringValueFromIntPtr(g.ID)
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.Version = helpers.StringPointerValueOrNull(g.Version)
	state.BundleID = helpers.StringPointerValueOrNull(g.BundleID)
	// os_type echo is asymmetric (wire-probed): a POST never persists/echoes it,
	// and a non-internal app never carries it, but once set via a PUT to an
	// in-house app it is stored and echoed on every GET. Reflect the echo when
	// present (authoritative; surfaces external drift) and fall back to the
	// configured/prior value when absent so a Required attribute is never nulled
	// (which would trip "inconsistent result after apply"). Create issues a
	// follow-up PUT precisely so the server persists it and this echo is present.
	state.OsType = helpers.WireWhenPresentString(g.OsType, state.OsType)

	state.Description = helpers.StringPointerValueOrNull(g.Description)
	state.DeploymentType = helpers.StringPointerValueOrNull(g.DeploymentType)

	// Optional+Computed echoes.
	state.IsFree = helpers.BoolPointerValueOrNull(g.Free)
	state.ExternalURL = helpers.ReconcileOptionalStringPointer(g.ExternalURL, state.ExternalURL)
	state.ItunesStoreURL = helpers.ReconcileOptionalStringPointer(g.ItunesStoreURL, state.ItunesStoreURL)
	state.ItunesCountryRegion = helpers.ReconcileOptionalStringPointer(g.ItunesCountryRegion, state.ItunesCountryRegion)
	state.ItunesSyncTime = helpers.Int64FromIntPtr(g.ItunesSyncTime)
	state.MakeAvailableAfterInstall = helpers.BoolPointerValueOrNull(g.MakeAvailableAfterInstall)
	state.KeepDescriptionAndIconUpToDate = helpers.BoolPointerValueOrNull(g.KeepDescriptionAndIconUpToDate)
	state.KeepAppUpdatedOnDevices = helpers.BoolPointerValueOrNull(g.KeepAppUpdatedOnDevices)
	state.DeployAsManagedApp = helpers.BoolPointerValueOrNull(g.DeployAsManagedApp)
	state.TakeOverManagement = helpers.BoolPointerValueOrNull(g.TakeOverManagement)
	state.DeployAutomatically = helpers.BoolPointerValueOrNull(g.DeployAutomatically)
	state.RemoveAppWhenMDMProfileIsRemoved = helpers.BoolPointerValueOrNull(g.RemoveAppWhenMDMProfileIsRemoved)
	state.PreventBackupOfAppData = helpers.BoolPointerValueOrNull(g.PreventBackupOfAppData)
	state.AllowUserToDelete = helpers.BoolPointerValueOrNull(g.AllowUserToDelete)
	state.RequireNetworkTethered = helpers.BoolPointerValueOrNull(g.RequireNetworkTethered)
	state.HostExternally = helpers.StickyIgnoringDriftBool(g.HostExternally, state.HostExternally)

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

// flattenMobileAppScope refreshes the scope sub-blocks the caller already
// manages. When includeUnmanaged is set (import hydration, config generation,
// and the Update merge base) every
// wire-present sub-block is first allocated so the from-scratch read hydrates
// the full scope rather than leaving unmanaged targets/limitations/exclusions
// null.
func flattenMobileAppScope(ctx context.Context, s *proclassic.MobileDeviceApplicationScope, state *scope.MobileScopeModelNoIbeacons, includeUnmanaged bool) {
	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &scope.MobileScopeTargetsModel{}
		}
		if state.Limitations == nil && s.Limitations != nil {
			state.Limitations = &scope.MobileScopeLimitationsModelNoIbeacons{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &scope.MobileScopeExclusionsModelNoIbeacons{}
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
		t.AllMobileDevices = scope.RefreshManagedBool(t.AllMobileDevices, s.AllMobileDevices, includeUnmanaged)
		t.AllJssUsers = scope.RefreshManagedBool(t.AllJssUsers, s.AllJssUsers, includeUnmanaged)

		t.MobileDeviceIDs = scope.RefreshManagedSet(t.MobileDeviceIDs, flattenMobileDeviceItemSet(ctx, s.MobileDevices), includeUnmanaged)
		t.MobileDeviceGroupIDs = scope.RefreshManagedSet(t.MobileDeviceGroupIDs, scope.FlattenIDNameSet(ctx, mobileDeviceGroupSlice(s.MobileDeviceGroups)), includeUnmanaged)
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
		x.MobileDeviceIDs = scope.RefreshManagedSet(x.MobileDeviceIDs, flattenExclMobileDeviceItemSet(ctx, se.MobileDevices), includeUnmanaged)
		x.MobileDeviceGroupIDs = scope.RefreshManagedSet(x.MobileDeviceGroupIDs, scope.FlattenIDNameSet(ctx, exclMobileDeviceGroupSlice(se.MobileDeviceGroups)), includeUnmanaged)
		x.BuildingIDs = scope.RefreshManagedSet(x.BuildingIDs, scope.FlattenIDNameSet(ctx, exclBuildingSlice(se.Buildings)), includeUnmanaged)
		x.DepartmentIDs = scope.RefreshManagedSet(x.DepartmentIDs, scope.FlattenIDNameSet(ctx, exclDepartmentSlice(se.Departments)), includeUnmanaged)
		x.UserIDs = scope.RefreshManagedSet(x.UserIDs, scope.FlattenIDNameSet(ctx, exclJssUserSlice(se.JssUsers)), includeUnmanaged)
		x.UserGroupIDs = scope.RefreshManagedSet(x.UserGroupIDs, scope.FlattenIDNameSet(ctx, exclJssUserGroupSlice(se.JssUserGroups)), includeUnmanaged)
		x.NetworkSegmentIDs = scope.RefreshManagedSet(x.NetworkSegmentIDs, flattenExclNetworkSegmentSet(ctx, se.NetworkSegments), includeUnmanaged)
		x.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(x.DirectoryServiceOrLocalUserNames, flattenExclUsersNameSet(ctx, se.Users), includeUnmanaged)
		x.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(x.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, exclUserGroupSlice(se.UserGroups)), includeUnmanaged)
	}
}

// flattenMobileAppSelfService maps the wire <self_service> block onto the
// model. Four fields are echoed conditionally and read through
// helpers.WireWhenPresent*, which adopts the wire whenever it carries a value
// and keeps state when it does not.
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
// after_install_button_text has a gate of its own, and a nearer one:
// general.make_available_after_install. With that toggle off the label is
// accepted, silently discarded and omitted from the GET — probed in isolation,
// the same POST round-tripping it with the toggle on and dropping it with the
// toggle off. Because the read deliberately keeps the configured value rather
// than nulling it, the combination is rejected at plan time instead; see
// requiresMakeAvailableAfterInstall in validators.go.
//
// The rest of the block, the icon included, is echoed unconditionally and reads
// from the wire. Wire-probed against Jamf Pro 11.31.1 on 2026-09-06; see issue
// #387.
//
// self_service_icon and self_service_categories are ownership-gated like the
// blocks above them, and includeUnmanaged releases both on first hydration —
// import, config generation and the Update merge base. Without it the caller
// allocating an empty self_service block was not enough: the zero-value struct
// carries a nil icon pointer and a nil category slice, so neither ever
// populated, on that Read or any later one (issue #391). categories is
// Optional-only, so a nil slice means the configuration does not manage the
// attribute and an empty non-nil one means it declares `[]`; unmanaged, it is
// adopted only when the server has any, so an ordinary refresh never fabricates
// a list the practitioner never wrote. This mirrors
// mobile_device_configuration_profile, which took the same fix under issue
// #392.
func flattenMobileAppSelfService(ss *proclassic.MobileDeviceApplicationSelfService, state *MobileAppSelfServiceModel, includeUnmanaged bool) {
	state.InstallButtonText = helpers.ReconcileOptionalStringPointer(ss.SelfServiceInstallButtonText, state.InstallButtonText)
	state.AfterInstallButtonText = helpers.WireWhenPresentString(ss.SelfServiceAfterInstallButtonText, state.AfterInstallButtonText)
	state.SelfServiceDescription = helpers.PreserveStringWhenWireEmpty(ss.SelfServiceDescription, state.SelfServiceDescription)
	state.FeatureOnMainPage = helpers.BoolPointerValueOrNull(ss.FeatureOnMainPage)

	var apiEnabled *bool
	if ss.Notification != nil {
		apiEnabled = ss.Notification.Enabled
	}
	state.NotificationEnabled = helpers.WireWhenPresentBool(apiEnabled, state.NotificationEnabled)
	state.NotificationSubject = helpers.WireWhenPresentString(ss.NotificationSubject, state.NotificationSubject)
	state.NotificationMessage = helpers.WireWhenPresentString(ss.NotificationMessage, state.NotificationMessage)

	if includeUnmanaged && state.SelfServiceIcon == nil && ss.SelfServiceIcon != nil {
		state.SelfServiceIcon = &MobileAppSelfServiceIconModel{}
	}
	if state.SelfServiceIcon != nil && ss.SelfServiceIcon != nil {
		state.SelfServiceIcon.ID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(ss.SelfServiceIcon.ID), state.SelfServiceIcon.ID)
		state.SelfServiceIcon.URI = helpers.ReconcileOptionalStringPointer(ss.SelfServiceIcon.URI, state.SelfServiceIcon.URI)
	}

	if ss.SelfServiceCategories != nil && ss.SelfServiceCategories.Category != nil {
		cats := *ss.SelfServiceCategories.Category
		if state.SelfServiceCategories != nil || (includeUnmanaged && len(cats) > 0) {
			flattenMobileAppSelfServiceCategories(cats, state)
		}
	}
}

// flattenMobileAppSelfServiceCategories refreshes the managed category set,
// matching server items to existing state items by ID so caller-authored values
// (display_in) stick across refreshes. Server items not in state are appended;
// the set is keyed by category ID.
func flattenMobileAppSelfServiceCategories(api []proclassic.MobileDeviceApplicationSelfServiceSelfServiceCategoriesCategoryItem, state *MobileAppSelfServiceModel) {
	byID := make(map[string]MobileAppSelfServiceCategoryModel, len(state.SelfServiceCategories))
	for _, c := range state.SelfServiceCategories {
		byID[c.ID.ValueString()] = c
	}

	out := make([]MobileAppSelfServiceCategoryModel, 0, len(api))
	for _, c := range api {
		idStr := ""
		if s := helpers.StringFromIntPtr(c.ID); s != nil {
			idStr = *s
		}
		current := byID[idStr]
		out = append(out, MobileAppSelfServiceCategoryModel{
			ID:        types.StringValue(idStr),
			Name:      helpers.ReconcileOptionalStringPointer(c.Name, current.Name),
			DisplayIn: helpers.BoolPointerValueOrNull(c.DisplayIn),
		})
	}
	state.SelfServiceCategories = out
}

func flattenMobileAppVpp(v *proclassic.MobileDeviceApplicationVpp, state *MobileAppVppModel) {
	state.AssignVppDeviceBasedLicenses = helpers.BoolPointerValueOrNull(v.AssignVppDeviceBasedLicenses)
	state.VppAdminAccountID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(v.VppAdminAccountID), state.VppAdminAccountID)
}

// ---- scope sub-slice accessors -------------------------------------------------

func mobileDeviceGroupSlice(g *proclassic.MobileDeviceApplicationScopeMobileDeviceGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.MobileDeviceGroup
}

func buildingSlice(b *proclassic.MobileDeviceApplicationScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func departmentSlice(d *proclassic.MobileDeviceApplicationScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func jssUserSlice(u *proclassic.MobileDeviceApplicationScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func jssUserGroupSlice(u *proclassic.MobileDeviceApplicationScopeJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func limitationsSegmentSlice(s *proclassic.MobileDeviceApplicationScopeLimitationsNetworkSegments) *[]proclassic.IDName {
	if s == nil {
		return nil
	}
	return s.NetworkSegment
}

func limitationsUserSlice(u *proclassic.MobileDeviceApplicationScopeLimitationsUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func limitationsUserGroupSlice(u *proclassic.MobileDeviceApplicationScopeLimitationsUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclMobileDeviceGroupSlice(g *proclassic.MobileDeviceApplicationScopeExclusionsMobileDeviceGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.MobileDeviceGroup
}

func exclBuildingSlice(b *proclassic.MobileDeviceApplicationScopeExclusionsBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func exclDepartmentSlice(d *proclassic.MobileDeviceApplicationScopeExclusionsDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func exclJssUserSlice(u *proclassic.MobileDeviceApplicationScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func exclJssUserGroupSlice(u *proclassic.MobileDeviceApplicationScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclUserGroupSlice(u *proclassic.MobileDeviceApplicationScopeExclusionsUserGroups) *[]proclassic.IDName {
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

func flattenMobileDeviceItemSet(ctx context.Context, m *proclassic.MobileDeviceApplicationScopeMobileDevices) types.Set {
	if m == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, m.MobileDevice, func(i proclassic.MobileDeviceApplicationScopeMobileDevicesMobileDeviceItem) *int { return i.ID })
	return out
}

func flattenExclMobileDeviceItemSet(ctx context.Context, m *proclassic.MobileDeviceApplicationScopeExclusionsMobileDevices) types.Set {
	if m == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, m.MobileDevice, func(i proclassic.MobileDeviceApplicationScopeExclusionsMobileDevicesMobileDeviceItem) *int {
		return i.ID
	})
	return out
}

func flattenExclNetworkSegmentSet(ctx context.Context, n *proclassic.MobileDeviceApplicationScopeExclusionsNetworkSegments) types.Set {
	if n == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, n.NetworkSegment, func(i proclassic.MobileDeviceApplicationScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int {
		return i.ID
	})
	return out
}

func flattenExclUsersNameSet(ctx context.Context, u *proclassic.MobileDeviceApplicationScopeExclusionsUsers) types.Set {
	if u == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenNameSlice(ctx, u.User, func(i proclassic.MobileDeviceApplicationScopeExclusionsUsersUserItem) *string { return i.Name })
	return out
}
