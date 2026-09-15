// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// hasCertificateData reports whether the config supplies certificate material —
// the discriminator between the import (upload) and generate create paths. The
// value is WriteOnly, so it must be read from req.Config (it is null in plan).
func hasCertificateData(cfg SupervisionIdentityResourceModel) bool {
	return !cfg.CertificateData.IsNull() && !cfg.CertificateData.IsUnknown() && strings.TrimSpace(cfg.CertificateData.ValueString()) != ""
}

// buildGenerateInput builds the payload for the generate path (no certificate
// supplied). Jamf Pro mints a new self-signed identity protected by password.
//
// The password is trimmed defensively: a trailing newline does not error but
// silently corrupts the minted identity (empty common name, epoch expiration —
// wire-probed 2026-06-12).
func buildGenerateInput(plan, cfg SupervisionIdentityResourceModel) *pro.SupervisionIdentityCreate {
	return &pro.SupervisionIdentityCreate{
		DisplayName: plan.DisplayName.ValueString(),
		Password:    strings.TrimSpace(cfg.Password.ValueString()),
	}
}

// buildUploadInput builds the payload for the import path (certificate supplied).
// certificate_data arrives as a base64 string; it is decoded to raw bytes and the
// SDK's JSON encoder re-encodes it to base64 on the wire, round-tripping the
// user's filebase64() input. The password is trimmed defensively (see above).
func buildUploadInput(plan, cfg SupervisionIdentityResourceModel) (*pro.SupervisionIdentityCertificateUpload, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.CertificateData.ValueString()))
	if err != nil {
		return nil, fmt.Errorf("certificate_data is not valid base64 (use filebase64(\"identity.p12\")): %w", err)
	}
	return &pro.SupervisionIdentityCertificateUpload{
		DisplayName:     plan.DisplayName.ValueString(),
		Password:        strings.TrimSpace(cfg.Password.ValueString()),
		CertificateData: &raw,
	}, nil
}

// buildUpdateInput builds the rename-only update payload. display_name is the
// only mutable field; the secrets are never part of an update.
func buildUpdateInput(plan SupervisionIdentityResourceModel) *pro.SupervisionIdentityUpdate {
	return &pro.SupervisionIdentityUpdate{
		DisplayName: plan.DisplayName.ValueString(),
	}
}
