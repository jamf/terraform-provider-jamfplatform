// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestSerialNumberToState_BigInt ensures the *json.Number SDK field surfaces
// full-precision 157-bit X.509 serials losslessly via .String().
func TestSerialNumberToState_BigInt(t *testing.T) {
	bigSerial := json.Number("139740726707269723692607826204984509091849452814")
	got := serialNumberToState(&bigSerial)
	if got.IsNull() {
		t.Fatal("expected non-null serial number")
	}
	if got.ValueString() != bigSerial.String() {
		t.Errorf("expected %q, got %q", bigSerial.String(), got.ValueString())
	}
}

// TestSerialNumberToState_Nil ensures nil maps to null state.
func TestSerialNumberToState_Nil(t *testing.T) {
	if !serialNumberToState((*json.Number)(nil)).IsNull() {
		t.Error("expected null state for nil serial")
	}
}

// TestAssignSamlSettingsModel_PreservesUserBase64 confirms the user's
// authored federation_metadata_file string flows through verbatim from
// prev (plan) to state, regardless of how the wire bytes encode. This
// matches the Optional-only schema shape: state mirrors the user's input
// directly so canonical re-encoding cannot trip Terraform Core's plan-vs-
// apply consistency guarantee on a Sensitive nested attribute.
func TestAssignSamlSettingsModel_PreservesUserBase64(t *testing.T) {
	userBase64 := "PEVudGl0eURlc2NyaXB0b3IgLz4="
	prev := &samlSettingsModel{
		FederationMetadataFile: types.StringValue(userBase64),
	}
	rawXML := []byte("<EntityDescriptor />")
	wire := &pro.SamlSettings{
		FederationMetadataFile: &rawXML,
	}
	out := assignSamlSettingsModel(prev, wire)
	if out == nil {
		t.Fatal("expected non-nil model")
	}
	if got := out.FederationMetadataFile.ValueString(); got != userBase64 {
		t.Errorf("federation_metadata_file = %q, want %q (verbatim user value)", got, userBase64)
	}
}

// TestAssignSsoSettingsResourceModel_OmitsInactiveBranch verifies that in
// pure SAML mode the OIDC stub injection does not surface as a populated
// state block when the user did not author oidc_settings.
func TestAssignSsoSettingsResourceModel_OmitsInactiveBranch(t *testing.T) {
	wire := &pro.SsoSettingsV3{
		ConfigurationType: configurationTypeSAML,
		OidcSettings: pro.OidcSettings{
			UserMapping: "EMAIL", // the stub the input builder injects
		},
		SamlSettings: pro.SamlSettings{},
	}
	state := &SsoSettingsResourceModel{}
	if d := assignSsoSettingsResourceModel(context.Background(), state, wire); d.HasError() {
		t.Fatalf("assigner returned errors: %v", d)
	}
	if state.OidcSettings != nil {
		t.Error("OIDC sub-block must remain nil when user did not author it in pure SAML mode")
	}
	if state.SamlSettings == nil {
		t.Error("SAML sub-block should be populated in SAML mode")
	}
}

// TestAssignSsoSettingsResourceModel_PopulatesAuthoredBranch verifies that
// when the user authored oidc_settings, server-echoed fields land in state.
func TestAssignSsoSettingsResourceModel_PopulatesAuthoredBranch(t *testing.T) {
	wire := &pro.SsoSettingsV3{
		ConfigurationType: configurationTypeOIDC,
		OidcSettings: pro.OidcSettings{
			UserMapping: "EMAIL",
		},
	}
	state := &SsoSettingsResourceModel{
		OidcSettings: &oidcSettingsModel{UserMapping: types.StringValue("EMAIL")},
	}
	if d := assignSsoSettingsResourceModel(context.Background(), state, wire); d.HasError() {
		t.Fatalf("assigner returned errors: %v", d)
	}
	if state.OidcSettings == nil {
		t.Fatal("OIDC sub-block must be populated when user authored it")
	}
	if state.OidcSettings.UserMapping.ValueString() != "EMAIL" {
		t.Errorf("user_mapping = %q, want EMAIL", state.OidcSettings.UserMapping.ValueString())
	}
}

