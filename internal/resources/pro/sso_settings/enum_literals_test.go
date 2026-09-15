// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"slices"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.SsoSettingsV3ConfigurationTypeValues(),
			pro.SamlSettingsMetadataSourceValues(),
			pro.SamlSettingsIdpProviderTypeValues(),
			pro.SamlSettingsUserMappingValues(),
			pro.OidcSettingsUserMappingValues(),
			pro.SsoKeystoreKeystoreSetupTypeValues(),
			pro.SsoKeystoreTypeValues(),
		),
	})
	if err != nil {
		t.Fatalf("enumguard.Check: %v", err)
	}
	for _, problem := range got.Problems() {
		t.Error(problem)
	}
	if got.Examined == 0 {
		t.Fatal("no string literals parsed — the guard found nothing to check")
	}
}

// TestUserMappingVocabulariesAgree backs the single validUserMappings shared by
// the SAML and OIDC blocks. The API declares the same two values for both, so
// one var is right for both — but only while that holds. If a future SDK
// release adds a value to one side, this fails and the schema needs two sets.
func TestUserMappingVocabulariesAgree(t *testing.T) {
	saml := pro.SamlSettingsUserMappingValues()
	oidc := pro.OidcSettingsUserMappingValues()

	for _, v := range saml {
		if !slices.Contains(oidc, v) {
			t.Errorf("SamlSettingsUserMapping carries %q but OidcSettingsUserMapping does not", v)
		}
	}
	for _, v := range oidc {
		if !slices.Contains(saml, v) {
			t.Errorf("OidcSettingsUserMapping carries %q but SamlSettingsUserMapping does not", v)
		}
	}
}
