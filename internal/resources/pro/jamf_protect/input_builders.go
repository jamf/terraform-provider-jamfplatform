// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_protect

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildJamfProtectRegistrationInput converts the Terraform plan (plus the
// Config-sourced WriteOnly password) into the SDK register payload.
//
// `password` is WriteOnly: its plaintext is sourced from req.Config (req.Plan
// exposes it as null). The caller threads the Config model in via `cfg`. The
// values are sent verbatim — the server echoes `protectUrl` back unchanged
// and validates the credentials live against the Protect instance, so no
// client-side normalisation is applied.
func buildJamfProtectRegistrationInput(plan, cfg JamfProtectResourceModel) *pro.ProtectRegistrationRequest {
	return &pro.ProtectRegistrationRequest{
		ClientID:   plan.ClientID.ValueString(),
		Password:   cfg.Password.ValueString(),
		ProtectURL: plan.APIURL.ValueString(),
	}
}

// buildJamfProtectSettingsInput converts the planned auto_install value into
// the SDK PUT payload. The request carries only AutoInstall — the server
// rejects every other settings field as read-only. Returns nil when the
// planned value is null/unknown (nothing to assert).
func buildJamfProtectSettingsInput(autoInstall *bool) *pro.ProtectUpdatableSettingsRequest {
	if autoInstall == nil {
		return nil
	}
	return &pro.ProtectUpdatableSettingsRequest{AutoInstall: autoInstall}
}
