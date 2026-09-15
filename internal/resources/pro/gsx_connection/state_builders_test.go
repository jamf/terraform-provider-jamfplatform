// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package gsx_connection

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignGsxConnectionSettingsResourceModel_Full verifies the assigner copies the
// non-secret + read-only-metadata fields and never populates the WriteOnly secrets.
func TestAssignGsxConnectionSettingsResourceModel_Full(t *testing.T) {
	ship := "54321"
	errMsg := "bad cert"
	var exp int64 = 1893456000000
	s := &pro.GsxConnection{
		Enabled:          true,
		Username:         "gsx@example.com",
		ServiceAccountNo: "1234567890",
		ShipToNo:         &ship,
		Token:            "should-be-ignored", // server never returns this; assigner must not read it
		GsxKeystore: pro.GsxKeystore{
			Name:            "cert.p12",
			ErrorMessage:    &errMsg,
			ExpirationEpoch: &exp,
		},
	}

	var state GsxConnectionSettingsResourceModel
	assignGsxConnectionSettingsResourceModel(&state, s)

	if !state.Enabled.ValueBool() {
		t.Errorf("Enabled not set")
	}
	if state.Username.ValueString() != "gsx@example.com" {
		t.Errorf("Username = %q", state.Username.ValueString())
	}
	if state.ServiceAccountNumber.ValueString() != "1234567890" {
		t.Errorf("ServiceAccountNumber = %q", state.ServiceAccountNumber.ValueString())
	}
	if state.ShipToNumber.ValueString() != "54321" {
		t.Errorf("ShipToNumber = %q", state.ShipToNumber.ValueString())
	}
	if state.KeystoreName.ValueString() != "cert.p12" {
		t.Errorf("KeystoreName = %q", state.KeystoreName.ValueString())
	}
	if state.KeystoreErrorMessage.ValueString() != "bad cert" {
		t.Errorf("KeystoreErrorMessage = %q", state.KeystoreErrorMessage.ValueString())
	}
	if state.KeystoreExpirationEpoch.ValueInt64() != exp {
		t.Errorf("KeystoreExpirationEpoch = %d", state.KeystoreExpirationEpoch.ValueInt64())
	}

	// The WriteOnly secrets must remain null — the assigner must never populate them.
	if !state.TokenWo.IsNull() || !state.KeystoreBytesWo.IsNull() || !state.KeystorePasswordWo.IsNull() {
		t.Errorf("WriteOnly secrets must not be assigned into state")
	}
}

// TestAssignGsxConnectionSettingsResourceModel_EmptyKeystore verifies a null/zero keystore
// (no certificate uploaded) yields null read-only metadata and an empty keystore name.
func TestAssignGsxConnectionSettingsResourceModel_EmptyKeystore(t *testing.T) {
	s := &pro.GsxConnection{
		Enabled:          false,
		Username:         "",
		ServiceAccountNo: "",
		ShipToNo:         nil,
		GsxKeystore:      pro.GsxKeystore{}, // zero value (server returned null)
	}

	var state GsxConnectionSettingsResourceModel
	assignGsxConnectionSettingsResourceModel(&state, s)

	if state.ShipToNumber.ValueString() != "" {
		t.Errorf("nil ShipToNo should map to empty string, got %q", state.ShipToNumber.ValueString())
	}
	if state.KeystoreName.ValueString() != "" {
		t.Errorf("empty keystore name should be empty string")
	}
	if !state.KeystoreErrorMessage.IsNull() {
		t.Errorf("nil errorMessage should be null")
	}
	if !state.KeystoreExpirationEpoch.IsNull() {
		t.Errorf("nil expirationEpoch should be null")
	}
}

// TestAssignGsxConnectionSettingsDataSourceModel verifies the data source assigner mirrors
// the resource assigner for the non-secret subset.
func TestAssignGsxConnectionSettingsDataSourceModel(t *testing.T) {
	s := &pro.GsxConnection{
		Enabled:          true,
		Username:         "gsx@example.com",
		ServiceAccountNo: "1234567890",
		GsxKeystore:      pro.GsxKeystore{Name: "cert.p12"},
	}
	var state GsxConnectionSettingsDataSourceModel
	assignGsxConnectionSettingsDataSourceModel(&state, s)

	if !state.Enabled.ValueBool() || state.Username.ValueString() != "gsx@example.com" || state.KeystoreName.ValueString() != "cert.p12" {
		t.Errorf("data source assigner did not copy fields: %+v", state)
	}
}
