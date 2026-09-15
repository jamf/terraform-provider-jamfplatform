// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_adcs

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignResourceModel_Inbound(t *testing.T) {
	exp := "2036-06-06T17:42:41"
	checkIn := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	resp := &pro.AdcsSettingsResponse{
		ID:                            "25",
		DisplayName:                   "tf-adcs-in",
		CaName:                        "ca",
		Fqdn:                          "adcs.example.com",
		AdcsURL:                       "connector.example.com",
		Outbound:                      false,
		RevocationEnabled:             false,
		ApiClientID:                   "",
		ConnectorLastCheckInTimestamp: &checkIn,
		ServerCert:                    &pro.AdcsCertificateResponse{Filename: "server.pem", SerialNumber: "578", Subject: "CN=pki-dummy", Issuer: "CN=pki-dummy", ExpirationDate: &exp},
		ClientCert:                    &pro.AdcsCertificateResponse{Filename: "client.p12", SerialNumber: "579", Subject: "CN=pki-dummy", Issuer: "CN=pki-dummy", ExpirationDate: &exp},
	}

	state := AdcsResourceModel{
		// Input blocks present (with rotation-gate fields) — assigner must refresh
		// filename in place and leave wo_version/data_wo alone.
		ServerCertificate: &adcsCertInputModel{DataWo: types.StringValue("kept"), WoVersion: types.Int64Value(3)},
		ClientCertificate: &adcsClientCertInput{DataWo: types.StringValue("kept"), PasswordWo: types.StringValue("kept"), WoVersion: types.Int64Value(3)},
	}
	if diags := assignAdcsResourceModel(context.Background(), &state, resp, false); diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}

	if state.ConnectorMode.ValueString() != connectorModeInbound {
		t.Errorf("connector_mode = %q, want INBOUND", state.ConnectorMode.ValueString())
	}
	if !state.APIClientID.IsNull() {
		t.Error("api_client_id must be Null when server returns empty string")
	}
	if state.ConnectorLastCheckIn.IsNull() {
		t.Error("connector_last_check_in must be populated")
	}

	// Filename refreshed in place; rotation-gate fields untouched.
	if state.ServerCertificate.Filename.ValueString() != "server.pem" {
		t.Errorf("server filename = %q, want server.pem", state.ServerCertificate.Filename.ValueString())
	}
	if state.ServerCertificate.DataWo.ValueString() != "kept" || state.ServerCertificate.WoVersion.ValueInt64() != 3 {
		t.Error("server cert data_wo/wo_version must be preserved (rotation gate)")
	}
	if state.ClientCertificate.WoVersion.ValueInt64() != 3 || state.ClientCertificate.PasswordWo.ValueString() != "kept" {
		t.Error("client cert rotation-gate fields must be preserved")
	}

	// Details objects populated and non-null.
	if state.ServerCertificateDetails.IsNull() {
		t.Error("server_certificate_details must be populated")
	}
	if state.ClientCertificateDetails.IsNull() {
		t.Error("client_certificate_details must be populated")
	}
}

func TestAssignResourceModel_Outbound_NoCertDetails(t *testing.T) {
	resp := &pro.AdcsSettingsResponse{
		ID:          "30",
		Outbound:    true,
		ApiClientID: "11111111-2222-3333-4444-555555555555",
		ServerCert:  nil,
		ClientCert:  nil,
	}
	state := AdcsResourceModel{}
	if diags := assignAdcsResourceModel(context.Background(), &state, resp, false); diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}
	if state.ConnectorMode.ValueString() != connectorModeOutbound {
		t.Error("connector_mode must be OUTBOUND")
	}
	if state.APIClientID.ValueString() != "11111111-2222-3333-4444-555555555555" {
		t.Error("api_client_id must be populated for OUTBOUND")
	}
	if !state.ServerCertificateDetails.IsNull() || !state.ClientCertificateDetails.IsNull() {
		t.Error("details must be ObjectNull when server returns no certificate")
	}
	if !state.ConnectorLastCheckIn.IsNull() {
		t.Error("connector_last_check_in must be Null when timestamp absent")
	}
}

func TestAdcsCertExpiration(t *testing.T) {
	if !adcsCertExpiration(nil).IsNull() {
		t.Error("nil cert must yield Null")
	}
	if !adcsCertExpiration(&pro.AdcsCertificateResponse{}).IsNull() {
		t.Error("nil expiration must yield Null")
	}
	exp := "2036-06-06T17:42:41"
	got := adcsCertExpiration(&pro.AdcsCertificateResponse{ExpirationDate: &exp})
	if got.IsNull() || got.ValueString() == "" {
		t.Error("expiration must be populated when present")
	}
}
