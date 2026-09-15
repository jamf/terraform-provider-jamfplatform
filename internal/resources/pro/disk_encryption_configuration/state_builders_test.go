// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// This file uses Go 1.26's extended `new(v)` builtin, which allocates a
// pointer to the value `v` and returns the pointer.

package disk_encryption_configuration

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAssign_Individual_EmptyIRKCollapsesToNil pins the load-bearing
// quirk from audit §1, §2.2: the server ALWAYS emits
// `<institutional_recovery_key>` on read, even for `Individual`
// key_type, with all empty children. If the state builder surfaced
// that as a populated TF model, every plan would show a permanent
// diff against an Individual-key_type resource whose user config omits
// the block.
func TestAssign_Individual_EmptyIRKCollapsesToNil(t *testing.T) {
	state := DiskEncryptionConfigurationResourceModel{}
	in := &proclassic.DiskEncryptionConfiguration{
		ID:                    new(59),
		Name:                  new("test"),
		KeyType:               new("Individual"),
		FileVaultEnabledUsers: new("Current or Next User"),
		// Server emits an empty IRK block on read even when the
		// resource is Individual-only.
		InstitutionalRecoveryKey: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
			Key:             new(""),
			CertificateType: new(""),
			PasswordSha256:  new(""),
			Data:            new(""),
		},
	}
	diags := assignDiskEncryptionConfigurationResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InstitutionalRecoveryKey != nil {
		t.Errorf("empty IRK wire block must collapse to nil TF model to avoid perma-diff against `Individual` configurations; got %+v", state.InstitutionalRecoveryKey)
	}
}

// TestAssign_Institutional_PopulatedIRKSurfaces covers the inverse: a
// populated wire block (real cert) must surface in TF state.
func TestAssign_Institutional_PopulatedIRKSurfaces(t *testing.T) {
	state := DiskEncryptionConfigurationResourceModel{}
	in := &proclassic.DiskEncryptionConfiguration{
		ID:                    new(60),
		Name:                  new("test-inst"),
		KeyType:               new(keyTypeIndividualInstitutional),
		FileVaultEnabledUsers: new("Current or Next User"),
		InstitutionalRecoveryKey: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
			Key:             new("C=US, O=jamf-tf-provider, CN=tf-audit-probe"),
			CertificateType: new("DER"),
			PasswordSha256:  new(""),
			Data:            new("base64-cert-payload"),
		},
	}
	diags := assignDiskEncryptionConfigurationResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InstitutionalRecoveryKey == nil {
		t.Fatalf("populated IRK wire block must surface as TF model")
	}
	if state.InstitutionalRecoveryKey.Key.ValueString() != "C=US, O=jamf-tf-provider, CN=tf-audit-probe" {
		t.Errorf("Key did not round-trip; got %q", state.InstitutionalRecoveryKey.Key.ValueString())
	}
	if state.InstitutionalRecoveryKey.CertificateType.ValueString() != "DER" {
		t.Errorf("CertificateType did not round-trip; got %q", state.InstitutionalRecoveryKey.CertificateType.ValueString())
	}
	if state.InstitutionalRecoveryKey.Data.ValueString() != "base64-cert-payload" {
		t.Errorf("Data did not round-trip")
	}
}

// TestAssign_PKCS12_PasswordSha256IsMaskedSentinel pins the masked-
// sentinel quirk (audit §2.9): server returns 20 asterisks when a
// password is set, not a real hash.
func TestAssign_PKCS12_PopulatesIRKBlock(t *testing.T) {
	state := DiskEncryptionConfigurationResourceModel{}
	in := &proclassic.DiskEncryptionConfiguration{
		ID:                    new(61),
		Name:                  new("test-p12"),
		KeyType:               new(keyTypeInstitutional),
		FileVaultEnabledUsers: new("Current or Next User"),
		InstitutionalRecoveryKey: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
			Key:             new("CN=probe"),
			CertificateType: new("PKCS12"),
			PasswordSha256:  new("********************"),
			Data:            new("base64-p12-payload"),
		},
	}
	diags := assignDiskEncryptionConfigurationResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InstitutionalRecoveryKey == nil {
		t.Fatalf("populated IRK must surface")
	}
	if state.InstitutionalRecoveryKey.Key.ValueString() != "CN=probe" {
		t.Errorf("server-derived Key must surface; got %q", state.InstitutionalRecoveryKey.Key.ValueString())
	}
	if state.InstitutionalRecoveryKey.CertificateType.ValueString() != "PKCS12" {
		t.Errorf("CertificateType must surface; got %q", state.InstitutionalRecoveryKey.CertificateType.ValueString())
	}
}

