// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

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
func assignResourceModel(ctx context.Context, state *ResourceModel, p *proclassic.MobileDeviceConfigurationProfile, includeUnmanaged bool) diag.Diagnostics {
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
		state.Scope = &scope.MobileScopeModel{}
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

func flattenGeneral(g *proclassic.MobileDeviceConfigurationProfileGeneral, state *GeneralModel) {
	if g == nil {
		return
	}
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.ReconcileOptionalStringPointer(g.Name, state.Name)
	state.Description = helpers.ReconcileOptionalStringPointer(g.Description, state.Description)
	// RedeployOnUpdate: the Classic API always returns "Newly Assigned" on read
	// regardless of what was PUT — write-only on the wire. Preserve the
	// user-authored value so a plan after `redeploy_on_update = "All"` does
	// not snap back to "Newly Assigned" on every refresh.
	if state.RedeployOnUpdate.IsNull() || state.RedeployOnUpdate.IsUnknown() {
		state.RedeployOnUpdate = helpers.StringPointerValueOrNull(g.RedeployOnUpdate)
	}
	if g.RedeployDaysBeforeCertificateExpires != nil {
		state.RedeployDaysBeforeCertificateExpires = types.Int64Value(int64(*g.RedeployDaysBeforeCertificateExpires))
	}
	state.UUID = helpers.StringPointerValueOrNull(g.UUID)
	// Self-healing payload: keep user-authored bytes when the server's
	// canonical form is semantically equivalent; only overwrite when genuine
	// out-of-band drift is detected.
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
	// Wire element is <deployment_method>; TF attribute is distribution_method (UI-canonical).
	state.DistributionMethod = helpers.ReconcileOptionalStringPointer(g.DeploymentMethod, state.DistributionMethod)

	if g.Level != nil {
		state.Level = types.StringValue(levelFromWireRead(*g.Level))
	} else if !helpers.IsConfiguredValue(state.Level) {
		state.Level = types.StringNull()
	}

	if g.Category != nil {
		state.CategoryID = helpers.StringValueFromIntPtr(g.Category.ID)
		state.CategoryName = helpers.DerivedRefName(g.Category.ID, g.Category.Name)
	}
	if g.Site != nil {
		state.SiteID = helpers.StringValueFromIntPtr(g.Site.ID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	}
}

// flattenScope refreshes the scope sub-blocks the caller already manages.
// When includeUnmanaged is set (import hydration, config generation, the
// and the Update merge base) every wire-present
// sub-block is first allocated so the from-scratch read hydrates the full
// scope rather than leaving unmanaged targets/limitations/exclusions null.
func flattenScope(ctx context.Context, s *proclassic.MobileDeviceConfigurationProfileScope, state *scope.MobileScopeModel, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &scope.MobileScopeTargetsModel{}
		}
		if state.Limitations == nil && s.Limitations != nil {
			state.Limitations = &scope.MobileScopeLimitationsModel{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &scope.MobileScopeExclusionsModel{}
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
		diags.Append(flattenScopeLimitations(ctx, s.Limitations, state.Limitations, includeUnmanaged)...)
	}
	if state.Exclusions != nil && s.Exclusions != nil {
		diags.Append(flattenScopeExclusions(ctx, s.Exclusions, state.Exclusions, includeUnmanaged)...)
	}
	return diags
}

func flattenScopeLimitations(ctx context.Context, l *proclassic.MobileDeviceConfigurationProfileScopeLimitations, state *scope.MobileScopeLimitationsModel, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if l == nil {
		return diags
	}
	state.NetworkSegmentIDs = scope.RefreshManagedSet(state.NetworkSegmentIDs, scope.FlattenIDNameSet(ctx, limitationsSegmentSlice(l.NetworkSegments)), includeUnmanaged)
	state.IbeaconIDs = scope.RefreshManagedSet(state.IbeaconIDs, scope.FlattenIDNameSet(ctx, limitationsIbeaconSlice(l.Ibeacons)), includeUnmanaged)
	state.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(state.DirectoryServiceOrLocalUserNames, scope.FlattenNameSet(ctx, limitationsUserSlice(l.Users)), includeUnmanaged)
	state.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(state.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, limitationsUserGroupSlice(l.UserGroups)), includeUnmanaged)
	return diags
}

func flattenScopeExclusions(ctx context.Context, e *proclassic.MobileDeviceConfigurationProfileScopeExclusions, state *scope.MobileScopeExclusionsModel, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if e == nil {
		return diags
	}
	state.MobileDeviceIDs = scope.RefreshManagedSet(state.MobileDeviceIDs, flattenExclMobileDeviceItemSet(ctx, e.MobileDevices), includeUnmanaged)
	state.MobileDeviceGroupIDs = scope.RefreshManagedSet(state.MobileDeviceGroupIDs, scope.FlattenIDNameSet(ctx, exclMobileDeviceGroupSlice(e.MobileDeviceGroups)), includeUnmanaged)
	state.BuildingIDs = scope.RefreshManagedSet(state.BuildingIDs, scope.FlattenIDNameSet(ctx, exclBuildingSlice(e.Buildings)), includeUnmanaged)
	state.DepartmentIDs = scope.RefreshManagedSet(state.DepartmentIDs, scope.FlattenIDNameSet(ctx, exclDepartmentSlice(e.Departments)), includeUnmanaged)
	state.UserIDs = scope.RefreshManagedSet(state.UserIDs, scope.FlattenIDNameSet(ctx, exclJssUserSlice(e.JssUsers)), includeUnmanaged)
	state.UserGroupIDs = scope.RefreshManagedSet(state.UserGroupIDs, scope.FlattenIDNameSet(ctx, exclJssUserGroupSlice(e.JssUserGroups)), includeUnmanaged)
	state.NetworkSegmentIDs = scope.RefreshManagedSet(state.NetworkSegmentIDs, flattenExclNetworkSegmentSet(ctx, e.NetworkSegments), includeUnmanaged)
	state.IbeaconIDs = scope.RefreshManagedSet(state.IbeaconIDs, scope.FlattenIDNameSet(ctx, exclIbeaconSlice(e.Ibeacons)), includeUnmanaged)
	state.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(state.DirectoryServiceOrLocalUserNames, flattenExclUsersNameSet(ctx, e.Users), includeUnmanaged)
	state.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(state.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, exclUserGroupSlice(e.UserGroups)), includeUnmanaged)
	return diags
}

