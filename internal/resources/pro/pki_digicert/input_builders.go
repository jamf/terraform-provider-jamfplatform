// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_digicert

import (
	"encoding/base64"
	"fmt"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildDigicertInput converts the Terraform plan + config into an SDK
// DigiCertSetting for POST (create) or PATCH (update).
//
// DigiCert applies merge-patch semantics on update (omit = preserve), so scalar
// plan fields that are null/unknown are omitted (left as nil pointers). On create
// the scalars are always known (Optional+Computed; absent ones resolve via the
// GET-after read), so they are sent as supplied.
//
// The client certificate is all-or-nothing. It is included only when
// includeCert is true (create with the block set, or update when wo_version
// changed). When included, the WriteOnly bytes/password are read from the config
// model (`cfg`) — the framework strips WriteOnly values from plan/state — and the
// base64 data_wo is decoded into the SDK's raw []byte field (encoding/json
// re-encodes it to base64 on the wire).
func buildDigicertInput(plan, cfg DigicertResourceModel, includeCert bool) (*pro.DigiCertSetting, error) {
	out := &pro.DigiCertSetting{}

	if !plan.DisplayName.IsNull() && !plan.DisplayName.IsUnknown() {
		out.CaName = plan.DisplayName.ValueStringPointer()
	}
	if !plan.HostName.IsNull() && !plan.HostName.IsUnknown() {
		out.Fqdn = plan.HostName.ValueStringPointer()
	}
	if !plan.RevocationEnabled.IsNull() && !plan.RevocationEnabled.IsUnknown() {
		out.RevocationEnabled = plan.RevocationEnabled.ValueBoolPointer()
	}

	if includeCert {
		if cfg.ClientCertificate == nil {
			return nil, fmt.Errorf("client_certificate block must be set to send a certificate")
		}
		data, err := base64.StdEncoding.DecodeString(cfg.ClientCertificate.DataWo.ValueString())
		if err != nil {
			return nil, fmt.Errorf("client_certificate.data_wo is not valid base64 (use filebase64(\"cert.p12\")): %w", err)
		}
		cert := &pro.Certificate{
			Data:     data,
			Filename: plan.ClientCertificate.Filename.ValueString(),
		}
		if pw := cfg.ClientCertificate.PasswordWo; !pw.IsNull() && !pw.IsUnknown() {
			cert.Password = pw.ValueStringPointer()
		}
		out.ClientCert = cert
	}

	return out, nil
}

// shouldRotateCert reports whether the client certificate should be re-sent on
// update: true when the plan block is present and its wo_version differs from the
// prior state's. Guards a nil prior block (block newly added).
func shouldRotateCert(plan, state *DigicertClientCertModel) bool {
	if plan == nil {
		return false
	}
	if state == nil {
		return true
	}
	return !plan.WoVersion.Equal(state.WoVersion)
}
