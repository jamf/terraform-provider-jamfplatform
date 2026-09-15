// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package gsx_connection

import (
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// TestBuildGsxConnectionInput_FullPlan verifies every plan/config field maps to the right
// SDK field, the secrets come from config, and the keystore bytes are base64-decoded.
func TestBuildGsxConnectionInput_FullPlan(t *testing.T) {
	plan := GsxConnectionSettingsResourceModel{
		Enabled:              types.BoolValue(true),
		Username:             types.StringValue("gsx@example.com"),
		ServiceAccountNumber: types.StringValue("1234567890"),
		ShipToNumber:         types.StringValue("54321"),
		KeystoreName:         types.StringValue("cert.p12"),
	}
	cfg := GsxConnectionSettingsResourceModel{
		TokenWo:            types.StringValue("tok-123"),
		KeystoreBytesWo:    types.StringValue(b64("p12-bytes")),
		KeystorePasswordWo: types.StringValue("pw-123"),
	}

	out, err := buildGsxConnectionInput(plan, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if out.Username != "gsx@example.com" {
		t.Errorf("Username = %q", out.Username)
	}
	if out.ServiceAccountNo != "1234567890" {
		t.Errorf("ServiceAccountNo = %q", out.ServiceAccountNo)
	}
	if out.ShipToNo == nil || *out.ShipToNo != "54321" {
		t.Errorf("ShipToNo = %v", out.ShipToNo)
	}
	if out.Token != "tok-123" {
		t.Errorf("Token = %q (must come from config)", out.Token)
	}
	if out.GsxKeystore.KeystorePassword != "pw-123" {
		t.Errorf("KeystorePassword = %q (must come from config)", out.GsxKeystore.KeystorePassword)
	}
	if out.GsxKeystore.Name != "cert.p12" {
		t.Errorf("keystore Name = %q", out.GsxKeystore.Name)
	}
	if out.GsxKeystore.KeystoreBytes == nil || string(*out.GsxKeystore.KeystoreBytes) != "p12-bytes" {
		t.Errorf("KeystoreBytes not base64-decoded correctly: %v", out.GsxKeystore.KeystoreBytes)
	}
}

// TestBuildGsxConnectionInput_BadBase64 verifies an invalid keystore_bytes_wo returns an error.
func TestBuildGsxConnectionInput_BadBase64(t *testing.T) {
	cfg := GsxConnectionSettingsResourceModel{
		TokenWo:            types.StringValue("t"),
		KeystoreBytesWo:    types.StringValue("not!valid!base64!"),
		KeystorePasswordWo: types.StringValue("p"),
	}
	if _, err := buildGsxConnectionInput(GsxConnectionSettingsResourceModel{}, cfg, nil); err == nil {
		t.Errorf("expected error for invalid base64, got nil")
	}
}

// TestBuildGsxConnectionInput_OmittedAdoptsCurrent verifies omitted Optional fields adopt
// the live `current` values on first create, while a declared field wins.
func TestBuildGsxConnectionInput_OmittedAdoptsCurrent(t *testing.T) {
	ship := "99999"
	current := &pro.GsxConnection{
		Enabled:     true,
		ShipToNo:    &ship,
		GsxKeystore: pro.GsxKeystore{Name: "existing.p12"},
	}
	plan := GsxConnectionSettingsResourceModel{
		Enabled:              types.BoolNull(), // adopt true
		Username:             types.StringValue("u"),
		ServiceAccountNumber: types.StringValue("1"),
		ShipToNumber:         types.StringUnknown(), // adopt 99999
		KeystoreName:         types.StringNull(),    // adopt existing.p12
	}
	cfg := GsxConnectionSettingsResourceModel{
		TokenWo:            types.StringValue("t"),
		KeystoreBytesWo:    types.StringValue(b64("x")),
		KeystorePasswordWo: types.StringValue("p"),
	}

	out, err := buildGsxConnectionInput(plan, cfg, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Enabled {
		t.Errorf("omitted enabled should adopt current true")
	}
	if out.ShipToNo == nil || *out.ShipToNo != "99999" {
		t.Errorf("omitted ship_to should adopt current, got %v", out.ShipToNo)
	}
	if out.GsxKeystore.Name != "existing.p12" {
		t.Errorf("omitted keystore_name should adopt current, got %q", out.GsxKeystore.Name)
	}
}

// TestBuildGsxConnectionInput_EmptyShipToOmitted verifies an empty ship-to yields a nil
// pointer (omitted on the wire) rather than an empty string.
func TestBuildGsxConnectionInput_EmptyShipToOmitted(t *testing.T) {
	plan := GsxConnectionSettingsResourceModel{
		Username:             types.StringValue("u"),
		ServiceAccountNumber: types.StringValue("1"),
		ShipToNumber:         types.StringValue(""),
	}
	cfg := GsxConnectionSettingsResourceModel{
		TokenWo:            types.StringValue("t"),
		KeystoreBytesWo:    types.StringValue(b64("x")),
		KeystorePasswordWo: types.StringValue("p"),
	}
	out, err := buildGsxConnectionInput(plan, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ShipToNo != nil {
		t.Errorf("empty ship_to should be nil (omitted), got %v", *out.ShipToNo)
	}
}
