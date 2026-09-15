// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAssignJSONWebTokenConfigurationResourceModel_Mapping verifies the server
// response maps into the resource model, including the disabled→enabled
// inversion and the WriteOnly/rotation-trigger invariants.
func TestAssignJSONWebTokenConfigurationResourceModel_Mapping(t *testing.T) {
	c := &proclassic.JsonWebTokenConfiguration{
		ID:          new(33),
		Name:        new("token config"),
		TokenExpiry: new(30),
		Disabled:    new(false),
	}
	var state JSONWebTokenConfigurationResourceModel
	// Seed a wo_version to confirm it is preserved (server has no such field).
	state.EncryptionKeyWoVersion = types.Int64Value(3)

	assignJSONWebTokenConfigurationResourceModel(&state, c)

	if state.ID.ValueString() != "33" {
		t.Errorf("id = %q, want 33", state.ID.ValueString())
	}
	if state.Name.ValueString() != "token config" {
		t.Errorf("name not mapped")
	}
	if state.TokenExpiry.ValueInt64() != 30 {
		t.Errorf("token_expiry not mapped")
	}
	// disabled=false inverts to enabled=true.
	if state.Enabled.IsNull() || !state.Enabled.ValueBool() {
		t.Errorf("disabled=false must map to enabled=true, got %v", state.Enabled)
	}
	// encryption_key_wo is WriteOnly — must never be written into state.
	if !state.EncryptionKey.IsNull() {
		t.Errorf("encryption_key_wo must stay null in state, got %q", state.EncryptionKey.ValueString())
	}
	// wo_version preserved.
	if state.EncryptionKeyWoVersion.ValueInt64() != 3 {
		t.Errorf("encryption_key_wo_version must be preserved, got %v", state.EncryptionKeyWoVersion)
	}
}

// TestAssignJSONWebTokenConfigurationResourceModel_PlaintextNeverAssigned pins
// that even a server response carrying an encryption_key (defensive — the
// server never echoes one) does not leak into state.
func TestAssignJSONWebTokenConfigurationResourceModel_PlaintextNeverAssigned(t *testing.T) {
	c := &proclassic.JsonWebTokenConfiguration{
		ID:            new(33),
		Name:          new("x"),
		EncryptionKey: new("should-never-appear"),
	}
	var state JSONWebTokenConfigurationResourceModel
	assignJSONWebTokenConfigurationResourceModel(&state, c)
	if !state.EncryptionKey.IsNull() {
		t.Errorf("plaintext must never be assigned to state, got %q", state.EncryptionKey.ValueString())
	}
}

// TestEnabledFromDisabled_Inversion covers both polarities and nil handling.
func TestEnabledFromDisabled_Inversion(t *testing.T) {
	if got := enabledFromDisabled(new(true)); got.IsNull() || got.ValueBool() {
		t.Errorf("disabled=true must map to enabled=false, got %v", got)
	}
	if got := enabledFromDisabled(new(false)); got.IsNull() || !got.ValueBool() {
		t.Errorf("disabled=false must map to enabled=true, got %v", got)
	}
	if got := enabledFromDisabled(nil); !got.IsNull() {
		t.Errorf("absent disabled must map to null, got %v", got)
	}
}

// TestAssignJSONWebTokenConfigurationResourceModel_NilFields confirms nil
// pointer fields map to null without panicking, and a nil record is a no-op.
func TestAssignJSONWebTokenConfigurationResourceModel_NilFields(t *testing.T) {
	c := &proclassic.JsonWebTokenConfiguration{ID: new(7)}
	var state JSONWebTokenConfigurationResourceModel
	assignJSONWebTokenConfigurationResourceModel(&state, c)
	if !state.Name.IsNull() {
		t.Errorf("nil name must map to null")
	}
	if !state.TokenExpiry.IsNull() {
		t.Errorf("nil token_expiry must map to null")
	}
	if !state.Enabled.IsNull() {
		t.Errorf("nil disabled must map to null enabled")
	}

	// nil record leaves the model untouched.
	prior := JSONWebTokenConfigurationResourceModel{ID: types.StringValue("7")}
	assignJSONWebTokenConfigurationResourceModel(&prior, nil)
	if prior.ID.ValueString() != "7" {
		t.Errorf("nil record must be a no-op")
	}
}

// TestAssignJSONWebTokenConfigurationDataSourceModel_Mapping verifies the data
// source projection, including the disabled→enabled inversion.
func TestAssignJSONWebTokenConfigurationDataSourceModel_Mapping(t *testing.T) {
	c := &proclassic.JsonWebTokenConfiguration{
		ID:          new(34),
		Name:        new("lookup"),
		TokenExpiry: new(60),
		Disabled:    new(true),
	}
	var data JSONWebTokenConfigurationDataSourceModel
	assignJSONWebTokenConfigurationDataSourceModel(&data, c)

	if data.ID.ValueString() != "34" {
		t.Errorf("id = %q, want 34", data.ID.ValueString())
	}
	if data.Name.ValueString() != "lookup" {
		t.Errorf("name not mapped")
	}
	if data.TokenExpiry.ValueInt64() != 60 {
		t.Errorf("token_expiry not mapped")
	}
	if data.Enabled.IsNull() || data.Enabled.ValueBool() {
		t.Errorf("disabled=true must map to enabled=false, got %v", data.Enabled)
	}
}