// ---- scope sub-slice accessors -------------------------------------------------

func mobileDeviceGroupSlice(g *proclassic.MobileDeviceConfigurationProfileScopeMobileDeviceGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.MobileDeviceGroup
}

func buildingSlice(b *proclassic.MobileDeviceConfigurationProfileScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func departmentSlice(d *proclassic.MobileDeviceConfigurationProfileScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func jssUserSlice(u *proclassic.MobileDeviceConfigurationProfileScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func jssUserGroupSlice(u *proclassic.MobileDeviceConfigurationProfileScopeJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func limitationsSegmentSlice(s *proclassic.MobileDeviceConfigurationProfileScopeLimitationsNetworkSegments) *[]proclassic.IDName {
	if s == nil {
		return nil
	}
	return s.NetworkSegment
}

func limitationsIbeaconSlice(i *proclassic.MobileDeviceConfigurationProfileScopeLimitationsIbeacons) *[]proclassic.IDName {
	if i == nil {
		return nil
	}
	return i.Ibeacon
}

func limitationsUserSlice(u *proclassic.MobileDeviceConfigurationProfileScopeLimitationsUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func limitationsUserGroupSlice(u *proclassic.MobileDeviceConfigurationProfileScopeLimitationsUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclMobileDeviceGroupSlice(g *proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDeviceGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.MobileDeviceGroup
}

func exclBuildingSlice(b *proclassic.MobileDeviceConfigurationProfileScopeExclusionsBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func exclDepartmentSlice(d *proclassic.MobileDeviceConfigurationProfileScopeExclusionsDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func exclJssUserSlice(u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func exclJssUserGroupSlice(u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.UserGroup
}

func exclIbeaconSlice(i *proclassic.MobileDeviceConfigurationProfileScopeExclusionsIbeacons) *[]proclassic.IDName {
	if i == nil {
		return nil
	}
	return i.Ibeacon
}

func exclUserGroupSlice(u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsUserGroups) *[]proclassic.IDName {
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

func flattenMobileDeviceItemSet(ctx context.Context, m *proclassic.MobileDeviceConfigurationProfileScopeMobileDevices) types.Set {
	if m == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, m.MobileDevice, func(i proclassic.MobileDeviceConfigurationProfileScopeMobileDevicesMobileDeviceItem) *int {
		return i.ID
	})
	return out
}

func flattenExclMobileDeviceItemSet(ctx context.Context, m *proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevices) types.Set {
	if m == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, m.MobileDevice, func(i proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDevicesMobileDeviceItem) *int {
		return i.ID
	})
	return out
}

func flattenExclNetworkSegmentSet(ctx context.Context, n *proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegments) types.Set {
	if n == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenIDSlice(ctx, n.NetworkSegment, func(i proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int {
		return i.ID
	})
	return out
}

func flattenExclUsersNameSet(ctx context.Context, u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsers) types.Set {
	if u == nil {
		return scope.EmptyStringSet()
	}
	out, _ := scope.FlattenNameSlice(ctx, u.User, func(i proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem) *string { return i.Name })
	return out
}

// flattenSelfService refreshes the Self Service block the caller already
// manages. includeUnmanaged carries the same meaning as in assignResourceModel.
func flattenSelfService(ss *proclassic.MobileDeviceConfigurationProfileSelfService, state *SelfServiceModel, includeUnmanaged bool) {
	if ss == nil {
		return
	}
	state.FeatureOnMainPage = helpers.ReconcileOptionalBoolPointer(ss.FeatureOnMainPage, state.FeatureOnMainPage)

	if ss.Security != nil {
		state.RemovalDisallowed = helpers.ReconcileOptionalStringPointer(ss.Security.RemovalDisallowed, state.RemovalDisallowed)
		state.AuthorizationPassword = helpers.PreserveStringWhenWireEmpty(ss.Security.Password, state.AuthorizationPassword)
	}

	// categories is ownership-gated like every sibling above it. It used not to
	// be: the hydration tested only what the wire carried, so dropping
	// categories from a self_service block you otherwise keep failed the apply
	// with "Provider produced inconsistent result after apply" — the plan said
	// null and Read handed back a populated list. The server retains them
	// correctly; this was ours (issue #392, which names this resource alongside
	// the macOS profile).
	//
	// Optional-only, so a nil slice means the configuration does not manage the
	// attribute and an empty non-nil one means it declares `[]`. Managed, the
	// wire is authoritative including when it is empty. Unmanaged, only the
	// first hydration populates it — import, config generation and the Update
	// merge base — and even then only if the server has any, so an ordinary
	// refresh never fabricates a list the practitioner never wrote.
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
func flattenSelfServiceCategories(c *proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories) []SelfServiceCategoryItem {
	if c == nil || c.Category == nil {
		return []SelfServiceCategoryItem{}
	}
	cats := *c.Category
	items := make([]SelfServiceCategoryItem, 0, len(cats))
	for _, cat := range cats {
		items = append(items, SelfServiceCategoryItem{
			ID:   idPointerToString(cat.ID),
			Name: helpers.StringPointerValueOrNull(cat.Name),
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
