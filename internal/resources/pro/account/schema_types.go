// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// accountTimeoutAttributeTypes defines the timeout attribute types.
var accountTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// UI-facing enum values (config). Translated to/from the Pro wire spellings
// (wire-probed 2026-06-12) by the maps below.
//
// The access_level and privilege_set labels stay literals, and the reason is a
// judgement rather than an absence. They match proclassic.AccountAccessLevel
// and proclassic.AccountPrivilegeSet exactly, which is no coincidence — Jamf's
// admin UI labels are the classic API's own spellings. But the fields they
// configure are written to Pro v1 (CreateAccountV1 / UpdateAccountV1); the
// classic client this resource also holds is used only for the Custom privilege
// grid. Aliasing them would pin the provider's public schema vocabulary to the
// spec of an endpoint these fields never reach, so a classic respelling would
// silently become a breaking change to user configuration. The wire side below
// is a different matter and does alias.
//
// access_status is not in that position: it passes through untranslated to Pro
// v1's accountStatus, so it is that vocabulary and calls the helper.
var (
	accessLevelValues  = []string{"Full Access", "Site Access", "Group Access"}
	privilegeSetValues = []string{"Administrator", "Auditor", "Enrollment Only", "Custom"}
	accessStatusValues = pro.UserAccountAccountStatusValues()
	accountTypeValues  = pro.UserAccountAccountTypeValues()
)

// access_level UI <-> Pro wire. Pro uses "GroupBasedAccess" (not "GroupAccess").
var accessLevelToWire = map[string]string{
	"Full Access":  pro.UserAccountAccessLevelFullAccess,
	"Site Access":  pro.UserAccountAccessLevelSiteAccess,
	"Group Access": pro.UserAccountAccessLevelGroupBasedAccess,
}
var accessLevelFromWire = invert(accessLevelToWire)

// privilege_set UI <-> Pro wire. Pro uses "ENROLLMENT" (not "ENROLLMENT_ONLY").
var privilegeSetToWire = map[string]string{
	"Administrator":   pro.UserAccountPrivilegeLevelAdministrator,
	"Auditor":         pro.UserAccountPrivilegeLevelAuditor,
	"Enrollment Only": pro.UserAccountPrivilegeLevelEnrollment,
	"Custom":          pro.UserAccountPrivilegeLevelCustom,
}
var privilegeSetFromWire = invert(privilegeSetToWire)

func invert(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

// translate returns m[v]; if v is absent it returns v unchanged so an
// unexpected server value surfaces verbatim rather than being silently blanked.
func translate(m map[string]string, v string) string {
	if out, ok := m[v]; ok {
		return out
	}
	return v
}
