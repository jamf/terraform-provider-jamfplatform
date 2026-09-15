// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignAccountGroupResourceModel populates the resource model from a Group
// response. isImport distinguishes a fresh import (materialise the full server
// grid / membership) from a refresh (reconcile against what the user manages):
//   - privileges: intersect-on-read against the prior declared set so server-
//     added dependency privileges never enter state; categories the user does
//     not manage stay null.
//   - members: stored only when the user manages membership (prior non-null) or
//     on import; otherwise left null (directory-sourced membership is ignored).
func assignAccountGroupResourceModel(ctx context.Context, state *AccountGroupResourceModel, g *proclassic.Group, isImport bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if g == nil {
		return diags
	}

	assignServerDerivedBaseFields(state, g)

	// Members.
	if isImport {
		state.Members = memberSet(g.Members)
	} else if !state.Members.IsNull() && !state.Members.IsUnknown() {
		state.Members = memberSet(g.Members)
	} // else: unmanaged, leave null.

	// Privileges (intersect-on-read).
	serverPriv := accountprivileges.FromGroupPrivileges(g.Privileges)
	switch {
	case isImport:
		out, d := accountprivileges.IntersectIntoState(ctx, nil, serverPriv)
		diags.Append(d...)
		if out.IsEmpty() {
			state.Privileges = nil
		} else {
			state.Privileges = &out
		}
	case state.Privileges != nil:
		out, d := accountprivileges.IntersectIntoState(ctx, state.Privileges, serverPriv)
		diags.Append(d...)
		state.Privileges = &out
	} // else: unmanaged, leave nil.

	return diags
}

// assignServerDerivedBaseFields populates the scalar / site / ldap-server
// fields from a Group response. It deliberately leaves privileges and members
// untouched so write paths (Create/Update) can trust the planned values for
// those server-expanded collections (avoiding a hard "inconsistent result after
// apply" when Jamf Pro silently adds dependency privileges or directory
// members). Read calls assignAccountGroupResourceModel, which additionally
// reconciles privileges/members.
func assignServerDerivedBaseFields(state *AccountGroupResourceModel, g *proclassic.Group) {
	if g == nil {
		return
	}
	if g.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(g.ID)
	}
	if g.Name != nil {
		state.DisplayName = helpers.StringPointerValueOrNull(g.Name)
	}
	if g.AccessLevel != nil {
		state.AccessLevel = helpers.StringPointerValueOrNull(g.AccessLevel)
	}
	if g.PrivilegeSet != nil {
		state.PrivilegeSet = helpers.StringPointerValueOrNull(g.PrivilegeSet)
	}

	// Site (id Optional+Computed, name derived/Computed).
	if g.Site != nil && g.Site.ID != nil {
		state.SiteID = types.Int64Value(int64(*g.Site.ID))
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	} else {
		state.SiteID = types.Int64Value(-1)
		state.SiteName = types.StringNull()
	}

	// LDAP server (id Optional, name derived/Computed). Absent => null.
	if g.LdapServer != nil && g.LdapServer.ID != nil && *g.LdapServer.ID > 0 {
		state.LdapServerID = types.Int64Value(int64(*g.LdapServer.ID))
		state.LdapServerName = helpers.StringPointerValueOrNull(g.LdapServer.Name)
	} else {
		state.LdapServerID = types.Int64Null()
		state.LdapServerName = types.StringNull()
	}
}

// memberSet builds a Set[Int64] of account IDs from a Group's members block.
// A nil/empty members block yields an empty (non-null) set.
func memberSet(m *proclassic.GroupMembers) types.Set {
	var elems []attr.Value
	if m != nil && m.User != nil {
		for _, u := range *m.User {
			if u.ID != nil {
				elems = append(elems, types.Int64Value(int64(*u.ID)))
			}
		}
	}
	set, _ := types.SetValue(types.Int64Type, elems)
	return set
}
