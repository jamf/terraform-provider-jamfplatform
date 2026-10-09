// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DiskEncryptionConfigurationResourceModel is the Terraform resource model
// for a Jamf Pro disk encryption configuration. The Classic
// /diskencryptionconfigurations endpoint returns a flat envelope (id,
// name, key_type, file_vault_enabled_users) plus an
// institutional_recovery_key nested block. The wire encoding is
// described in `local-testing/diskencryption/AUDIT_FINDINGS.md`.
//
// `InstitutionalRecoveryKey` is modelled as a typed pointer
// (`*diskEncryptionConfigurationIRKModel`) — per STYLE_GUIDE the
// SingleNestedAttribute must stay Optional-only because the framework
// cannot fit an Unknown value into a typed pointer.
type DiskEncryptionConfigurationResourceModel struct {
	ID                       types.String                         `tfsdk:"id"`
	Name                     types.String                         `tfsdk:"name"`
	KeyType                  types.String                         `tfsdk:"key_type"`
	FileVaultEnabledUsers    types.String                         `tfsdk:"file_vault_enabled_users"`
	InstitutionalRecoveryKey *diskEncryptionConfigurationIRKModel `tfsdk:"institutional_recovery_key"`
	Timeouts                 resourceTimeouts.Value               `tfsdk:"timeouts"`
}

// DiskEncryptionConfigurationDataSourceModel is the Terraform data source
// model. Mirrors the resource shape minus the write-only `password` field
// (data sources are read-only — no user-supplied plaintext to preserve).
type DiskEncryptionConfigurationDataSourceModel struct {
	ID                       types.String                                   `tfsdk:"id"`
	Name                     types.String                                   `tfsdk:"name"`
	KeyType                  types.String                                   `tfsdk:"key_type"`
	FileVaultEnabledUsers    types.String                                   `tfsdk:"file_vault_enabled_users"`
	InstitutionalRecoveryKey *diskEncryptionConfigurationIRKDataSourceModel `tfsdk:"institutional_recovery_key"`
	Timeouts                 datasourceTimeouts.Value                       `tfsdk:"timeouts"`
}

// diskEncryptionConfigurationIRKModel is the nested model for
// institutional_recovery_key on the resource. Pointer-shaped so the
// SingleNestedAttribute can stay Optional-only (per STYLE_GUIDE typed-pointer
// rule).
//
// Wire-quirk reference (audit §1, §2.9, §2.10):
//
//   - `Key` is server-derived from the cert Subject DN — Computed.
//   - `CertificateType` is server-determined (PKCS12 / DER / PEM) — Computed.
//   - `Password` is `WriteOnly` plaintext — sent on writes, never persisted
//     in state, never echoed on read.
//   - `PasswordWoVersion` is the rotation trigger companion — bumping the
//     integer forces the next Update to re-send `Password` to Jamf Pro.
//   - `Data` is base64 of the recovery cert (PKCS12, DER, or PEM).
type diskEncryptionConfigurationIRKModel struct {
	Key               types.String `tfsdk:"key"`
	CertificateType   types.String `tfsdk:"certificate_type"`
	Password          types.String `tfsdk:"password"`
	PasswordWoVersion types.Int64  `tfsdk:"password_wo_version"`
	Data              types.String `tfsdk:"data"`
}

// diskEncryptionConfigurationIRKDataSourceModel is the nested model for the
// data source. The data source omits the write-only `password` attribute —
// the wire never returns the plaintext on read.
type diskEncryptionConfigurationIRKDataSourceModel struct {
	Key             types.String `tfsdk:"key"`
	CertificateType types.String `tfsdk:"certificate_type"`
	Data            types.String `tfsdk:"data"`
}

// diskEncryptionConfigurationIdentityModel is the identity object for
// resource imports and list-resource identities.
type diskEncryptionConfigurationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DiskEncryptionConfigurationListResourceModel is the config model for the
// list resource. Classic /diskencryptionconfigurations has no RSQL, so
// the filter shape reuses the shared client-side substring block.
type DiskEncryptionConfigurationListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
