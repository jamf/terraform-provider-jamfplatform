// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildCloudDistributionPointInput converts the Terraform plan (plus the
// Config-sourced WriteOnly secrets) into the SDK payload used for both Create
// (POST) and Update (PATCH).
//
// `cdnType`, `username`, and `password` are API-required on every write
// (OpenAPI `required`), so they are always emitted. The SDK models `Username`
// and `Password` as plain `string`s — a null/unknown TF value sends an empty
// string, which the server accepts (this is the JAMF_CLOUD shape).
//
// `password` and `privateKey` are WriteOnly: their plaintext is sourced from
// req.Config (req.Plan exposes them as null). The caller threads the Config
// model in via `cfg`.
//
// All other fields are SDK pointers with `omitempty`; helpers.Optional*Pointer
// returns nil for null/unknown so the field is dropped from the wire and the
// server keeps / derives its own value.
func buildCloudDistributionPointInput(plan, cfg CloudDistributionPointResourceModel) (*pro.CloudDistributionPoint, error) {
	privateKey, err := decodePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &pro.CloudDistributionPoint{
		CdnType:                 plan.CdnType.ValueString(),
		Master:                  helpers.OptionalBoolPointer(plan.Master),
		Username:                plan.Username.ValueString(),
		Password:                cfg.Password.ValueString(),
		Directory:               helpers.OptionalStringPointer(plan.Directory),
		UploadURL:               helpers.OptionalStringPointer(plan.UploadURL),
		DownloadURL:             helpers.OptionalStringPointer(plan.DownloadURL),
		RequireSignedUrls:       helpers.OptionalBoolPointer(plan.RequireSignedURLs),
		KeyPairID:               helpers.OptionalStringPointer(plan.KeyPairID),
		PrivateKey:              privateKey,
		ExpirationSeconds:       helpers.OptionalInt64Pointer(plan.ExpirationSeconds),
		SecondaryAuthRequired:   helpers.OptionalBoolPointer(plan.SecondaryAuthRequired),
		SecondaryAuthTimeToLive: helpers.OptionalInt64Pointer(plan.SecondaryAuthTimeToLive),
	}, nil
}

// decodePrivateKey decodes the base64 WriteOnly private_key into the SDK's
// *[]byte. Returns nil (field omitted on the wire) when unset.
func decodePrivateKey(v types.String) (*[]byte, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	raw := strings.TrimSpace(v.ValueString())
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("private_key must be valid base64: %w", err)
	}
	return &decoded, nil
}
