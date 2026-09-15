// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignVPPInvitationResourceModel refreshes a resource model from a GET. General
// scalars are always refreshed — the four email-mode strings through
// helpers.ReconcileOptionalStringPointer, because the input builder always
// emits them and an omitted attribute comes back as "", which must fold to
// null against the incoming model (plan on write, prior state on refresh)
// while an explicit "" the user configured is kept; the optional scope block is refreshed only when
// the caller already manages it (state.Scope non-nil) so the server's
// always-returned <scope> doesn't fabricate a block the user never declared.
// invitation_usages (read-only) is always refreshed.
//
// includeUnmanaged inverts the scope gate for the hydrate-everything paths —
// the list resource's config generation (terraform query -generate-config-out),
// import, and the scope merge base built by Update: there is no plan to stay
// consistent with, so every wire-present scope category is hydrated from the
// server. CRUD refresh callers pass false; within a managed scope block the
// per-category gate (scope.RefreshManagedSet) then keeps undeclared categories
// null.
func assignVPPInvitationResourceModel(ctx context.Context, state *VPPInvitationResourceModel, api *proclassic.VppInvitation, includeUnmanaged bool) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	if g.VppAccount != nil {
		state.VPPAccountID = helpers.StringValueFromIntPtr(g.VppAccount.ID)
	}
	state.DistributionMethod = helpers.StringPointerValueOrNull(g.DistributionMethod)
	state.AutoRegisterManagedUsers = helpers.BoolPointerValueOrNull(g.AutoRegisterManagedUsers)
	state.SenderName = helpers.ReconcileOptionalStringPointer(g.SenderName, state.SenderName)
	state.SenderEmailAddress = helpers.ReconcileOptionalStringPointer(g.SenderEmailAddress, state.SenderEmailAddress)
	state.Subject = helpers.ReconcileOptionalStringPointer(g.Subject, state.Subject)
	state.Message = helpers.ReconcileOptionalStringPointer(g.Message, state.Message)
	state.RequireLogin = helpers.BoolPointerValueOrNull(g.RequireLogin)

	if includeUnmanaged && state.Scope == nil && api.Scope != nil {
		state.Scope = &scope.UserScopeModel{}
	}
	if state.Scope != nil && api.Scope != nil {
		flattenScope(ctx, api.Scope, state.Scope, includeUnmanaged)
	}
	state.InvitationUsages = flattenUsages(api.InvitationUsages)
}

// assignVPPInvitationDataSourceModel populates a DS model from a GET. The DS
// always surfaces scope + usages (read-only lookup).
func assignVPPInvitationDataSourceModel(ctx context.Context, state *VPPInvitationDataSourceModel, api *proclassic.VppInvitation) {
	if api == nil || api.General == nil {
		return
	}
	g := api.General
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	if g.VppAccount != nil {
		state.VPPAccountID = helpers.StringValueFromIntPtr(g.VppAccount.ID)
	}
	state.DistributionMethod = helpers.StringPointerValueOrNull(g.DistributionMethod)
	state.AutoRegisterManagedUsers = helpers.BoolPointerValueOrNull(g.AutoRegisterManagedUsers)
	state.SenderName = helpers.StringPointerValueOrNull(g.SenderName)
	state.SenderEmailAddress = helpers.StringPointerValueOrNull(g.SenderEmailAddress)
	state.Subject = helpers.StringPointerValueOrNull(g.Subject)
	state.Message = helpers.StringPointerValueOrNull(g.Message)
	state.RequireLogin = helpers.BoolPointerValueOrNull(g.RequireLogin)

	state.Scope = &scope.UserScopeModel{
		Targets:     &scope.UserScopeTargetsModel{},
		Limitations: &scope.UserScopeLimitationsModel{},
		Exclusions:  &scope.UserScopeExclusionsModel{},
	}
	if api.Scope != nil {
		// Hydrate-all: a read-only lookup surfaces every category.
		flattenScope(ctx, api.Scope, state.Scope, true)
	}
	state.InvitationUsages = flattenUsages(api.InvitationUsages)
}

// flattenScope refreshes the scope sub-blocks the caller already manages. When
// includeUnmanaged is set every wire-present sub-block is first allocated so a
// from-scratch read hydrates the full scope rather than leaving unmanaged
// targets/limitations/exclusions null.
func flattenScope(ctx context.Context, s *proclassic.VppInvitationScope, state *scope.UserScopeModel, includeUnmanaged bool) {
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

// ---- scope sub-slice accessors -------------------------------------------------

func jssUsersSlice(u *proclassic.VppInvitationScopeJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func jssUserGroupsSlice(g *proclassic.VppInvitationScopeJssUserGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.UserGroup
}

func limitationsUserGroupsSlice(l *proclassic.VppInvitationScopeLimitations) *[]proclassic.IDName {
	if l == nil || l.UserGroups == nil {
		return nil
	}
	return l.UserGroups.UserGroup
}

func exclJssUsersSlice(u *proclassic.VppInvitationScopeExclusionsJssUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

func exclJssUserGroupsSlice(g *proclassic.VppInvitationScopeExclusionsJssUserGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.UserGroup
}

func exclDSGroupsSlice(g *proclassic.VppInvitationScopeExclusionsUserGroups) *[]proclassic.VppInvitationScopeExclusionsUserGroupsUserGroupItem {
	if g == nil {
		return nil
	}
	return g.UserGroup
}

// ---- set flatteners ------------------------------------------------------------

func flattenExclDSGroupSet(ctx context.Context, items *[]proclassic.VppInvitationScopeExclusionsUserGroupsUserGroupItem) types.Set {
	out, _ := scope.FlattenNameSlice(ctx, items, func(i proclassic.VppInvitationScopeExclusionsUserGroupsUserGroupItem) *string { return i.Name })
	return out
}
