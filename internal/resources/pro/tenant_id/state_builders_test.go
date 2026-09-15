// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenant_id

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestAssignTenantIDDataSourceModel(t *testing.T) {
	tenant := "ff584e5b-d9f8-4c1c-8752-449d8c5e45d5"
	empty := ""

	tests := []struct {
		name      string
		info      *pro.CsaTenantIDInfo
		wantError bool
		want      string
	}{
		{
			name: "populated",
			info: &pro.CsaTenantIDInfo{TenantID: &tenant},
			want: tenant,
		},
		{
			name:      "nil response",
			info:      nil,
			wantError: true,
		},
		{
			name:      "nil identifier",
			info:      &pro.CsaTenantIDInfo{},
			wantError: true,
		},
		{
			name:      "empty identifier",
			info:      &pro.CsaTenantIDInfo{TenantID: &empty},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var state TenantIDDataSourceModel
			diags := assignTenantIDDataSourceModel(&state, tc.info)

			if tc.wantError {
				if !diags.HasError() {
					t.Fatal("expected an error diagnostic, got none")
				}
				if !state.TenantID.IsNull() {
					t.Errorf("tenant_id was written despite the error: %q", state.TenantID.ValueString())
				}
				return
			}

			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got := state.TenantID.ValueString(); got != tc.want {
				t.Errorf("tenant_id = %q, want %q", got, tc.want)
			}
			if got := state.ID.ValueString(); got != helpers.SingletonID {
				t.Errorf("id = %q, want %q", got, helpers.SingletonID)
			}
		})
	}
}
