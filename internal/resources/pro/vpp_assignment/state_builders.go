// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignVPPAssignmentResourceModel refreshes a resource model from a GET. General
// scalars are always refreshed; each content Set is refreshed only when the
// caller already manages it (state Set non-null) so the server's always-returned
// collections don't fabricate management of a collection the user omitted. The
// optional scope block is refreshed only when state.Scope is non-nil (mirrors the
// content opt-out + the vpp_invitation precedent). Item names are discarded
// (only adam_id round-trips).
//
// includeUnmanaged inverts the content-set and scope gates for the
// hydrate-everything paths — the list resource's config generation (terraform
// query -generate-config-out), import, and the scope merge base built by
// Update: there is no plan to stay consistent with, so every wire-present
// content set and every scope category are hydrated from the server. CRUD
// refresh callers pass false; within a managed scope block the per-category
// gate (scope.RefreshManagedSet) then keeps undeclared categories null.
func assignVPPAssignmentResourceModel(ctx context.Context, state *VPPAssignmentResourceModel, api *proclassic.VppAssignment, includeUnmanaged bool) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.VPPAdminAccountID = intStringOrNull(g.VppAdminAccountID)
	state.VPPAdminAccountName = helpers.StringPointerValueOrNull(g.VppAdminAccountName)

	// Content: refresh only managed (non-null) sets. The flatten returns a known
	// (possibly empty) Set so a managed-but-now-empty collection reconciles to []
	// rather than null (these are plain Optional attrs — a null after an empty
	// config is a "provider produced inconsistent result" error). Config
	// generation hydrates all three regardless of prior management.
	if includeUnmanaged || !state.IosAppAdamIDs.IsNull() {
		state.IosAppAdamIDs = flattenAdamSet(ctx, iosAppAdamIDs(api.IosApps))
	}
	if includeUnmanaged || !state.MacAppAdamIDs.IsNull() {
		state.MacAppAdamIDs = flattenAdamSet(ctx, macAppAdamIDs(api.MacApps))
	}
	if includeUnmanaged || !state.EbookAdamIDs.IsNull() {
		state.EbookAdamIDs = flattenAdamSet(ctx, ebookAdamIDs(api.Ebooks))
	}

	if includeUnmanaged && state.Scope == nil && api.Scope != nil {
		state.Scope = &scope.UserScopeModel{}
	}
	if state.Scope != nil && api.Scope != nil {
		flattenScope(ctx, api.Scope, state.Scope, includeUnmanaged)
	}
}

// assignVPPAssignmentDataSourceModel populates a DS model from a GET. The DS
// always surfaces scope + the three content lists (read-only lookup).
func assignVPPAssignmentDataSourceModel(ctx context.Context, state *VPPAssignmentDataSourceModel, api *proclassic.VppAssignment) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.VPPAdminAccountID = intStringOrNull(g.VppAdminAccountID)
	state.VPPAdminAccountName = helpers.StringPointerValueOrNull(g.VppAdminAccountName)

	state.IosApps = flattenContentList(iosAppItems(api.IosApps))
	state.MacApps = flattenContentList(macAppItems(api.MacApps))
	state.Ebooks = flattenContentList(ebookItems(api.Ebooks))

	state.Scope = &scope.UserScopeModel{
		Targets:     &scope.UserScopeTargetsModel{},
		Limitations: &scope.UserScopeLimitationsModel{},
		Exclusions:  &scope.UserScopeExclusionsModel{},
	}
	if api.Scope != nil {
		// Hydrate-all: a read-only lookup surfaces every category.
		flattenScope(ctx, api.Scope, state.Scope, true)
	}
}

// flattenScope refreshes the scope sub-blocks the caller already manages. When
// includeUnmanaged is set every wire-present sub-block is first allocated so a
// from-scratch read hydrates the full scope rather than leaving unmanaged
// targets/limitations/exclusions null.
func flattenScope(ctx context.Context, s *proclassic.VppAssignmentScope, state *scope.UserScopeModel, includeUnmanaged bool) {
	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &scope.UserScopeTargetsModel{}
		}
		if state.Limitations == nil && s.Limitations != nil {
			state.Limitations = &scope.UserScopeLimitationsModel{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &scope.UserScopeExclusionsModel{}
		}
	}

	// Sub-blocks are gated on caller management (typed-pointer models cannot
	// hold categories without the block struct); within a managed sub-block
	// each category refreshes independently via RefreshManagedSet — a category
	// the caller did not declare (null) stays null, so members maintained in
	// the admin UI never enter state. includeUnmanaged bypasses both gates for
	// import / config-generation / data-source hydration and for building the
	// server-side merge base in Update. The wire flatteners feed it known
	// (possibly empty) Sets — never null — so a managed-but-now-empty category
	// reconciles to `[]` rather than null.
	if state.Targets != nil {
		t := state.Targets
		t.AllJssUsers = scope.RefreshManagedBool(t.AllJssUsers, s.AllJssUsers, includeUnmanaged)
		t.JssUserIDs = scope.RefreshManagedSet(t.JssUserIDs, scope.FlattenIDNameSet(ctx, jssUsersSlice(s.JssUsers)), includeUnmanaged)
		t.JssUserGroupIDs = scope.RefreshManagedSet(t.JssUserGroupIDs, scope.FlattenIDNameSet(ctx, jssUserGroupsSlice(s.JssUserGroups)), includeUnmanaged)
	}

	if state.Limitations != nil {
		l := state.Limitations
		l.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(l.DirectoryServiceUserGroupNames, scope.FlattenNameSet(ctx, limitationsUserGroupsSlice(s.Limitations)), includeUnmanaged)
	}
	if state.Exclusions != nil && s.Exclusions != nil {
		x, e := state.Exclusions, s.Exclusions
		x.JssUserIDs = scope.RefreshManagedSet(x.JssUserIDs, scope.FlattenIDNameSet(ctx, exclJssUsersSlice(e.JssUsers)), includeUnmanaged)
		x.JssUserGroupIDs = scope.RefreshManagedSet(x.JssUserGroupIDs, scope.FlattenIDNameSet(ctx, exclJssUserGroupsSlice(e.JssUserGroups)), includeUnmanaged)
		x.DirectoryServiceUserGroupNames = scope.RefreshManagedSet(x.DirectoryServiceUserGroupNames, flattenExclDSGroupSet(ctx, exclDSGroupsSlice(e.UserGroups)), includeUnmanaged)
	}
}

