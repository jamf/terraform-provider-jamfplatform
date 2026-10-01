// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignProBaseFields_EmptyStringReconcile pins how full_name and
// email_address read back when Jamf Pro echoes "": an authored "" is kept, an
// unset or unknown value lands null, and a populated echo always wins.
func TestAssignProBaseFields_EmptyStringReconcile(t *testing.T) {
	empty := ""
	named := "TF Acc"
	tests := []struct {
		name  string
		prior types.String
		wire  *string
		want  types.String
	}{
		{"authored empty kept", types.StringValue(""), &empty, types.StringValue("")},
		{"unset stays null", types.StringNull(), &empty, types.StringNull()},
		{"unknown resolves null", types.StringUnknown(), &empty, types.StringNull()},
		{"absent field stays null", types.StringNull(), nil, types.StringNull()},
		{"echo beats authored empty", types.StringValue(""), &named, types.StringValue(named)},
		{"cleared out of band", types.StringValue(named), &empty, types.StringNull()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := AccountResourceModel{FullName: tc.prior, EmailAddress: tc.prior}
			assignProBaseFields(&state, &pro.UserAccount{Realname: tc.wire, Email: tc.wire})
			if !state.FullName.Equal(tc.want) {
				t.Errorf("full_name = %s, want %s", state.FullName, tc.want)
			}
			if !state.EmailAddress.Equal(tc.want) {
				t.Errorf("email_address = %s, want %s", state.EmailAddress, tc.want)
			}
		})
	}
}
