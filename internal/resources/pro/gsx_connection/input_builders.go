// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package gsx_connection

import (
	"encoding/base64"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildGsxConnectionInput converts the Terraform plan + config into an SDK payload for the
// full-replace PUT. The GSX PUT mandates token + keystore on every write (wire-probed
// 2026-06-09), so this always emits all three secrets read from req.Config (WriteOnly
// strips them from plan/state). There is no rotation gate.
//
// Non-secret fields:
//   - username / service_account_number are Required, so always taken from the plan.
//   - enabled / ship_to_number / keystore_name are Optional+Computed with
//     UseStateForUnknown: on update an omitted value is a known prior value (carried by
//     USFU). On first create there is no prior state, so the `current` merge base — the
//     live settings read in Create — supplies the value for any omitted field, so the
//     singleton is adopted rather than reset. On update `current` is nil.
//
// `keystore_bytes_wo` is base64-decoded into the SDK's `*[]byte`; Go's JSON encoder
// re-encodes it to base64 on the wire, round-tripping the user's filebase64() input.
func buildGsxConnectionInput(plan, cfg GsxConnectionSettingsResourceModel, current *pro.GsxConnection) (*pro.GsxConnection, error) {
	keystoreBytes, err := base64.StdEncoding.DecodeString(cfg.KeystoreBytesWo.ValueString())
	if err != nil {
		return nil, fmt.Errorf("keystore_bytes_wo is not valid base64 (use filebase64(\"certificate.p12\")): %w", err)
	}

	out := &pro.GsxConnection{
		Enabled:          boolOrCurrent(plan.Enabled, currentBool(current, func(c *pro.GsxConnection) bool { return c.Enabled })),
		Username:         plan.Username.ValueString(),
		ServiceAccountNo: plan.ServiceAccountNumber.ValueString(),
		ShipToNo:         shipToOrCurrent(plan.ShipToNumber, current),
		Token:            cfg.TokenWo.ValueString(),
		GsxKeystore: pro.GsxKeystore{
			KeystoreBytes:    &keystoreBytes,
			KeystorePassword: cfg.KeystorePasswordWo.ValueString(),
			Name:             keystoreNameOrCurrent(plan.KeystoreName, current),
		},
	}
	return out, nil
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried on update),
// else falls back to the live value read from the server (preserve undeclared values on
// first create).
func boolOrCurrent(v types.Bool, current bool) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	return current
}

// currentBool safely extracts a bool field from a possibly-nil current read.
func currentBool(current *pro.GsxConnection, get func(*pro.GsxConnection) bool) bool {
	if current == nil {
		return false
	}
	return get(current)
}

// shipToOrCurrent resolves the ship-to number, preferring the plan, then the current read.
// An empty result yields a nil pointer (omitted on the wire — the field is optional).
func shipToOrCurrent(v types.String, current *pro.GsxConnection) *string {
	resolved := ""
	switch {
	case !v.IsNull() && !v.IsUnknown():
		resolved = v.ValueString()
	case current != nil && current.ShipToNo != nil:
		resolved = *current.ShipToNo
	}
	if resolved == "" {
		return nil
	}
	return &resolved
}

// keystoreNameOrCurrent resolves the keystore name, preferring the plan, then the current
// read. The SDK field is a non-pointer string, so an unset value is emitted as "".
func keystoreNameOrCurrent(v types.String, current *pro.GsxConnection) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	if current != nil {
		return current.GsxKeystore.Name
	}
	return ""
}
