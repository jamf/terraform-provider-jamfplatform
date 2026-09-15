// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignProBaseFields populates the base (Pro-owned) fields from a Pro v1
// UserAccount response, translating Pro wire enum spellings back to the UI
// values. Privileges are NOT touched here (they come from the classic side).
func assignProBaseFields(state *AccountResourceModel, a *pro.UserAccount) {
	if a == nil {
		return
	}
	if a.ID != nil {
		state.ID = helpers.StringPointerValueOrNull(a.ID)
	}
	if a.Username != nil {
		state.Username = helpers.StringPointerValueOrNull(a.Username)
	}
	state.FullName = helpers.StringPointerValueOrNull(a.Realname)
	state.EmailAddress = helpers.StringPointerValueOrNull(a.Email)
	if a.AccessLevel != nil {
		state.AccessLevel = types.StringValue(translate(accessLevelFromWire, *a.AccessLevel))
	}
	if a.PrivilegeLevel != nil {
		state.PrivilegeSet = types.StringValue(translate(privilegeSetFromWire, *a.PrivilegeLevel))
	}
	state.AccessStatus = helpers.StringPointerValueOrNull(a.AccountStatus)
	state.AccountType = helpers.StringPointerValueOrNull(a.AccountType)
	state.LdapServerID = int64OrNull(a.LdapServerID)
	state.SiteID = int64OrNull(a.SiteID)
	state.ForcePasswordChange = helpers.BoolPointerValueOrNull(a.ChangePasswordOnNextLogin)
}

// importHydration reports whether this Read is the one Terraform issues after an
// import, with state holding the account id and nothing else. A null state value
// cannot answer that: the plugin framework writes the passthrough id into state
// before calling Read, so on the import path Read receives a populated object and
// `Raw.IsNull()` returns false. A null `username` does answer it, because the
// attribute is Required and every other path into Read sets it.
//
// The username Read passes here comes off the immutable request state, never off
// the model Read is assembling. `assignProBaseFields` writes `username` from the
// Pro response, so a signal taken from the model would answer differently
// depending on where in Read it was sampled, and the request state is the only
// source no later assignment can reach. Wire-verified 2026-09-04: the post-import
// Read reached assignClassicPrivileges with a false import flag and a populated
// `username`, and the privilege grid the classic endpoint had returned in full
// was discarded (issue #372).
func importHydration(stateAbsent bool, username types.String) bool {
	return stateAbsent || username.IsNull()
}

// assignClassicPrivileges reconciles the Custom privilege grid from a ProClassic
// Account response using intersect-on-read (same semantics as account_group):
// declared ∩ server per managed category, null categories stay null, import
// materialises the full grid.
func assignClassicPrivileges(ctx context.Context, state *AccountResourceModel, a *proclassic.Account, hydrating bool) diag.Diagnostics {
	var diags diag.Diagnostics
	var serverPriv map[string][]string
	if a != nil {
		serverPriv = accountprivileges.FromAccountPrivileges(a.Privileges)
	}
	switch {
	case hydrating:
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
	}
	return diags
}

// int64OrNull converts a *int to a types.Int64 (null when nil).
func int64OrNull(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}
