// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smtp_server

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func basicPlan(secretWoVersion types.Int64) SmtpServerResourceModel {
	return SmtpServerResourceModel{
		AuthenticationType: types.StringValue(authBasic),
		Enabled:            types.BoolValue(true),
		SenderSettings: &smtpSenderSettingsModel{
			EmailAddress: types.StringValue("sender@example.com"),
			DisplayName:  types.StringValue("Example"),
		},
		ConnectionSettings: &smtpConnectionSettingsModel{
			Host:              types.StringValue("smtp.example.com"),
			Port:              types.Int64Value(465),
			EncryptionType:    types.StringValue("SSL"),
			ConnectionTimeout: types.Int64Value(30),
		},
		BasicAuthCredentials: &smtpBasicCredentialsModel{
			Username:          types.StringValue("user@example.com"),
			Password:          types.StringValue("s3cret"),
			PasswordWoVersion: secretWoVersion,
		},
	}
}

func TestBuildInput_Basic(t *testing.T) {
	plan := basicPlan(types.Int64Value(1))
	out := buildSmtpServerInput(plan, nil, "s3cret")

	if out.AuthenticationType != authBasic {
		t.Errorf("authType = %q", out.AuthenticationType)
	}
	if !out.Enabled {
		t.Error("enabled should be true")
	}
	if out.ConnectionSettings == nil || out.ConnectionSettings.Host != "smtp.example.com" || out.ConnectionSettings.Port != 465 {
		t.Errorf("connection settings wrong: %+v", out.ConnectionSettings)
	}
	if out.BasicAuthCredentials == nil || out.BasicAuthCredentials.Username != "user@example.com" || out.BasicAuthCredentials.Password != "s3cret" {
		t.Errorf("basic creds wrong: %+v", out.BasicAuthCredentials)
	}
	if out.GraphApiCredentials != nil || out.GoogleMailCredentials != nil {
		t.Error("foreign credential blocks must be nil for BASIC")
	}
	if out.SenderSettings.DisplayName == nil || *out.SenderSettings.DisplayName != "Example" {
		t.Errorf("display name wrong: %v", out.SenderSettings.DisplayName)
	}
}

func TestBuildInput_None_NoCredentialBlocks(t *testing.T) {
	plan := SmtpServerResourceModel{
		AuthenticationType: types.StringValue(authNone),
		Enabled:            types.BoolValue(false),
		SenderSettings:     &smtpSenderSettingsModel{EmailAddress: types.StringValue("s@example.com"), DisplayName: types.StringNull()},
		ConnectionSettings: &smtpConnectionSettingsModel{Host: types.StringValue("h"), Port: types.Int64Value(25), EncryptionType: types.StringValue("NONE"), ConnectionTimeout: types.Int64Value(30)},
	}
	out := buildSmtpServerInput(plan, nil, "")
	if out.ConnectionSettings == nil {
		t.Error("NONE must send connection_settings")
	}
	if out.BasicAuthCredentials != nil || out.GraphApiCredentials != nil || out.GoogleMailCredentials != nil {
		t.Error("NONE must send no credential blocks")
	}
}

func TestBuildInput_Graph(t *testing.T) {
	plan := SmtpServerResourceModel{
		AuthenticationType: types.StringValue(authGraphAPI),
		Enabled:            types.BoolValue(true),
		SenderSettings:     &smtpSenderSettingsModel{EmailAddress: types.StringValue("s@example.com"), DisplayName: types.StringNull()},
		GraphAPICredentials: &smtpGraphAPICredentialsModel{
			ClientID: types.StringValue("cid"),
			TenantID: types.StringValue("tid"),
		},
	}
	out := buildSmtpServerInput(plan, nil, "gsecret")
	if out.ConnectionSettings != nil {
		t.Error("GRAPH_API must not send connection_settings")
	}
	if out.GraphApiCredentials == nil || out.GraphApiCredentials.ClientID != "cid" || out.GraphApiCredentials.TenantID != "tid" || out.GraphApiCredentials.ClientSecret != "gsecret" {
		t.Errorf("graph creds wrong: %+v", out.GraphApiCredentials)
	}
}

func TestBuildInput_Google_NoAuthenticationsSent(t *testing.T) {
	plan := SmtpServerResourceModel{
		AuthenticationType: types.StringValue(authGoogleMail),
		Enabled:            types.BoolValue(true),
		SenderSettings:     &smtpSenderSettingsModel{EmailAddress: types.StringValue("s@example.com"), DisplayName: types.StringNull()},
		GoogleMailCredentials: &smtpGoogleMailCredentialsModel{
			ClientID:        types.StringValue("cid"),
			Authentications: types.ListNull(authenticationsListType.ElemType),
		},
	}
	out := buildSmtpServerInput(plan, nil, "gsecret")
	if out.GoogleMailCredentials == nil || out.GoogleMailCredentials.ClientSecret != "gsecret" {
		t.Errorf("google creds wrong: %+v", out.GoogleMailCredentials)
	}
	if out.GoogleMailCredentials.Authentications != nil {
		t.Error("authentications must never be sent on write")
	}
}

func TestBoolOrCurrent_AdoptsCurrentWhenPlanUnknown(t *testing.T) {
	current := &pro.SmtpServerV2{Enabled: true}
	if got := boolOrCurrent(types.BoolUnknown(), current); !got {
		t.Error("unknown plan + current.Enabled=true should adopt true")
	}
	if got := boolOrCurrent(types.BoolNull(), current); !got {
		t.Error("null plan + current.Enabled=true should adopt true")
	}
	if got := boolOrCurrent(types.BoolValue(false), current); got {
		t.Error("known plan false must win over current true")
	}
	if got := boolOrCurrent(types.BoolUnknown(), nil); got {
		t.Error("unknown plan + nil current should default false")
	}
}

func TestBuildSender_AdoptsCurrentDisplayName(t *testing.T) {
	cur := "Existing Name"
	current := &pro.SmtpServerV2{SenderSettings: pro.SmtpSenderSettings{DisplayName: &cur}}
	m := &smtpSenderSettingsModel{EmailAddress: types.StringValue("s@example.com"), DisplayName: types.StringNull()}
	out := buildSenderSettings(m, current)
	if out.DisplayName == nil || *out.DisplayName != "Existing Name" {
		t.Errorf("display name should adopt current; got %v", out.DisplayName)
	}
}

func TestCreateSecret_ReturnsConfigSecretForActiveMode(t *testing.T) {
	cfg := basicPlan(types.Int64Value(1))
	if got := createSecret(cfg); got != "s3cret" {
		t.Errorf("createSecret = %q, want s3cret", got)
	}
	// Wrong/absent block → "".
	none := SmtpServerResourceModel{AuthenticationType: types.StringValue(authNone)}
	if got := createSecret(none); got != "" {
		t.Errorf("createSecret(NONE) = %q, want empty", got)
	}
}

func TestUpdateSecret_GatedByWoVersion(t *testing.T) {
	plan := basicPlan(types.Int64Value(2))
	state := basicPlan(types.Int64Value(1))
	// rotated (2 != 1) → secret sent.
	if got := updateSecret(plan, plan, state); got != "s3cret" {
		t.Errorf("rotated updateSecret = %q, want s3cret", got)
	}
	// not rotated (1 == 1) → "".
	state2 := basicPlan(types.Int64Value(2))
	if got := updateSecret(plan, plan, state2); got != "" {
		t.Errorf("unrotated updateSecret = %q, want empty", got)
	}
}