// ---- content accessors / flatteners --------------------------------------------

func iosAppAdamIDs(a *proclassic.VppAssignmentIosApps) []*int {
	if a == nil || a.IosApp == nil {
		return nil
	}
	out := make([]*int, 0, len(*a.IosApp))
	for _, it := range *a.IosApp {
		out = append(out, it.AdamID)
	}
	return out
}

func macAppAdamIDs(a *proclassic.VppAssignmentMacApps) []*int {
	if a == nil || a.MacApp == nil {
		return nil
	}
	out := make([]*int, 0, len(*a.MacApp))
	for _, it := range *a.MacApp {
		out = append(out, it.AdamID)
	}
	return out
}

func ebookAdamIDs(a *proclassic.VppAssignmentEbooks) []*int {
	if a == nil || a.Ebook == nil {
		return nil
	}
	out := make([]*int, 0, len(*a.Ebook))
	for _, it := range *a.Ebook {
		out = append(out, it.AdamID)
	}
	return out
}

// flattenAdamSet builds a known Set[Int64] from the server's adam_id pointers.
// Returns a known empty Set (NOT null) for an empty/absent collection so a
// managed-but-now-cleared content set reconciles to [] rather than null.
func flattenAdamSet(ctx context.Context, ids []*int) types.Set {
	values := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			continue
		}
		values = append(values, int64(*id))
	}
	out, _ := types.SetValueFrom(ctx, types.Int64Type, values)
	return out
}

// contentName / contentAdamID adapt one element of each content type into the DS
// {adam_id, name} object. Defined per type because the SDK item structs share no
// interface.

func iosAppItems(a *proclassic.VppAssignmentIosApps) []contentTriple {
	if a == nil || a.IosApp == nil {
		return nil
	}
	out := make([]contentTriple, 0, len(*a.IosApp))
	for _, it := range *a.IosApp {
		out = append(out, contentTriple{adamID: it.AdamID, name: it.Name})
	}
	return out
}

func macAppItems(a *proclassic.VppAssignmentMacApps) []contentTriple {
	if a == nil || a.MacApp == nil {
		return nil
	}
	out := make([]contentTriple, 0, len(*a.MacApp))
	for _, it := range *a.MacApp {
		out = append(out, contentTriple{adamID: it.AdamID, name: it.Name})
	}
	return out
}

func ebookItems(a *proclassic.VppAssignmentEbooks) []contentTriple {
	if a == nil || a.Ebook == nil {
		return nil
	}
	out := make([]contentTriple, 0, len(*a.Ebook))
	for _, it := range *a.Ebook {
		out = append(out, contentTriple{adamID: it.AdamID, name: it.Name})
	}
	return out
}

// contentTriple is a type-erased content item for the DS list flatten.
type contentTriple struct {
	adamID *int
	name   *string
}

// flattenContentList builds a Computed list of {adam_id, name} objects. Always
// returns a known (possibly empty) list so the Computed attribute never stays
// unknown after read.
func flattenContentList(items []contentTriple) types.List {
	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		adam := types.Int64Null()
		if it.adamID != nil {
			adam = types.Int64Value(int64(*it.adamID))
		}
		elems = append(elems, types.ObjectValueMust(contentAttrTypes, map[string]attr.Value{
			"adam_id": adam,
			"name":    helpers.StringPointerValueOrNull(it.name),
		}))
	}
	return types.ListValueMust(contentObjectType, elems)
}

// ---- scope sub-slice accessors -------------------------------------------------

func jssUsersSlice(u *proclassic.VppAssignmentScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func jssUserGroupsSlice(g *proclassic.VppAssignmentScopeJssUserGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.UserGroup
}

func limitationsUserGroupsSlice(l *proclassic.VppAssignmentScopeLimitations) *[]proclassic.IDName {
	if l == nil || l.UserGroups == nil {
		return nil
	}
	return l.UserGroups.UserGroup
}

func exclJssUsersSlice(u *proclassic.VppAssignmentScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func exclJssUserGroupsSlice(g *proclassic.VppAssignmentScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.UserGroup
}

func exclDSGroupsSlice(g *proclassic.VppAssignmentScopeExclusionsUserGroups) *[]proclassic.VppAssignmentScopeExclusionsUserGroupsUserGroupItem {
	if g == nil {
		return nil
	}
	return g.UserGroup
}

// ---- scope set flatteners ------------------------------------------------------

func flattenExclDSGroupSet(ctx context.Context, items *[]proclassic.VppAssignmentScopeExclusionsUserGroupsUserGroupItem) types.Set {
	out, _ := scope.FlattenNameSlice(ctx, items, func(i proclassic.VppAssignmentScopeExclusionsUserGroupsUserGroupItem) *string { return i.Name })
	return out
}
