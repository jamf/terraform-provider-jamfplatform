// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// stringOrNull is a nil-safe *string → TF String (empty → null).
func stringOrNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// intStringOrNull maps a nil/zero *int to a null TF String, else its decimal form.
func intStringOrNull(p *int) types.String {
	if p == nil || *p == 0 {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(*p))
}

// flattenUsages builds the Computed invitation_usages list from the SDK block.
// Always returns a known (possibly empty) list so the Computed attribute never
// stays unknown after apply.
func flattenUsages(u *proclassic.VppInvitationInvitationUsages) types.List {
	if u == nil || u.Usage == nil || len(*u.Usage) == 0 {
		return types.ListValueMust(usageObjectType, []attr.Value{})
	}
	elems := make([]attr.Value, 0, len(*u.Usage))
	for _, item := range *u.Usage {
		obj := types.ObjectValueMust(usageAttrTypes, map[string]attr.Value{
			"id":                     intStringOrNull(item.ID),
			"name":                   stringOrNull(item.Name),
			"email_address":          stringOrNull(item.EmailAddress),
			"status":                 stringOrNull(item.Status),
			"last_action_date_utc":   stringOrNull(item.LastActionDateUtc),
			"last_action_date_epoch": intStringOrNull(item.LastActionDateEpoch),
			"vpp_account":            stringOrNull(item.VppAccount),
		})
		elems = append(elems, obj)
	}
	return types.ListValueMust(usageObjectType, elems)
}
