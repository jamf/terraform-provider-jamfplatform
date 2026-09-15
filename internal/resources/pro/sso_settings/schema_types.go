// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// ssoSettingsTimeoutAttributeTypes defines the timeout attribute types.
var ssoSettingsTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// configurationType values accepted by /v3/sso. The generated set is the three
// accepted values; the UNKNOWN that the upstream openapi enum once carried, and
// that probe G7 found runtime-rejected, is no longer generated, so there is
// nothing left to narrow.
const (
	configurationTypeSAML         = pro.SsoSettingsV3ConfigurationTypeSaml
	configurationTypeOIDC         = pro.SsoSettingsV3ConfigurationTypeOidc
	configurationTypeOIDCWithSAML = pro.SsoSettingsV3ConfigurationTypeOidcWithSaml
)

// validConfigurationTypes are the accepted configuration_type values.
var validConfigurationTypes = pro.SsoSettingsV3ConfigurationTypeValues()

// metadataSource values accepted by /v3/sso.
const (
	metadataSourceURL  = pro.SamlSettingsMetadataSourceURL
	metadataSourceFILE = pro.SamlSettingsMetadataSourceFile
)

// validMetadataSources is deliberately narrower than
// pro.SamlSettingsMetadataSourceValues(), which also carries UNKNOWN: probe G7
// found /v3/sso rejects UNKNOWN at runtime, so offering it in the schema would
// let a plan pass validation and then fail mid-apply. The set is curated; the
// spellings are still the SDK's.
var validMetadataSources = []string{metadataSourceURL, metadataSourceFILE}

// validIdpProviderTypes are the accepted saml_settings.idp_provider_type values.
var validIdpProviderTypes = pro.SamlSettingsIdpProviderTypeValues()

// validUserMappings is the userMapping enum, shared between the SAML and OIDC
// blocks because the API declares the same two values for both. Keyed on the
// SAML vocabulary; TestUserMappingVocabulariesAgree fails if a future SDK
// release stops the two agreeing, since one shared var could then only be right
// for one of them.
var validUserMappings = pro.SamlSettingsUserMappingValues()

// signing_certificate.setup_type enum.
const (
	setupTypeGenerated = pro.SsoKeystoreKeystoreSetupTypeGenerated
	setupTypeUploaded  = pro.SsoKeystoreKeystoreSetupTypeUploaded
	setupTypeNone      = pro.SsoKeystoreKeystoreSetupTypeNone
)

var validSetupTypes = pro.SsoKeystoreKeystoreSetupTypeValues()

// validKeystoreTypes is deliberately narrower than pro.SsoKeystoreTypeValues(),
// which also carries NONE: NONE is what the keystore type reads back as when no
// certificate is configured, not a type a caller can ask for. The set is
// curated; the spellings are the SDK's.
var validKeystoreTypes = []string{pro.SsoKeystoreTypePkcs12, pro.SsoKeystoreTypeJks}

// signingCertificateKeyAttrTypes describes the element type of the Computed
// `keys` list in the cert sub-block. The wire shape is
// `[{id: string, valid: bool}]` per pro.CertificateKey.
var signingCertificateKeyAttrTypes = map[string]attr.Type{
	"id":    types.StringType,
	"valid": types.BoolType,
}
