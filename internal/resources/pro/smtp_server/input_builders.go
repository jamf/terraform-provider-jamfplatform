// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smtp_server

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildSmtpServerInput converts the Terraform plan into an SDK SmtpServerV2
// payload. The SMTP Server API is a full-replace PUT (despite its
// merge-patch+json content type, wire-probed 2026-06-09): the server validates
// the complete body against authentication_type, rejecting a body that omits the
// fields required for that mode.
//
// Only the block matching authentication_type is sent; the foreign credential
// blocks are left nil so the server clears them. `enabled` and the sender
// display name are adopted from `current` (the live settings read on Create)
// when the user omitted them, so a first apply adopts existing values rather
// than resetting them; on Update `current` is nil — UseStateForUnknown has
// already carried omitted knowns into the plan.
//
// `secret` is the resolved WriteOnly secret for the active mode: the config
// value on Create (and on a wo_version rotation), or "" otherwise. An empty
// secret is sent verbatim; the server preserves the stored value when the secret
// is empty (wire-probed 2026-06-09).
func buildSmtpServerInput(plan SmtpServerResourceModel, current *pro.SmtpServerV2, secret string) *pro.SmtpServerV2 {
	out := &pro.SmtpServerV2{
		AuthenticationType: plan.AuthenticationType.ValueString(),
		Enabled:            boolOrCurrent(plan.Enabled, current),
		SenderSettings:     buildSenderSettings(plan.SenderSettings, current),
	}

	switch plan.AuthenticationType.ValueString() {
	case authNone:
		out.ConnectionSettings = buildConnectionSettings(plan.ConnectionSettings)
	case authBasic:
		out.ConnectionSettings = buildConnectionSettings(plan.ConnectionSettings)
		if plan.BasicAuthCredentials != nil {
			out.BasicAuthCredentials = &pro.SmtpBasicCredentials{
				Username: plan.BasicAuthCredentials.Username.ValueString(),
				Password: secret,
			}
		}
	case authGraphAPI:
		if plan.GraphAPICredentials != nil {
			out.GraphApiCredentials = &pro.SmtpGraphApiCredentials{
				ClientID:     plan.GraphAPICredentials.ClientID.ValueString(),
				TenantID:     plan.GraphAPICredentials.TenantID.ValueString(),
				ClientSecret: secret,
			}
		}
	case authGoogleMail:
		if plan.GoogleMailCredentials != nil {
			out.GoogleMailCredentials = &pro.SmtpGoogleMailCredentials{
				ClientID:     plan.GoogleMailCredentials.ClientID.ValueString(),
				ClientSecret: secret,
				// authentications is server-managed (out-of-band OAuth); never sent.
			}
		}
	}

	return out
}

// buildSenderSettings builds the required senderSettings block. display_name is
// optional: send the plan value when known, else adopt the current value on
// create, else omit (nil pointer).
func buildSenderSettings(m *smtpSenderSettingsModel, current *pro.SmtpServerV2) pro.SmtpSenderSettings {
	out := pro.SmtpSenderSettings{}
	if m != nil {
		out.EmailAddress = m.EmailAddress.ValueString()
		if !m.DisplayName.IsNull() && !m.DisplayName.IsUnknown() {
			v := m.DisplayName.ValueString()
			out.DisplayName = &v
			return out
		}
	}
	if current != nil && current.SenderSettings.DisplayName != nil {
		v := *current.SenderSettings.DisplayName
		out.DisplayName = &v
	}
	return out
}

// buildConnectionSettings builds the connectionSettings block, or nil when the
// block is absent.
func buildConnectionSettings(m *smtpConnectionSettingsModel) *pro.SmtpConnectionSettings {
	if m == nil {
		return nil
	}
	return &pro.SmtpConnectionSettings{
		Host:              m.Host.ValueString(),
		Port:              int(m.Port.ValueInt64()),
		EncryptionType:    m.EncryptionType.ValueString(),
		ConnectionTimeout: int(m.ConnectionTimeout.ValueInt64()),
	}
}

// boolOrCurrent returns the plan bool when known (declared, or USFU-carried on
// update), else the current live value (adopt undeclared `enabled` on first
// create), else false.
func boolOrCurrent(v types.Bool, current *pro.SmtpServerV2) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	if current != nil {
		return current.Enabled
	}
	return false
}

// createSecret returns the config WriteOnly secret for the active mode, sent
// unconditionally on Create (the server has no stored secret to preserve yet).
// Mirrors cloud_identity_provider.buildGoogleCreateRequest, which always
// includes the keystore on create.
func createSecret(cfg SmtpServerResourceModel) string {
	switch cfg.AuthenticationType.ValueString() {
	case authBasic:
		if cfg.BasicAuthCredentials == nil {
			return ""
		}
		return secretValue(cfg.BasicAuthCredentials.Password)
	case authGraphAPI:
		if cfg.GraphAPICredentials == nil {
			return ""
		}
		return secretValue(cfg.GraphAPICredentials.ClientSecret)
	case authGoogleMail:
		if cfg.GoogleMailCredentials == nil {
			return ""
		}
		return secretValue(cfg.GoogleMailCredentials.ClientSecret)
	}
	return ""
}

// updateSecret returns the WriteOnly secret to send for the active mode on
// Update: the config value only when the rotation trigger (wo_version) changed
// between state and plan, otherwise "" (the server preserves the stored secret
// on an empty value).
func updateSecret(cfg SmtpServerResourceModel, plan, state SmtpServerResourceModel) string {
	switch cfg.AuthenticationType.ValueString() {
	case authBasic:
		if cfg.BasicAuthCredentials == nil {
			return ""
		}
		return secretIfRotated(cfg.BasicAuthCredentials.Password, basicWoVersion(&plan), basicWoVersion(&state))
	case authGraphAPI:
		if cfg.GraphAPICredentials == nil {
			return ""
		}
		return secretIfRotated(cfg.GraphAPICredentials.ClientSecret, graphWoVersion(&plan), graphWoVersion(&state))
	case authGoogleMail:
		if cfg.GoogleMailCredentials == nil {
			return ""
		}
		return secretIfRotated(cfg.GoogleMailCredentials.ClientSecret, googleWoVersion(&plan), googleWoVersion(&state))
	}
	return ""
}

// secretValue returns the secret string, or "" when null/unknown.
func secretValue(secret types.String) string {
	if secret.IsNull() || secret.IsUnknown() {
		return ""
	}
	return secret.ValueString()
}

// secretIfRotated returns the config secret only when the rotation trigger
// changed; otherwise "".
func secretIfRotated(secret types.String, planVersion, stateVersion types.Int64) string {
	if planVersion.Equal(stateVersion) {
		return ""
	}
	return secretValue(secret)
}

func basicWoVersion(m *SmtpServerResourceModel) types.Int64 {
	if m.BasicAuthCredentials == nil {
		return types.Int64Null()
	}
	return m.BasicAuthCredentials.PasswordWoVersion
}

func graphWoVersion(m *SmtpServerResourceModel) types.Int64 {
	if m.GraphAPICredentials == nil {
		return types.Int64Null()
	}
	return m.GraphAPICredentials.ClientSecretWoVersion
}

func googleWoVersion(m *SmtpServerResourceModel) types.Int64 {
	if m.GoogleMailCredentials == nil {
		return types.Int64Null()
	}
	return m.GoogleMailCredentials.ClientSecretWoVersion
}
