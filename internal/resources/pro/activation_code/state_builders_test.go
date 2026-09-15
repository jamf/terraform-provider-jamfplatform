// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestAssignActivationCodeResourceModel(t *testing.T) {
	tests := []struct {
		name     string
		resp     *proclassic.ActivationCode
		wantOrg  string
		wantCode string
	}{
		{
			name:     "both populated round-trip",
			resp:     &proclassic.ActivationCode{OrganizationName: new("Acme Inc"), Code: new("ABCD-EFGH-IJKL")},
			wantOrg:  "Acme Inc",
			wantCode: "ABCD-EFGH-IJKL",
		},
		{
			name:     "nil pointers default to empty",
			resp:     &proclassic.ActivationCode{},
			wantOrg:  "",
			wantCode: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var state ActivationCodeResourceModel
			assignActivationCodeResourceModel(&state, tc.resp)
			if state.OrganizationName.IsNull() || state.Code.IsNull() {
				t.Fatalf("expected concrete strings, got null")
			}
			if state.OrganizationName.ValueString() != tc.wantOrg {
				t.Errorf("OrganizationName = %q, want %q", state.OrganizationName.ValueString(), tc.wantOrg)
			}
			if state.Code.ValueString() != tc.wantCode {
				t.Errorf("Code = %q, want %q", state.Code.ValueString(), tc.wantCode)
			}
		})
	}
}

func TestAssignActivationCodeDataSourceModel(t *testing.T) {
	var state ActivationCodeDataSourceModel
	assignActivationCodeDataSourceModel(&state, &proclassic.ActivationCode{OrganizationName: new("Acme"), Code: new("XXXX")})
	if state.OrganizationName.ValueString() != "Acme" || state.Code.ValueString() != "XXXX" {
		t.Errorf("data source assign mismatch: org=%q code=%q", state.OrganizationName.ValueString(), state.Code.ValueString())
	}
}

func TestAssign_DoesNotClobberID(t *testing.T) {
	state := ActivationCodeResourceModel{ID: types.StringValue("singleton")}
	assignActivationCodeResourceModel(&state, &proclassic.ActivationCode{OrganizationName: new("x"), Code: new("y")})
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q", state.ID.ValueString())
	}
}

func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
