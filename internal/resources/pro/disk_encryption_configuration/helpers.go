// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

import "github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

// Wire enum values for the top-level `<key_type>` element.
//
// Asymmetric server normalisation (STYLE_GUIDE §Asymmetric server
// normalisation on `type`-style discriminator fields): the classic POST
// endpoint rejects `Individual and Institutional` (lowercase `and`) with
// HTTP 409 "Problem with key type" — only Title-Case `Individual And
// Institutional` is accepted on write. The GET path always returns the
// lowercase form. Pass the user's TF value through the server and state
// will silently drift on the next refresh; pass the lowercase form on
// write and Create fails.
//
// Pattern (mirrors directory_binding's `Likewise` PowerBroker alias):
// store the read-canonical lowercase form in TF state, translate one-way
// to the Title-Case write form inside the input builder via
// keyTypeWriteAlias. Users see the lowercase wire form everywhere; the
// alias is an implementation detail.
const (
	keyTypeIndividual    = proclassic.DiskEncryptionConfigurationKeyTypeIndividual
	keyTypeInstitutional = proclassic.DiskEncryptionConfigurationKeyTypeInstitutional

	// keyTypeIndividualInstitutional is the read-canonical form and stays a
	// literal: proclassic.DiskEncryptionConfigurationKeyType carries only the
	// Title-Cased write spelling, so there is no constant for the lowercase
	// "and" the server returns on read. Aliasing the write constant here would
	// collapse the two spellings this package exists to keep apart.
	keyTypeIndividualInstitutional = "Individual and Institutional"

	// keyTypeIndividualInstitutionalWriteAlias is the only spelling the
	// classic POST/PUT endpoint accepts for the combined recovery key
	// type. Server returns the lowercase form on read regardless.
	keyTypeIndividualInstitutionalWriteAlias = proclassic.DiskEncryptionConfigurationKeyTypeIndividualAndInstitutional
)

// allKeyTypeWireValues lists the canonical read-form `key_type` values.
// These are the only spellings the schema validator accepts at plan time;
// the write alias is applied silently by the input builder.
var allKeyTypeWireValues = []string{
	keyTypeIndividual,
	keyTypeInstitutional,
	keyTypeIndividualInstitutional,
}

// keyTypeWriteAlias returns the spelling the classic POST/PUT endpoint
// accepts. The TF value uses the read-canonical lowercase form (per the
// schema validator); the combined type must be Title-Cased on the wire
// because the server rejects the lowercase form on writes.
func keyTypeWriteAlias(v string) string {
	if v == keyTypeIndividualInstitutional {
		return keyTypeIndividualInstitutionalWriteAlias
	}
	return v
}

// Wire enum values for the top-level `<file_vault_enabled_users>` element.
const (
	fileVaultEnabledUsersCurrentOrNext   = proclassic.DiskEncryptionConfigurationFileVaultEnabledUsersCurrentOrNextUser
	fileVaultEnabledUsersManagementAccnt = proclassic.DiskEncryptionConfigurationFileVaultEnabledUsersManagementAccount
)

// allFileVaultEnabledUsersValues lists the accepted file_vault_enabled_users
// wire enum values.
var allFileVaultEnabledUsersValues = []string{
	fileVaultEnabledUsersCurrentOrNext,
	fileVaultEnabledUsersManagementAccnt,
}
