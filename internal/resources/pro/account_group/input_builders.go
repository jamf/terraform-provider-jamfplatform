// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildAccountGroupInput converts the Terraform plan into an SDK Group payload.
// The classic /accounts/groupid PUT merges at the top level only: an element
// that is not sent is retained, so null members are omitted (retained) and an
// explicitly empty members set clears membership. Inside <privileges> there is
// no merge — a sent element replaces the whole grid (wire-probed 2026-09-06,
// Jamf Pro 11.31.1, issue #385) — so when the plan declares privileges the grid
// is built by accountprivileges.MergeGrid from the live group's grid, replacing
// only the declared categories and carrying the rest verbatim. live is the
// group as the server currently holds it and may be nil on Create, where nothing
// exists to carry. Privileges are only emitted when the privilege set is Custom
// (Jamf Pro ignores them otherwise).
func buildAccountGroupInput(ctx context.Context, plan AccountGroupResourceModel, live *proclassic.Group) (*proclassic.Group, diag.Diagnostics) {
	var diags diag.Diagnostics

	group := &proclassic.Group{
		Name:         helpers.OptionalStringPointer(plan.DisplayName),
		AccessLevel:  helpers.OptionalStringPointer(plan.AccessLevel),
		PrivilegeSet: helpers.OptionalStringPointer(plan.PrivilegeSet),
	}

	if id := helpers.OptionalInt64Pointer(plan.SiteID); id != nil {
		group.Site = &proclassic.Site{ID: id}
	}
	group.LdapServer = &proclassic.GroupLdapServer{ID: ldapServerIDForWrite(plan.LdapServerID)}

	// Members: null => omit (retain server/directory membership); set (incl.
	// empty) => send, where empty clears.
	if !plan.Members.IsNull() && !plan.Members.IsUnknown() {
		var ids []int64
		diags.Append(plan.Members.ElementsAs(ctx, &ids, false)...)
		if diags.HasError() {
			return nil, diags
		}
		users := make([]proclassic.IDName, 0, len(ids))
		for _, id := range ids {
			id := int(id)
			users = append(users, proclassic.IDName{ID: &id})
		}
		group.Members = &proclassic.GroupMembers{User: &users}
	}

	if managesPrivileges(plan) {
		var serverGrid map[string][]string
		if live != nil {
			serverGrid = accountprivileges.FromGroupPrivileges(live.Privileges)
		}
		merged, d := accountprivileges.MergeGrid(ctx, plan.Privileges, serverGrid)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		group.Privileges = accountprivileges.ToGroupPrivileges(merged)
	}

	return group, diags
}

// managesPrivileges reports whether the plan will send a <privileges> element:
// the group is Custom (see customPrivilegeSet) and the block is present with at
// least one declared category. When false the element is omitted and the server
// keeps its grid, which is the one retention the wire does provide.
func managesPrivileges(plan AccountGroupResourceModel) bool {
	return customPrivilegeSet(plan.PrivilegeSet) && plan.Privileges != nil && !plan.Privileges.IsEmpty()
}