// TestAssign_WoVersionPreserved verifies that the rotation companion
// `password_wo_version` round-trips: the prior state value survives the
// state-builder pass (the framework persists it normally — it is a regular
// Optional Int64, not WriteOnly). The WriteOnly `Password` itself is
// stripped by the framework regardless of what the state builder writes.
func TestAssign_WoVersionPreserved(t *testing.T) {
	state := DiskEncryptionConfigurationResourceModel{
		InstitutionalRecoveryKey: &diskEncryptionConfigurationIRKModel{
			PasswordWoVersion: types.Int64Value(3),
		},
	}
	in := &proclassic.DiskEncryptionConfiguration{
		ID:                    new(1),
		Name:                  new("test"),
		KeyType:               new(keyTypeInstitutional),
		FileVaultEnabledUsers: new("Current or Next User"),
		InstitutionalRecoveryKey: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
			Key:             new("CN=probe"),
			CertificateType: new("PKCS12"),
			Data:            new("base64-p12"),
		},
	}
	diags := assignDiskEncryptionConfigurationResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InstitutionalRecoveryKey == nil {
		t.Fatalf("populated IRK must surface")
	}
	if state.InstitutionalRecoveryKey.PasswordWoVersion.ValueInt64() != 3 {
		t.Errorf("PasswordWoVersion must round-trip from prior state; got %d", state.InstitutionalRecoveryKey.PasswordWoVersion.ValueInt64())
	}
}

// TestAssign_IDNotClobbered verifies that a transient GET response with
// a nil ID does not clobber a populated state.ID from Create.
func TestAssign_IDNotClobbered(t *testing.T) {
	state := DiskEncryptionConfigurationResourceModel{
		ID: types.StringValue("42"),
	}
	in := &proclassic.DiskEncryptionConfiguration{
		Name:    new("test"),
		KeyType: new(keyTypeIndividual),
	}
	diags := assignDiskEncryptionConfigurationResourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "42" {
		t.Errorf("nil API ID must not clobber existing state ID; got %s", state.ID.ValueString())
	}
}

// TestAssign_NilSafe covers the contract that a nil response leaves
// state untouched.
func TestAssign_NilSafe(t *testing.T) {
	state := DiskEncryptionConfigurationResourceModel{ID: types.StringValue("1")}
	diags := assignDiskEncryptionConfigurationResourceModel(&state, nil)
	if diags.HasError() {
		t.Errorf("nil-response assigner must return clean diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "1" {
		t.Errorf("nil response must leave state untouched")
	}
}

// TestAssign_DataSource_EmptyIRKCollapsesToNil mirrors the resource-side
// empty-collapse test for the data source.
func TestAssign_DataSource_EmptyIRKCollapsesToNil(t *testing.T) {
	state := DiskEncryptionConfigurationDataSourceModel{}
	in := &proclassic.DiskEncryptionConfiguration{
		ID:                    new(1),
		Name:                  new("test"),
		KeyType:               new(keyTypeIndividual),
		FileVaultEnabledUsers: new("Current or Next User"),
		InstitutionalRecoveryKey: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
			Key:             new(""),
			CertificateType: new(""),
			PasswordSha256:  new(""),
			Data:            new(""),
		},
	}
	diags := assignDiskEncryptionConfigurationDataSourceModel(&state, in)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InstitutionalRecoveryKey != nil {
		t.Errorf("data source: empty IRK wire block must collapse to nil; got %+v", state.InstitutionalRecoveryKey)
	}
}

// TestIrkIsWireEmpty pins the empty-detector across the field
// permutations the server can emit.
func TestIrkIsWireEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   *proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey
		want bool
	}{
		{
			name: "all_empty_strings",
			in: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
				Key:             new(""),
				CertificateType: new(""),
				PasswordSha256:  new(""),
				Data:            new(""),
			},
			want: true,
		},
		{
			name: "all_nil_pointers",
			in:   &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{},
			want: true,
		},
		{
			name: "has_data",
			in: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
				Data: new("base64"),
			},
			want: false,
		},
		{
			name: "has_key",
			in: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
				Key: new("CN=x"),
			},
			want: false,
		},
		{
			name: "has_sentinel",
			in: &proclassic.DiskEncryptionConfigurationInstitutionalRecoveryKey{
				PasswordSha256: new("********************"),
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := irkIsWireEmpty(tc.in); got != tc.want {
				t.Errorf("irkIsWireEmpty(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
