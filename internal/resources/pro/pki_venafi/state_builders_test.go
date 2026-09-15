// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_venafi

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignVenafiServerFields(t *testing.T) {
	rec := &pro.VenafiCaRecord{
		ID:                     new(23),
		Name:                   "Venafi CA",
		ProxyAddress:           new("proxy.example.com:8443"),
		ClientID:               new("client-abc"),
		RevocationEnabled:      new(true),
		RefreshTokenConfigured: new(true),
	}
	state := PkiVenafiResourceModel{
		// Computed/echo fields carry no prior planned value here.
		ProxyAddress: types.StringValue("proxy.example.com:8443"),
		ClientID:     types.StringValue("client-abc"),
	}
	assignVenafiServerFields(&state, rec)

	if state.ID.ValueString() != "23" {
		t.Errorf("ID = %q, want 23", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Venafi CA" {
		t.Errorf("Name = %q", state.Name.ValueString())
	}
	if state.ProxyAddress.ValueString() != "proxy.example.com:8443" {
		t.Errorf("ProxyAddress = %q", state.ProxyAddress.ValueString())
	}
	if !state.RevocationEnabled.ValueBool() {
		t.Errorf("RevocationEnabled should be true")
	}
	if !state.RefreshTokenConfigured.ValueBool() {
		t.Errorf("RefreshTokenConfigured should be true")
	}
	// assign must not touch write-only / rotation triggers.
	if !state.RefreshTokenWo.IsNull() {
		t.Errorf("assign must not set refresh_token_wo")
	}
	if !state.JamfPublicKey.IsNull() {
		t.Errorf("assign must not set jamf_public_key")
	}
}

// Clearing proxy_address: the user sets "", the wire echoes the field
// absent/empty. The assigner must keep the planned "" (not collapse to null),
// otherwise Terraform reports a post-apply inconsistency.
func TestAssignVenafiServerFields_WireEmptyPreservesPlannedEmpty(t *testing.T) {
	rec := &pro.VenafiCaRecord{
		ID:           new(23),
		Name:         "Venafi CA",
		ProxyAddress: new(""), // wire echoes empty after a "" clear
		ClientID:     nil,     // wire omits entirely
	}
	state := PkiVenafiResourceModel{
		ProxyAddress: types.StringValue(""), // planned clear
		ClientID:     types.StringValue(""), // planned clear
	}
	assignVenafiServerFields(&state, rec)

	if state.ProxyAddress.IsNull() || state.ProxyAddress.ValueString() != "" {
		t.Errorf("ProxyAddress must keep planned empty string, got null=%v val=%q", state.ProxyAddress.IsNull(), state.ProxyAddress.ValueString())
	}
	if state.ClientID.IsNull() || state.ClientID.ValueString() != "" {
		t.Errorf("ClientID must keep planned empty string, got null=%v val=%q", state.ClientID.IsNull(), state.ClientID.ValueString())
	}
}

func TestAssignVenafiServerFields_NilIDLeavesIDUntouched(t *testing.T) {
	rec := &pro.VenafiCaRecord{ID: nil, Name: "Venafi CA"}
	state := PkiVenafiResourceModel{ID: types.StringValue("23")}
	assignVenafiServerFields(&state, rec)
	if state.ID.ValueString() != "23" {
		t.Errorf("nil record ID must leave existing ID untouched, got %q", state.ID.ValueString())
	}
}

func TestAssignVenafiDataSourceModel(t *testing.T) {
	rec := &pro.VenafiCaRecord{
		ID:                     new(7),
		Name:                   "DS CA",
		ProxyAddress:           new("p:443"),
		ClientID:               nil,
		RevocationEnabled:      new(false),
		RefreshTokenConfigured: new(true),
	}
	var data PkiVenafiDataSourceModel
	assignVenafiDataSourceModel(&data, rec)

	if data.ID.ValueString() != "7" {
		t.Errorf("ID = %q", data.ID.ValueString())
	}
	if data.ProxyAddress.ValueString() != "p:443" {
		t.Errorf("ProxyAddress = %q", data.ProxyAddress.ValueString())
	}
	if !data.ClientID.IsNull() {
		t.Errorf("nil ClientID must map to null on data source, got %q", data.ClientID.ValueString())
	}
	if !data.RefreshTokenConfigured.ValueBool() {
		t.Errorf("RefreshTokenConfigured should be true")
	}
}

func TestIDString(t *testing.T) {
	if s, ok := idString(new(23)); !ok || s != "23" {
		t.Errorf("idString(23) = (%q,%v), want (23,true)", s, ok)
	}
	if s, ok := idString(nil); ok || s != "" {
		t.Errorf("idString(nil) = (%q,%v), want (\"\",false)", s, ok)
	}
}
