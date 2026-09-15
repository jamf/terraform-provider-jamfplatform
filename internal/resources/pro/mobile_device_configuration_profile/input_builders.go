// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func buildInput(ctx context.Context, plan ResourceModel, existingUUID string) (*proclassic.MobileDeviceConfigurationProfile, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &proclassic.MobileDeviceConfigurationProfile{}

	if plan.General != nil {
		general, _, d := buildGeneral(plan.General, existingUUID)
		diags.Append(d...)
		out.General = general
	}
	if plan.Scope != nil {
		s, d := buildScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}
	if plan.SelfService != nil {
		ss, d := buildSelfService(plan.SelfService)
		diags.Append(d...)
		out.SelfService = ss
	}
	return out, diags
}

func buildGeneral(m *GeneralModel, existingUUID string) (*proclassic.MobileDeviceConfigurationProfileGeneral, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	g := &proclassic.MobileDeviceConfigurationProfileGeneral{
		Name:             helpers.OptionalStringPointer(m.Name),
		Description:      helpers.OptionalStringPointer(m.Description),
		RedeployOnUpdate: helpers.OptionalStringPointer(m.RedeployOnUpdate),
	}

	if !m.RedeployDaysBeforeCertificateExpires.IsNull() && !m.RedeployDaysBeforeCertificateExpires.IsUnknown() {
		g.RedeployDaysBeforeCertificateExpires = helpers.OptionalInt64Pointer(m.RedeployDaysBeforeCertificateExpires)
	}

	if v := m.Level.ValueString(); !m.Level.IsNull() && !m.Level.IsUnknown() {
		wire := levelToWireWrite(v)
		g.Level = &wire
	}
	if v := m.DistributionMethod.ValueString(); !m.DistributionMethod.IsNull() && !m.DistributionMethod.IsUnknown() {
		dm := v
		g.DeploymentMethod = &dm
	}

	if id := helpers.StringIDPtr(m.CategoryID); id != nil {
		g.Category = &proclassic.CategoryObject{ID: id}
	}
	if id := helpers.StringIDPtr(m.SiteID); id != nil {
		g.Site = &proclassic.SiteObject{ID: id}
	}

	if v := m.Payloads.ValueString(); !m.Payloads.IsNull() && !m.Payloads.IsUnknown() && v != "" {
		raw := []byte(v)
		prepared, err := payloadhelpers.PrepareWirePayload(raw, existingUUID, existingUUID)
		if err != nil {
			diags.AddError("Failed to inject server-canonical PayloadUUID/PayloadIdentifier into update payload", helpers.APIErrorDetail(err))
			return nil, nil, diags
		}
		s := proclassic.PayloadsXMLText(prepared)
		g.Payloads = &s
		return g, prepared, diags
	}
	return g, nil, diags
}

