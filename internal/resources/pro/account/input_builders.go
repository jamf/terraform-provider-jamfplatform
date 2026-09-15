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

// optTranslatedString returns a pointer to the wire-translated value of a
// UI-facing enum string, or nil when the value is null/unknown.
func optTranslatedString(m map[string]string, v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := translate(m, v.ValueString())
	return &out
}

// buildProUserAccount projects the plan into a COMPLETE Pro v1 UserAccount
// payload. The Pro /accounts endpoint NPEs (HTTP 500 with an empty error list)
// when fields are omitted, so every field is always populated — Optional+
// Computed fields that are null/unknown on create fall back to the same defaults
// the Jamf Pro UI uses (Enabled / DEFAULT / no site / no LDAP / no forced
// change). UI enum values are translated to Pro wire spellings. password is
// supplied separately (WriteOnly, from config) and sent only when non-empty.
func buildProUserAccount(plan AccountResourceModel, password *string) *pro.UserAccount {
	empty := ""
	acct := &pro.UserAccount{
		Username:                  helpers.OptionalStringPointer(plan.Username),
		Realname:                  strOrDefault(plan.FullName, ""),
		Email:                     strOrDefault(plan.EmailAddress, ""),
		AccessLevel:               optTranslatedString(accessLevelToWire, plan.AccessLevel),
		PrivilegeLevel:            optTranslatedString(privilegeSetToWire, plan.PrivilegeSet),
		AccountStatus:             strOrDefault(plan.AccessStatus, pro.UserAccountAccountStatusEnabled),
		AccountType:               strOrDefault(plan.AccountType, pro.UserAccountAccountTypeDefault),
		LdapServerID:              intOrDefault(plan.LdapServerID, -1),
		SiteID:                    intOrDefault(plan.SiteID, -1),
		ChangePasswordOnNextLogin: boolOrDefault(plan.ForcePasswordChange, false),
		// Not modelled as attributes, but the endpoint requires their presence.
		DistinguishedName: &empty,
		Phone:             &empty,
	}
	if password != nil && *password != "" {
		acct.PlainPassword = password
	}
	return acct
}

// strOrDefault returns a pointer to the string value, or to def when the value
// is null/unknown (so the full payload shape is always sent).
func strOrDefault(v types.String, def string) *string {
	if v.IsNull() || v.IsUnknown() {
		return &def
	}
	s := v.ValueString()
	return &s
}

// intOrDefault returns a pointer to the int value, or to def when null/unknown.
func intOrDefault(v types.Int64, def int) *int {
	if v.IsNull() || v.IsUnknown() {
		return &def
	}
	n := int(v.ValueInt64())
	return &n
}

// boolOrDefault returns a pointer to the bool value, or to def when null/unknown.
func boolOrDefault(v types.Bool, def bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return &def
	}
	b := v.ValueBool()
	return &b
}

// buildClassicPrivileges projects the Custom privilege grid into a ProClassic
// Account payload carrying only <privileges>. The classic PUT merges at the top
// level, so other account fields are intentionally omitted (they are owned by
// the Pro side). Inside <privileges> it does not merge: a sent element replaces
// the whole grid (wire-probed 2026-09-06, Jamf Pro 11.31.1, issue #385), so the
// grid is built by accountprivileges.MergeGrid from live, the account as the
// classic endpoint currently returns it, replacing only the declared categories
// and carrying the rest verbatim. live may be nil when nothing has been read.
// Returns nil when there are no privileges to send.
func buildClassicPrivileges(ctx context.Context, plan AccountResourceModel, live *proclassic.Account) (*proclassic.Account, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !managesPrivileges(plan) {
		return nil, diags
	}
	var serverGrid map[string][]string
	if live != nil {
		serverGrid = accountprivileges.FromAccountPrivileges(live.Privileges)
	}
	merged, d := accountprivileges.MergeGrid(ctx, plan.Privileges, serverGrid)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}
	grid := accountprivileges.ToAccountPrivileges(merged)
	if grid == nil {
		return nil, diags
	}
	return &proclassic.Account{Privileges: grid}, diags
}

// managesPrivileges reports whether the plan declares at least one privilege
// category. When false no classic write is issued and the server keeps its
// grid, which is the one retention the wire does provide.
func managesPrivileges(plan AccountResourceModel) bool {
	return plan.Privileges != nil && !plan.Privileges.IsEmpty()
}
