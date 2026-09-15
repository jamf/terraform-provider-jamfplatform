// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smtp_server

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignSmtpServerResourceModel folds a GET response into the resource model.
//
// Block population is gated by the returned authenticationType, NOT by raw wire
// presence: the connection / credential blocks are Optional-only (not Computed),
// so a block the active mode forbids must be null in state to match the
// validated config (plan). The server is expected to null the foreign blocks
// (wire-probed for NONE→GRAPH and BASIC→NONE transitions), but gating on the
// discriminator is strictly safer — a stale block echoed in a foreign mode would
// otherwise trip "was null, but now object" after apply. WriteOnly secrets are
// never returned and are set null; each `*_wo_version` rotation trigger is
// carried from the prior model (`prior`) so a refresh does not null it out.
func assignSmtpServerResourceModel(state *SmtpServerResourceModel, s *pro.SmtpServerV2, prior *SmtpServerResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	// Capture the rotation triggers from `prior` BEFORE mutating state. Callers
	// pass the same pointer for `state` and `prior` (the plan is read back into
	// itself), so a `state.XCredentials = nil` below would null `prior`'s block
	// out from under a later read — dropping wo_version to null in state and
	// tripping "inconsistent result after apply" (masked to the sensitive
	// credential block). Read them up front.
	priorBasicWo := priorBasicWoVersion(prior)
	priorGraphWo := priorGraphWoVersion(prior)
	priorGoogleWo := priorGoogleWoVersion(prior)

	state.Enabled = types.BoolValue(s.Enabled)
	state.AuthenticationType = types.StringValue(s.AuthenticationType)

	state.SenderSettings = &smtpSenderSettingsModel{
		EmailAddress: types.StringValue(s.SenderSettings.EmailAddress),
		DisplayName:  stringPtrValueOrNull(s.SenderSettings.DisplayName),
	}

	authType := s.AuthenticationType

	state.ConnectionSettings = nil
	if (authType == authNone || authType == authBasic) && s.ConnectionSettings != nil {
		state.ConnectionSettings = &smtpConnectionSettingsModel{
			Host:              types.StringValue(s.ConnectionSettings.Host),
			Port:              types.Int64Value(int64(s.ConnectionSettings.Port)),
			EncryptionType:    types.StringValue(s.ConnectionSettings.EncryptionType),
			ConnectionTimeout: types.Int64Value(int64(s.ConnectionSettings.ConnectionTimeout)),
		}
	}

	state.BasicAuthCredentials = nil
	if authType == authBasic && s.BasicAuthCredentials != nil {
		state.BasicAuthCredentials = &smtpBasicCredentialsModel{
			Username:          types.StringValue(s.BasicAuthCredentials.Username),
			Password:          types.StringNull(),
			PasswordWoVersion: priorBasicWo,
		}
	}

	state.GraphAPICredentials = nil
	if authType == authGraphAPI && s.GraphApiCredentials != nil {
		state.GraphAPICredentials = &smtpGraphAPICredentialsModel{
			ClientID:              types.StringValue(s.GraphApiCredentials.ClientID),
			TenantID:              types.StringValue(s.GraphApiCredentials.TenantID),
			ClientSecret:          types.StringNull(),
			ClientSecretWoVersion: priorGraphWo,
		}
	}

	state.GoogleMailCredentials = nil
	if authType == authGoogleMail && s.GoogleMailCredentials != nil {
		authList, d := buildAuthenticationsList(s.GoogleMailCredentials.Authentications)
		diags.Append(d...)
		state.GoogleMailCredentials = &smtpGoogleMailCredentialsModel{
			ClientID:              types.StringValue(s.GoogleMailCredentials.ClientID),
			ClientSecret:          types.StringNull(),
			ClientSecretWoVersion: priorGoogleWo,
			Authentications:       authList,
		}
	}

	return diags
}

