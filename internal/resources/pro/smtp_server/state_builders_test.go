// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smtp_server

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignState_BasicMode(t *testing.T) {
	resp := &pro.SmtpServerV2{
		AuthenticationType: authBasic,
		Enabled:            true,
		SenderSettings:     pro.SmtpSenderSettings{EmailAddress: "s@example.com"},
		ConnectionSettings: &pro.SmtpConnectionSettings{Host: "h", Port: 465, EncryptionType: "SSL", ConnectionTimeout: 30},
		BasicAuthCredentials: &pro.SmtpBasicCredentials{
			Username: "user@example.com",
			// Password never returned by server.
		},
	}
	prior := &SmtpServerResourceModel{BasicAuthCredentials: &smtpBasicCredentialsModel{PasswordWoVersion: types.Int64Value(3)}}

	var state SmtpServerResourceModel
	if diags := assignSmtpServerResourceModel(&state, resp, prior); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if state.ConnectionSettings == nil || state.ConnectionSettings.Host.ValueString() != "h" {
		t.Errorf("connection_settings missing/wrong: %+v", state.ConnectionSettings)
	}
	if state.BasicAuthCredentials == nil || state.BasicAuthCredentials.Username.ValueString() != "user@example.com" {
		t.Errorf("basic creds wrong: %+v", state.BasicAuthCredentials)
	}
	if !state.BasicAuthCredentials.Password.IsNull() {
		t.Error("WriteOnly password must be null in state")
	}
	if state.BasicAuthCredentials.PasswordWoVersion.ValueInt64() != 3 {
		t.Errorf("wo_version must be carried from prior; got %v", state.BasicAuthCredentials.PasswordWoVersion)
	}
	if state.GraphAPICredentials != nil || state.GoogleMailCredentials != nil {
		t.Error("foreign credential blocks must be nil")
	}
}

// TestAssignState_AliasedPriorPreservesWoVersion is the regression test for the
// state/prior aliasing bug: CRUD passes the SAME pointer for state and prior (the
// plan is read back into itself). Nilling state.GraphAPICredentials before
// reading prior's wo_version dropped it to null → "inconsistent result after
// apply" masked to the sensitive credential block. The wo_version must survive.
func TestAssignState_AliasedPriorPreservesWoVersion(t *testing.T) {
	m := SmtpServerResourceModel{
		AuthenticationType: types.StringValue(authGraphAPI),
		GraphAPICredentials: &smtpGraphAPICredentialsModel{
			ClientID:              types.StringValue("cid"),
			TenantID:              types.StringValue("tid"),
			ClientSecret:          types.StringValue("plan-secret"),
			ClientSecretWoVersion: types.Int64Value(7),
		},
	}
	resp := &pro.SmtpServerV2{
		AuthenticationType:  authGraphAPI,
		SenderSettings:      pro.SmtpSenderSettings{EmailAddress: "s@example.com"},
		GraphApiCredentials: &pro.SmtpGraphApiCredentials{ClientID: "cid", TenantID: "tid"},
	}

	// Same pointer for state and prior — exactly how CRUD calls it.
	if diags := assignSmtpServerResourceModel(&m, resp, &m); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.GraphAPICredentials == nil {
		t.Fatal("graph creds nil after assign")
	}
	if m.GraphAPICredentials.ClientSecretWoVersion.ValueInt64() != 7 {
		t.Errorf("wo_version must survive aliased prior; got %v", m.GraphAPICredentials.ClientSecretWoVersion)
	}
	if !m.GraphAPICredentials.ClientSecret.IsNull() {
		t.Error("WriteOnly client_secret must be null in state")
	}
}

// TestAssignState_GatesForeignBlockEcho is the regression test for the advisor's
// finding: even if the server echoes a stale foreign block, the state builder
// must null it because the active authentication_type forbids it. Otherwise a
// "was null, but now object" inconsistency fires after apply.
func TestAssignState_GatesForeignBlockEcho(t *testing.T) {
	resp := &pro.SmtpServerV2{
		AuthenticationType: authGraphAPI,
		Enabled:            true,
		SenderSettings:     pro.SmtpSenderSettings{EmailAddress: "s@example.com"},
		// Server (hypothetically) echoes a stale connection + basic block:
		ConnectionSettings:   &pro.SmtpConnectionSettings{Host: "stale", Port: 25, EncryptionType: "NONE", ConnectionTimeout: 30},
		BasicAuthCredentials: &pro.SmtpBasicCredentials{Username: "stale"},
		GraphApiCredentials:  &pro.SmtpGraphApiCredentials{ClientID: "cid", TenantID: "tid"},
	}

	var state SmtpServerResourceModel
	if diags := assignSmtpServerResourceModel(&state, resp, nil); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if state.ConnectionSettings != nil {
		t.Error("connection_settings must be nil in GRAPH_API mode even if server echoes it")
	}
	if state.BasicAuthCredentials != nil {
		t.Error("basic_auth_credentials must be nil in GRAPH_API mode even if server echoes it")
	}
	if state.GraphAPICredentials == nil || state.GraphAPICredentials.ClientID.ValueString() != "cid" {
		t.Errorf("graph creds wrong: %+v", state.GraphAPICredentials)
	}
	if !state.GraphAPICredentials.ClientSecret.IsNull() {
		t.Error("WriteOnly client_secret must be null in state")
	}
}

func TestAssignState_GoogleAuthenticationsList(t *testing.T) {
	auths := []pro.SmtpGoogleMailAuthentication{
		{EmailAddress: "a@example.com", Status: "AUTHENTICATED"},
		{EmailAddress: "b@example.com", Status: "UNAUTHENTICATED"},
	}
	resp := &pro.SmtpServerV2{
		AuthenticationType:    authGoogleMail,
		Enabled:               true,
		SenderSettings:        pro.SmtpSenderSettings{EmailAddress: "s@example.com"},
		GoogleMailCredentials: &pro.SmtpGoogleMailCredentials{ClientID: "cid", Authentications: &auths},
	}

	var state SmtpServerResourceModel
	if diags := assignSmtpServerResourceModel(&state, resp, nil); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.GoogleMailCredentials == nil {
		t.Fatal("google creds missing")
	}
	if state.GoogleMailCredentials.Authentications.IsNull() {
		t.Fatal("authentications list must not be null")
	}
	if got := len(state.GoogleMailCredentials.Authentications.Elements()); got != 2 {
		t.Errorf("authentications len = %d, want 2", got)
	}
}

func TestAssignState_EmptyAuthenticationsIsEmptyNotNull(t *testing.T) {
	resp := &pro.SmtpServerV2{
		AuthenticationType:    authGoogleMail,
		SenderSettings:        pro.SmtpSenderSettings{EmailAddress: "s@example.com"},
		GoogleMailCredentials: &pro.SmtpGoogleMailCredentials{ClientID: "cid", Authentications: nil},
	}
	var state SmtpServerResourceModel
	if diags := assignSmtpServerResourceModel(&state, resp, nil); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	a := state.GoogleMailCredentials.Authentications
	if a.IsNull() {
		t.Error("nil authentications must yield empty (non-null) list")
	}
	if got := len(a.Elements()); got != 0 {
		t.Errorf("empty authentications len = %d, want 0", got)
	}
}
