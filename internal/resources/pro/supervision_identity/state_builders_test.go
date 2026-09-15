// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignSupervisionIdentityResourceModel(t *testing.T) {
	// Seed secrets to confirm the assigner never touches them.
	state := SupervisionIdentityResourceModel{
		Password:        types.StringValue("should-stay"),
		CertificateData: types.StringValue("should-stay-too"),
	}
	s := &pro.SupervisionIdentity{
		ID:             40,
		DisplayName:    "Configurator",
		CommonName:     "Jamf Identity - Configurator",
		ExpirationDate: "2031-06-12",
	}

	assignSupervisionIdentityResourceModel(&state, s)

	if state.ID.ValueString() != "40" {
		t.Errorf("ID = %q, want %q", state.ID.ValueString(), "40")
	}
	if state.DisplayName.ValueString() != "Configurator" {
		t.Errorf("DisplayName = %q", state.DisplayName.ValueString())
	}
	if state.CommonName.ValueString() != "Jamf Identity - Configurator" {
		t.Errorf("CommonName = %q", state.CommonName.ValueString())
	}
	if state.ExpirationDate.ValueString() != "2031-06-12" {
		t.Errorf("ExpirationDate = %q", state.ExpirationDate.ValueString())
	}
	// Secrets must be untouched (they are WriteOnly; the framework strips them
	// from state regardless, but the assigner must not clobber them either).
	if state.Password.ValueString() != "should-stay" {
		t.Errorf("Password was modified: %q", state.Password.ValueString())
	}
	if state.CertificateData.ValueString() != "should-stay-too" {
		t.Errorf("CertificateData was modified: %q", state.CertificateData.ValueString())
	}
}

// TestAssignSupervisionIdentityResourceModel_NilSafe verifies a nil response and a
// zero ID do not clobber existing state.
func TestAssignSupervisionIdentityResourceModel_NilSafe(t *testing.T) {
	state := SupervisionIdentityResourceModel{ID: types.StringValue("7")}
	assignSupervisionIdentityResourceModel(&state, nil)
	if state.ID.ValueString() != "7" {
		t.Errorf("nil response clobbered ID: %q", state.ID.ValueString())
	}

	assignSupervisionIdentityResourceModel(&state, &pro.SupervisionIdentity{ID: 0, DisplayName: "x"})
	if state.ID.ValueString() != "7" {
		t.Errorf("zero ID clobbered existing ID: %q", state.ID.ValueString())
	}
}

func TestAssignSupervisionIdentityDataSourceModel(t *testing.T) {
	var data SupervisionIdentityDataSourceModel
	s := &pro.SupervisionIdentity{
		ID:             37,
		DisplayName:    "Test",
		CommonName:     "Jamf Identity - Test",
		ExpirationDate: "2031-06-12",
	}
	assignSupervisionIdentityDataSourceModel(&data, s)

	if data.ID.ValueString() != "37" {
		t.Errorf("ID = %q", data.ID.ValueString())
	}
	if data.DisplayName.ValueString() != "Test" {
		t.Errorf("DisplayName = %q", data.DisplayName.ValueString())
	}
	if data.CommonName.ValueString() != "Jamf Identity - Test" {
		t.Errorf("CommonName = %q", data.CommonName.ValueString())
	}
	if data.ExpirationDate.ValueString() != "2031-06-12" {
		t.Errorf("ExpirationDate = %q", data.ExpirationDate.ValueString())
	}
}
