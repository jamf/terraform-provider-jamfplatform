// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// keyTypeToWire returns the wire `key_type` value to send on Create /
// Update. The schema validator (stringvalidator.OneOf) enforces the
// read-canonical spellings at plan time, but the classic POST/PUT
// endpoint asymmetrically demands the Title-Case
// "Individual And Institutional" form for the combined type — sending
// the lowercase read-form returns HTTP 409 "Problem with key type".
// keyTypeWriteAlias translates one-way at the input boundary so users
// only see the read-canonical lowercase form. Null / unknown stays
// nil so the SDK omits the field — preserving the server's stored
// value under Classic's partial-merge semantics.
func keyTypeToWire(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := keyTypeWriteAlias(v.ValueString())
	return &out
}

// buildDiskEncryptionConfigurationInput converts the Terraform plan model
// into the SDK payload used for both Create and Update.
//
// Wire-quirk handling (audit reference §2.5–§2.10):
//
//   - `Key` is never emitted on writes. The server derives Subject DN from
//     the uploaded `Data`; the schema marks `key` Computed-only.
//   - `Password` is `WriteOnly`; the caller (Create/Update handler) passes
//     in the cfg-sourced plaintext only when the user bumped
//     `password_wo_version`. Otherwise the caller passes nil so this
//     builder omits the field; under Classic's partial-merge semantics
//     the omitted field preserves the server's stored value (audit §2.10
//     confirms password is independently writable). The server-side
//     `password_sha256` redaction sentinel is no longer surfaced (it
//     returns the literal 20-asterisk string, not a real hash).
//
// `ID` is omitted on write — Create uses path id="0" and Update derives ID
// from state.
func buildDiskEncryptionConfigurationInput(plan DiskEncryptionConfigurationResourceModel, password *string) *proclassic.DiskEncryptionConfiguration {
	return &proclassic.DiskEncryptionConfiguration{
		Name:                     helpers.OptionalStringPointer(plan.Name),
		KeyType:                  keyTypeToWire(plan.KeyType),
		FileVaultEnabledUsers:    helpers.OptionalStringPointer(plan.FileVaultEnabledUsers),
		InstitutionalRecoveryKey: buildIRKInput(plan.InstitutionalRecoveryKey, password),
	}
}

// buildIRKInput converts the TF institutional_recovery_key block into the
// SDK shape. Returns nil when the user omits the block — the SDK's
// omitempty drops `<institutional_recovery_key>` from the wire so the
// server preserves stored values under Classic's partial-merge semantics
// (audit §2.6, §2.7).
//
// `Key` is never emitted on writes — it is server-derived from the
// uploaded `Data` (cert Subject DN). `PasswordSha256` is never emitted
// (the server's read value is a redaction sentinel, not a real hash;
// echoing it back would just confuse the server).
//
// `CertificateType` IS emitted on writes — the classic POST endpoint
// rejects an IRK block without it: `Certificate type is required if a
// recovery key is specified`. The schema marks it Required so the value
// is always present when the IRK block is supplied.
func buildIRKInput(m *diskEncryptionConfigurationIRKModel, password *string) *proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey {
	if m == nil {
		return nil
	}
	return &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
		CertificateType: helpers.OptionalStringPointer(m.CertificateType),
		Password:        password,
		Data:            helpers.OptionalStringPointer(m.Data),
	}
}
