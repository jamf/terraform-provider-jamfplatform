// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ldapServerNone is the id the classic /accounts/groupid endpoint takes as
// "no directory server". It is the only clear token the endpoint accepts:
// wire-probed 2026-09-06 on Jamf Pro 11.31.1, a PUT carrying
// <ldap_server><id>-1</id></ldap_server> removes the association (the GET
// afterwards carries no <ldap_server> element at all), <id>0</id> is refused
// `409 Problem with LDAP Server ID`, and an empty <ldap_server></ldap_server>
// is merged like an omission and retains the server. -1 is also accepted on a
// group that never had a server, so Create can send it unconditionally.
const ldapServerNone = -1

// ldapServerIDForWrite encodes `ldap_server_id` for the classic PUT, which
// merges field by field and would otherwise keep a directory server the config
// dropped: Read then echoes the retained id back as an inconsistent result
// (issue #384). Null therefore becomes ldapServerNone rather than an omitted
// element; the state builder already reads an absent or non-positive id as
// null, so the cleared association round-trips. Unknown is omitted — the
// attribute is Optional-only, so that cannot arise at apply, but the helper
// keeps the shared contract of the AlwaysEmit* helpers.
func ldapServerIDForWrite(value types.Int64) *int {
	if value.IsNull() {
		return new(ldapServerNone)
	}
	return helpers.OptionalInt64Pointer(value)
}

// customPrivilegeSet reports whether the group's privilege_set is the one Jamf
// Pro honours a declared privilege grid for. A preset set ("Administrator",
// "Auditor", "Enrollment Only") expands into a server-owned grid that no write
// can change: the classic endpoint discards a <privileges> element sent
// alongside a preset set, wire-verified 2026-09-08 on Jamf Pro 11.x, where a
// create carrying privilege_set = "Auditor" and a one-entry jss_objects grid
// read back the full preset expansion instead — 89 object privileges and 46
// settings privileges. That is why the write path omits the element
// (managesPrivileges), why neither hydration path adopts the expansion, and why
// ModifyPlan does not validate a grid it cannot send.
func customPrivilegeSet(privilegeSet types.String) bool {
	return privilegeSet.ValueString() == proclassic.GroupPrivilegeSetCustom
}
