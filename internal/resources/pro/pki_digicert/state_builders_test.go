// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_digicert

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignDigicertServerFields_WithCert(t *testing.T) {
	exp := "2036-06-06T17:42:41"
	resp := &pro.DigiCertSettingResponse{
		ID:                "24",
		CaName:            "tf-probe-digicert",
		Fqdn:              "one.digicert.com",
		RevocationEnabled: false,
		ClientCert: &pro.CertificateResponse{
			Filename:       "client.p12",
			SerialNumber:   "6569c979a052bba3cab79f363338c1c1102e7d7e",
			Subject:        "CN=pki-dummy",
			Issuer:         "CN=pki-dummy",
			ExpirationDate: &exp,
		},
	}

	var state DigicertResourceModel
	if diags := assignDigicertServerFields(&state, resp, false); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if state.ID.ValueString() != "24" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.DisplayName.ValueString() != "tf-probe-digicert" {
		t.Errorf("display_name: got %q", state.DisplayName.ValueString())
	}
	if state.HostName.ValueString() != "one.digicert.com" {
		t.Errorf("host_name: got %q", state.HostName.ValueString())
	}
	if state.RevocationEnabled.ValueBool() != false {
		t.Errorf("revocation_enabled: got %v", state.RevocationEnabled.ValueBool())
	}

	if state.ClientCertificateDetails.IsNull() {
		t.Fatalf("client_certificate_details must be populated when cert present")
	}
	attrs := state.ClientCertificateDetails.Attributes()
	if got := attrs["serial_number"].(types.String).ValueString(); got != "6569c979a052bba3cab79f363338c1c1102e7d7e" {
		t.Errorf("serial_number: got %q", got)
	}
	if got := attrs["subject"].(types.String).ValueString(); got != "CN=pki-dummy" {
		t.Errorf("subject: got %q", got)
	}
	if got := attrs["expiration_date"].(types.String).ValueString(); got != "2036-06-06T17:42:41" {
		t.Errorf("expiration_date: got %q, want verbatim wire value", got)
	}
}

func TestAssignDigicertServerFields_NoCert(t *testing.T) {
	resp := &pro.DigiCertSettingResponse{ID: "5", CaName: "x", Fqdn: "y"}
	var state DigicertResourceModel
	if diags := assignDigicertServerFields(&state, resp, false); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.ClientCertificateDetails.IsNull() {
		t.Errorf("client_certificate_details must be null when no cert present")
	}
}

func TestClientCertificateDetailsObject_NilExpiry(t *testing.T) {
	obj, diags := clientCertificateDetailsObject(&pro.CertificateResponse{Filename: "f"})
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if got := obj.Attributes()["expiration_date"].(types.String); !got.IsNull() {
		t.Errorf("expiration_date must be null when expiry nil, got %q", got.ValueString())
	}
}