func buildScope(ctx context.Context, m *scope.MobileScopeModel) (*proclassic.MobileDeviceConfigurationProfileScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.MobileDeviceConfigurationProfileScope{
		AllMobileDevices: helpers.OptionalBoolPointer(t.AllMobileDevices),
		AllJssUsers:      helpers.OptionalBoolPointer(t.AllJssUsers),
	}

	mds, d := scope.BuildIDSlice(ctx, t.MobileDeviceIDs, func(id int) proclassic.MobileDeviceConfigurationProfileScopeMobileDevicesMobileDeviceItem {
		return proclassic.MobileDeviceConfigurationProfileScopeMobileDevicesMobileDeviceItem{ID: &id}
	})
	diags.Append(d...)
	if mds != nil {
		s.MobileDevices = &proclassic.MobileDeviceConfigurationProfileScopeMobileDevices{MobileDevice: mds}
	}

	mdgs, d := scope.BuildIDSlice(ctx, t.MobileDeviceGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if mdgs != nil {
		s.MobileDeviceGroups = &proclassic.MobileDeviceConfigurationProfileScopeMobileDeviceGroups{MobileDeviceGroup: mdgs}
	}

	buildings, d := scope.BuildIDSlice(ctx, t.BuildingIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if buildings != nil {
		s.Buildings = &proclassic.MobileDeviceConfigurationProfileScopeBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, t.DepartmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if departments != nil {
		s.Departments = &proclassic.MobileDeviceConfigurationProfileScopeDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, t.UserIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUsers != nil {
		s.JssUsers = &proclassic.MobileDeviceConfigurationProfileScopeJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, t.UserGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUserGroups != nil {
		s.JssUserGroups = &proclassic.MobileDeviceConfigurationProfileScopeJssUserGroups{UserGroup: jssUserGroups}
	}

	if m.Limitations != nil {
		l, ld := buildScopeLimitations(ctx, m.Limitations)
		diags.Append(ld...)
		s.Limitations = l
	}
	if m.Exclusions != nil {
		e, ed := buildScopeExclusions(ctx, m.Exclusions)
		diags.Append(ed...)
		s.Exclusions = e
	}

	if s.AllMobileDevices == nil && s.AllJssUsers == nil && s.MobileDevices == nil &&
		s.MobileDeviceGroups == nil && s.Buildings == nil && s.Departments == nil &&
		s.JssUsers == nil && s.JssUserGroups == nil &&
		s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

func buildScopeLimitations(ctx context.Context, m *scope.MobileScopeLimitationsModel) (*proclassic.MobileDeviceConfigurationProfileScopeLimitations, diag.Diagnostics) {
	var diags diag.Diagnostics
	l := &proclassic.MobileDeviceConfigurationProfileScopeLimitations{}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		l.NetworkSegments = &proclassic.MobileDeviceConfigurationProfileScopeLimitationsNetworkSegments{NetworkSegment: segs}
	}

	ibeacons, d := scope.BuildIDSlice(ctx, m.IbeaconIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if ibeacons != nil {
		l.Ibeacons = &proclassic.MobileDeviceConfigurationProfileScopeLimitationsIbeacons{Ibeacon: ibeacons}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		l.Users = &proclassic.MobileDeviceConfigurationProfileScopeLimitationsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if userGroups != nil {
		l.UserGroups = &proclassic.MobileDeviceConfigurationProfileScopeLimitationsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when the user declared `limitations` (the caller's
	// gate). The classic /mobiledeviceconfigurationprofiles endpoint MERGES an
	// omitted sub-block (wire-probed), so collapsing an all-empty block to nil
	// would retain the server's existing members. An empty
	// <limitations></limitations> clears every category, which is what `[]` /
	// omission means.
	return l, diags
}

func buildScopeExclusions(ctx context.Context, m *scope.MobileScopeExclusionsModel) (*proclassic.MobileDeviceConfigurationProfileScopeExclusions, diag.Diagnostics) {
	var diags diag.Diagnostics
	e := &proclassic.MobileDeviceConfigurationProfileScopeExclusions{}

	mds, d := scope.BuildIDSlice(ctx, m.MobileDeviceIDs, func(id int) proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevicesMobileDeviceItem {
		return proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevicesMobileDeviceItem{ID: &id}
	})
	diags.Append(d...)
	if mds != nil {
		e.MobileDevices = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevices{MobileDevice: mds}
	}

	mdgs, d := scope.BuildIDSlice(ctx, m.MobileDeviceGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if mdgs != nil {
		e.MobileDeviceGroups = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDeviceGroups{MobileDeviceGroup: mdgs}
	}

	buildings, d := scope.BuildIDSlice(ctx, m.BuildingIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if buildings != nil {
		e.Buildings = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, m.DepartmentIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if departments != nil {
		e.Departments = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsDepartments{Department: departments}
	}

	jssUsers, d := scope.BuildIDSlice(ctx, m.UserIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUsers != nil {
		e.JssUsers = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, m.UserGroupIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if jssUserGroups != nil {
		e.JssUserGroups = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsJssUserGroups{UserGroup: jssUserGroups}
	}

	segs, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem {
		return proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem{ID: &id}
	})
	diags.Append(d...)
	if segs != nil {
		e.NetworkSegments = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegments{NetworkSegment: segs}
	}

	ibeacons, d := scope.BuildIDSlice(ctx, m.IbeaconIDs, func(id int) proclassic.IDName {
		return proclassic.IDName{ID: &id}
	})
	diags.Append(d...)
	if ibeacons != nil {
		e.Ibeacons = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsIbeacons{Ibeacon: ibeacons}
	}

	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem {
		n := name
		return proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		e.Users = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsers{User: users}
	}

	userGroups, d := scope.BuildNameSlice(ctx, m.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if userGroups != nil {
		e.UserGroups = &proclassic.MobileDeviceConfigurationProfileScopeExclusionsUserGroups{UserGroup: userGroups}
	}

	// Always emit the block when declared — see buildScopeLimitations.
	return e, diags
}

// buildSelfService maps the Self Service block onto the classic payload.
//
// It never emits <self_service_description>, and there is no attribute for one:
// the field is a write-alias for <description>, so sending it overwrote a value
// the practitioner had set under general, an empty one erased that value, and a
// read returned an empty element every time. Probed across eighteen create and
// update combinations against Jamf Pro 11.31.1 on 2026-09-07; deployment_method
// made no difference and update matched create. Issue #393 has the tables, and
// TestBuildSelfService_NeverSendsASelfServiceDescription pins the omission. The
// admin UI does offer the two descriptions separately and the macOS profile
// keeps them independent, so this is a Jamf Pro defect: if it is fixed, restore
// the attribute rather than reviving this field on its own.
//
// Every <category> it emits carries display_in=true, because that is what makes
// Jamf Pro store the category at all — see helpers.SelfServiceCategoryDisplayIn
// for the wire law, which is identical on all six classic resources carrying
// this block. The value is unconditional here because this resource exposes no
// `display_in` attribute for the practitioner to set: alone among the six, the
// mobile profile's GET echoes only <id> and <name>, so a configured value could
// never be read back or drift-checked, and the only value other than true
// deletes the category, which listing it cannot have meant.
func buildSelfService(m *SelfServiceModel) (*proclassic.MobileDeviceConfigurationProfileSelfService, diag.Diagnostics) {
	var diags diag.Diagnostics
	ss := &proclassic.MobileDeviceConfigurationProfileSelfService{
		FeatureOnMainPage: helpers.OptionalBoolPointer(m.FeatureOnMainPage),
	}

	hasDisallowed := !m.RemovalDisallowed.IsNull() && !m.RemovalDisallowed.IsUnknown() && m.RemovalDisallowed.ValueString() != ""
	hasPassword := !m.AuthorizationPassword.IsNull() && !m.AuthorizationPassword.IsUnknown() && m.AuthorizationPassword.ValueString() != ""
	if hasDisallowed || hasPassword {
		sec := &proclassic.MobileDeviceConfigurationProfileSelfServiceSecurity{}
		if hasDisallowed {
			v := m.RemovalDisallowed.ValueString()
			sec.RemovalDisallowed = &v
		}
		if hasPassword {
			v := m.AuthorizationPassword.ValueString()
			sec.Password = &v
		}
		ss.Security = sec
	}

	if len(m.Categories) > 0 {
		items := make([]proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem, 0, len(m.Categories))
		for _, c := range m.Categories {
			item := proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem{
				DisplayIn: helpers.SelfServiceCategoryDisplayIn(types.BoolNull()),
			}
			if id := helpers.StringIDPtr(c.ID); id != nil {
				item.ID = id
			}
			items = append(items, item)
		}
		ss.SelfServiceCategories = &proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories{
			Category: &items,
		}
	}

	return ss, diags
}