// TestAssignSigningCertificateState_NeverHydratesAnUnauthoredBlock pins a
// behaviour that looks like the import-hydration bug fixed elsewhere in this
// provider (#391, #399) but must NOT be "fixed" the same way.
//
// assignSigningCertificateState leaves signing_certificate nil when the
// incoming model has none, even for a fully populated wire certificate. On
// every other resource that shape is the bug: import leaves a declared block
// null and it plans as an addition. Here it is load-bearing.
//
// applyCertificateOnUpdate opens with
//
//	if plan.SigningCertificate == nil && state.SigningCertificate == nil { return true }
//
// which is what stops Terraform deleting a certificate it did not create, and
// falls back to a wire read (currentCertSetupType) when state carries no setup
// type — so GENERATED → GENERATED is already a no-op after an import, with no
// hydration needed. Populate state here and that guard stops firing: a
// practitioner who imports and does not declare the block then hits the
// transition table's GEN → nil row, which is DELETE, and loses the tenant's SSO
// signing certificate. The block is Optional, so that path is fully reachable.
//
// If this test fails because someone applied the #391 pattern uniformly, the
// fix is to revert that, not to update this test.
func TestAssignSigningCertificateState_NeverHydratesAnUnauthoredBlock(t *testing.T) {
	state := &SsoSettingsResourceModel{} // no signing_certificate authored
	cert := &pro.SsoKeystoreResponseWithDetails{
		Keystore: &pro.SsoKeystoreResponse{
			Key:               "jamf",
			KeystoreFileName:  "sso.p12",
			KeystoreSetupType: "GENERATED",
			Type:              "PKCS12",
		},
	}

	diags := assignSigningCertificateState(context.Background(), state, cert)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if state.SigningCertificate != nil {
		t.Fatalf("signing_certificate must stay nil for an unauthored block; hydrating it defeats the delete guard in applyCertificateOnUpdate. Got %+v", state.SigningCertificate)
	}
}

// TestAssignEnrollmentSsoConfigModel_ManagementHintEmptyString pins #445:
// Jamf Pro echoes an unset management hint as "", so an authored "" must
// survive the read while an unset hint, or one with no prior block, stays null.
func TestAssignEnrollmentSsoConfigModel_ManagementHintEmptyString(t *testing.T) {
	wire := &pro.EnrollmentSsoConfig{ManagementHint: new(""), Hosts: &[]string{"idp.example.com"}}
	for name, tc := range map[string]struct {
		prev     *enrollmentSsoConfigModel
		wantNull bool
	}{
		"authored empty": {prev: &enrollmentSsoConfigModel{ManagementHint: types.StringValue("")}, wantNull: false},
		"unset":          {prev: &enrollmentSsoConfigModel{ManagementHint: types.StringNull()}, wantNull: true},
		"no prior block": {prev: nil, wantNull: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, d := assignEnrollmentSsoConfigModel(context.Background(), wire, tc.prev)
			if d.HasError() {
				t.Fatalf("unexpected diagnostics: %v", d)
			}
			if got.ManagementHint.IsNull() != tc.wantNull || got.ManagementHint.ValueString() != "" {
				t.Errorf("management_hint = %s, want null=%t", got.ManagementHint, tc.wantNull)
			}
		})
	}
}

// TestAssignSigningCertificateState_GeneratedEmptyKeystoreFileName pins #445 on
// the certificate block: a generated certificate reports its keystore file name
// as "", so an authored "" must survive the read, an unset one stays null, and
// the alias Jamf Pro assigns still replaces an unknown key.
func TestAssignSigningCertificateState_GeneratedEmptyKeystoreFileName(t *testing.T) {
	cert := &pro.SsoKeystoreResponseWithDetails{
		Keystore: &pro.SsoKeystoreResponse{
			Key:               "saml",
			KeystoreFileName:  "",
			KeystoreSetupType: "GENERATED",
			Type:              "PKCS12",
		},
	}
	for name, tc := range map[string]struct {
		incoming types.String
		wantNull bool
	}{
		"authored empty": {incoming: types.StringValue(""), wantNull: false},
		"unset":          {incoming: types.StringUnknown(), wantNull: true},
	} {
		t.Run(name, func(t *testing.T) {
			state := &SsoSettingsResourceModel{SigningCertificate: &signingCertificateModel{
				SetupType:        types.StringValue("GENERATED"),
				Key:              types.StringUnknown(),
				KeystoreFileName: tc.incoming,
			}}
			if d := assignSigningCertificateState(context.Background(), state, cert); d.HasError() {
				t.Fatalf("diagnostics: %v", d)
			}
			got := state.SigningCertificate
			if got.KeystoreFileName.IsNull() != tc.wantNull || got.KeystoreFileName.ValueString() != "" {
				t.Errorf("keystore_file_name = %s, want null=%t", got.KeystoreFileName, tc.wantNull)
			}
			if got.Key.ValueString() != "saml" {
				t.Errorf("key = %s, want the server-assigned alias", got.Key)
			}
		})
	}
}
