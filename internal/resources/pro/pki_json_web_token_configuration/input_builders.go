// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildJSONWebTokenConfigurationInput projects a plan model into an SDK
// *proclassic.JsonWebTokenConfiguration for Create / Update. The classic PUT is
// a partial merge (omit = retain), so the builder emits only the fields the
// plan owns and drops null/unknown values.
//
// Two deliberate behaviours:
//
//   - `encryption_key_wo` is sourced separately (it is a WriteOnly attribute,
//     so the plan exposes it as null). The caller threads the non-nil plaintext
//     on every Create, and on Update only when `encryption_key_wo_version`
//     changed; otherwise nil omits <encryption_key> and the merge retains the
//     stored key (mirrors webhook / directory_binding).
//   - `enabled` inverts into the wire's `disabled` flag. The inversion lives
//     here and in the state assigner only — nothing else in the package knows
//     about the wire polarity.
func buildJSONWebTokenConfigurationInput(plan JSONWebTokenConfigurationResourceModel, encryptionKey *string) *proclassic.JsonWebTokenConfiguration {
	return &proclassic.JsonWebTokenConfiguration{
		Name:          helpers.OptionalStringPointer(plan.Name),
		EncryptionKey: encryptionKey,
		TokenExpiry:   helpers.OptionalInt64Pointer(plan.TokenExpiry),
		Disabled:      disabledFromEnabled(plan.Enabled),
	}
}

// disabledFromEnabled inverts the Terraform `enabled` attribute into the wire
// `disabled` flag. Null/unknown maps to nil so the element is omitted and the
// server default (or stored value, on update) applies.
func disabledFromEnabled(enabled types.Bool) *bool {
	if enabled.IsNull() || enabled.IsUnknown() {
		return nil
	}
	disabled := !enabled.ValueBool()
	return &disabled
}

// encryptionKeyForUpdate returns the plaintext encryption key to send on an
// update: the config value when the rotation trigger
// (`encryption_key_wo_version`) changed versus prior state, nil otherwise so
// the merge leaves the stored key untouched.
func encryptionKeyForUpdate(planVersion, stateVersion types.Int64, cfgKey types.String) *string {
	if planVersion.Equal(stateVersion) {
		return nil
	}
	return helpers.OptionalStringPointer(cfgKey)
}
