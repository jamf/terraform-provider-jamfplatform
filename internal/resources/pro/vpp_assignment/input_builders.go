// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildVPPAssignmentInput projects the plan into an SDK *proclassic.VppAssignmentPost
// for Create / Update.
//
// General scalars (name, vpp_admin_account_id) are always-emitted. The three
// content collections are OPT-OUT, keyed on IsNull at the call site (NOT on
// whether the built slice is nil):
//   - null set        → leave the wire block nil → omitted → server retains it.
//   - empty set ([])  → emit a non-nil wrapper with a nil inner slice → marshals
//     as <ios_apps></ios_apps> → server clears it.
//   - populated set   → emit the full list → server full-replaces it.
//
// Content item name is server-resolved (wire-probed) — only adam_id is sent.
//
// Scope follows per-category granular ownership: only declared categories are
// emitted (see buildScope). A nil Scope omits <scope> entirely and leaves the
// server's scope untouched.
func buildVPPAssignmentInput(ctx context.Context, plan VPPAssignmentResourceModel) (*proclassic.VppAssignmentPost, diag.Diagnostics) {
	var diags diag.Diagnostics

	general := &proclassic.VppAssignmentPostGeneral{
		Name:              helpers.OptionalStringPointer(plan.Name),
		VppAdminAccountID: helpers.StringIDPtr(plan.VPPAdminAccountID),
	}
	out := &proclassic.VppAssignmentPost{General: general}

	// ios_apps (opt-out).
	if !plan.IosAppAdamIDs.IsNull() {
		items, d := buildAdamItems(ctx, plan.IosAppAdamIDs, func(id int) proclassic.VppAssignmentPostIosAppsIosAppItem {
			return proclassic.VppAssignmentPostIosAppsIosAppItem{AdamID: &id}
		})
		diags.Append(d...)
		out.IosApps = &proclassic.VppAssignmentPostIosApps{IosApp: items}
	}

	// mac_apps (opt-out).
	if !plan.MacAppAdamIDs.IsNull() {
		items, d := buildAdamItems(ctx, plan.MacAppAdamIDs, func(id int) proclassic.VppAssignmentPostMacAppsMacAppItem {
			return proclassic.VppAssignmentPostMacAppsMacAppItem{AdamID: &id}
		})
		diags.Append(d...)
		out.MacApps = &proclassic.VppAssignmentPostMacApps{MacApp: items}
	}

	// ebooks (opt-out).
	if !plan.EbookAdamIDs.IsNull() {
		items, d := buildAdamItems(ctx, plan.EbookAdamIDs, func(id int) proclassic.VppAssignmentPostEbooksEbookItem {
			return proclassic.VppAssignmentPostEbooksEbookItem{AdamID: &id}
		})
		diags.Append(d...)
		out.Ebooks = &proclassic.VppAssignmentPostEbooks{Ebook: items}
	}

	if plan.Scope != nil {
		s, d := buildScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	return out, diags
}

// buildScope projects a declared scope model into the wire scope, emitting
// only the categories the model declares: a null category leaves its wrapper
// nil so the element is omitted entirely (members maintained in the admin UI
// survive the write), while a declared `[]` yields a non-nil empty slice
// (scope.BuildIDSlice / BuildNameSlice) whose wrapper marshals as an explicit
// empty element — the clear gesture. Create passes the raw plan, so undeclared
// categories never reach the wire; Update passes the scope.MergeUserScope
// output, whose fields are all non-null, so the full skeleton emerges
// naturally from the merge and the replace-the-whole-subtree write lands
// exactly the merged model. The scope wrapper is the Post type, but its
// sub-blocks reuse the shared non-Post VppAssignmentScope* types.
func buildScope(ctx context.Context, m *scope.UserScopeModel) (*proclassic.VppAssignmentPostScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.VppAssignmentPostScope{
		AllJssUsers: helpers.OptionalBoolPointer(t.AllJssUsers),
	}

	jssUsers, d := scope.BuildIDSlice(ctx, t.JssUserIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if jssUsers != nil {
		s.JssUsers = &proclassic.VppAssignmentScopeJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, t.JssUserGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if jssUserGroups != nil {
		s.JssUserGroups = &proclassic.VppAssignmentScopeJssUserGroups{UserGroup: jssUserGroups}
	}

	// limitations.user_groups: directory-service groups, NAME-keyed (populate
	// Name). The block is emitted whenever declared; the category wrapper only
	// when the category itself is declared.
	if m.Limitations != nil {
		limNames, ld := scope.BuildNameSlice(ctx, m.Limitations.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
			n := name
			return proclassic.IDName{Name: &n}
		})
		diags.Append(ld...)
		l := &proclassic.VppAssignmentScopeLimitations{}
		if limNames != nil {
			l.UserGroups = &proclassic.VppAssignmentScopeLimitationsUserGroups{UserGroup: limNames}
		}
		s.Limitations = l
	}

	// exclusions: id-keyed jss_users / jss_user_groups + name-keyed user_groups.
	if m.Exclusions != nil {
		excl := &proclassic.VppAssignmentScopeExclusions{}
		exclJssUsers, ed := scope.BuildIDSlice(ctx, m.Exclusions.JssUserIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
		diags.Append(ed...)
		if exclJssUsers != nil {
			excl.JssUsers = &proclassic.VppAssignmentScopeExclusionsJssUsers{User: exclJssUsers}
		}
		exclJssUserGroups, ed := scope.BuildIDSlice(ctx, m.Exclusions.JssUserGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
		diags.Append(ed...)
		if exclJssUserGroups != nil {
			excl.JssUserGroups = &proclassic.VppAssignmentScopeExclusionsJssUserGroups{UserGroup: exclJssUserGroups}
		}
		exclDSGroups, ed := scope.BuildNameSlice(ctx, m.Exclusions.DirectoryServiceUserGroupNames, func(name string) proclassic.VppAssignmentScopeExclusionsUserGroupsUserGroupItem {
			n := name
			return proclassic.VppAssignmentScopeExclusionsUserGroupsUserGroupItem{Name: &n}
		})
		diags.Append(ed...)
		if exclDSGroups != nil {
			excl.UserGroups = &proclassic.VppAssignmentScopeExclusionsUserGroups{UserGroup: exclDSGroups}
		}
		s.Exclusions = excl
	}

	// Omission semantics (STYLE_GUIDE.md §Scope helper): collapse to nil when
	// nothing at all is declared so the payload omits <scope> entirely.
	if s.AllJssUsers == nil && s.JssUsers == nil && s.JssUserGroups == nil &&
		s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

// buildAdamItems projects a Terraform Set[Int64] of Apple catalog adam IDs into
// the SDK item pointer-slice. Returns nil (so the wrapper marshals as an empty,
// clearing element) for an empty/unknown set. The caller decides whether to emit
// the wrapper at all (the opt-out null check lives at the call site).
func buildAdamItems[T any](ctx context.Context, set types.Set, mk func(int) T) (*[]T, diag.Diagnostics) {
	var diags diag.Diagnostics
	if set.IsNull() || set.IsUnknown() {
		return nil, diags
	}
	var elements []int64
	diags.Append(set.ElementsAs(ctx, &elements, false)...)
	if diags.HasError() || len(elements) == 0 {
		return nil, diags
	}
	out := make([]T, 0, len(elements))
	for _, raw := range elements {
		out = append(out, mk(int(raw)))
	}
	return &out, diags
}