// assignSmtpServerDataSourceModel folds a GET response into the data source
// model (server-readable fields only — no secrets, no rotation triggers).
// Block population is gated by authenticationType, matching the resource
// assigner.
func assignSmtpServerDataSourceModel(state *SmtpServerDataSourceModel, s *pro.SmtpServerV2) diag.Diagnostics {
	var diags diag.Diagnostics

	state.Enabled = types.BoolValue(s.Enabled)
	state.AuthenticationType = types.StringValue(s.AuthenticationType)

	state.SenderSettings = &smtpSenderSettingsModel{
		EmailAddress: types.StringValue(s.SenderSettings.EmailAddress),
		DisplayName:  stringPtrValueOrNull(s.SenderSettings.DisplayName),
	}

	authType := s.AuthenticationType

	state.ConnectionSettings = nil
	if (authType == authNone || authType == authBasic) && s.ConnectionSettings != nil {
		state.ConnectionSettings = &smtpConnectionSettingsModel{
			Host:              types.StringValue(s.ConnectionSettings.Host),
			Port:              types.Int64Value(int64(s.ConnectionSettings.Port)),
			EncryptionType:    types.StringValue(s.ConnectionSettings.EncryptionType),
			ConnectionTimeout: types.Int64Value(int64(s.ConnectionSettings.ConnectionTimeout)),
		}
	}

	state.BasicAuthCredentials = nil
	if authType == authBasic && s.BasicAuthCredentials != nil {
		state.BasicAuthCredentials = &smtpBasicCredentialsDSModel{
			Username: types.StringValue(s.BasicAuthCredentials.Username),
		}
	}

	state.GraphAPICredentials = nil
	if authType == authGraphAPI && s.GraphApiCredentials != nil {
		state.GraphAPICredentials = &smtpGraphAPICredentialsDSModel{
			ClientID: types.StringValue(s.GraphApiCredentials.ClientID),
			TenantID: types.StringValue(s.GraphApiCredentials.TenantID),
		}
	}

	state.GoogleMailCredentials = nil
	if authType == authGoogleMail && s.GoogleMailCredentials != nil {
		authList, d := buildAuthenticationsList(s.GoogleMailCredentials.Authentications)
		diags.Append(d...)
		state.GoogleMailCredentials = &smtpGoogleMailCredentialsDSModel{
			ClientID:        types.StringValue(s.GoogleMailCredentials.ClientID),
			Authentications: authList,
		}
	}

	return diags
}

// buildAuthenticationsList converts the server's OAuth-grant list into a
// Computed types.List of objects. A nil/empty list yields an empty (non-null)
// list so the Computed attribute is always known.
func buildAuthenticationsList(in *[]pro.SmtpGoogleMailAuthentication) (types.List, diag.Diagnostics) {
	if in == nil || len(*in) == 0 {
		return types.ListValueMust(authenticationsListType.ElemType, []attr.Value{}), nil
	}
	elems := make([]attr.Value, 0, len(*in))
	var diags diag.Diagnostics
	for _, a := range *in {
		obj, d := types.ObjectValue(googleAuthenticationAttrTypes, map[string]attr.Value{
			"email_address": types.StringValue(a.EmailAddress),
			"status":        types.StringValue(a.Status),
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	list, d := types.ListValue(authenticationsListType.ElemType, elems)
	diags.Append(d...)
	return list, diags
}

// prior*WoVersion safely extract the rotation trigger from the prior model so a
// refresh preserves it (the server never returns the secret or its version).
func priorBasicWoVersion(prior *SmtpServerResourceModel) types.Int64 {
	if prior == nil || prior.BasicAuthCredentials == nil {
		return types.Int64Null()
	}
	return prior.BasicAuthCredentials.PasswordWoVersion
}

func priorGraphWoVersion(prior *SmtpServerResourceModel) types.Int64 {
	if prior == nil || prior.GraphAPICredentials == nil {
		return types.Int64Null()
	}
	return prior.GraphAPICredentials.ClientSecretWoVersion
}

func priorGoogleWoVersion(prior *SmtpServerResourceModel) types.Int64 {
	if prior == nil || prior.GoogleMailCredentials == nil {
		return types.Int64Null()
	}
	return prior.GoogleMailCredentials.ClientSecretWoVersion
}

// stringPtrValueOrNull converts a *string into a TF String, mapping nil to null
// and preserving a non-nil empty string as "".
func stringPtrValueOrNull(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}
