// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignSsoSettingsResourceModel populates state from a /v3/sso GET response.
//
// The OidcSettings and SamlSettings sub-blocks always serialise on the wire,
// even in single-mode configurations (the SDK uses value types). State
// reflects user intent: if the user did not author the inactive block, keep
// state at nil rather than echoing back the server's empty placeholder.
//
// federation_metadata_file is re-base64-encoded from the SDK []byte so the
// state matches the canonical RFC 4648 base64 a user would produce via
// `filebase64()`.
func assignSsoSettingsResourceModel(ctx context.Context, state *SsoSettingsResourceModel, s *pro.SsoSettingsV3) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	state.SsoEnabled = types.BoolValue(s.SsoEnabled)
	state.SsoBypassAllowed = types.BoolValue(s.SsoBypassAllowed)
	state.SsoForEnrollmentEnabled = types.BoolValue(s.SsoForEnrollmentEnabled)
	state.SsoForMacOsSelfServiceEnabled = types.BoolValue(s.SsoForMacOsSelfServiceEnabled)
	state.EnrollmentSsoForAccountDrivenEnrollmentEnabled = types.BoolValue(s.EnrollmentSsoForAccountDrivenEnrollmentEnabled)
	state.GroupEnrollmentAccessEnabled = types.BoolValue(s.GroupEnrollmentAccessEnabled)
	state.GroupEnrollmentAccessName = helpers.ReconcileOptionalStringPointer(s.GroupEnrollmentAccessName, state.GroupEnrollmentAccessName)
	state.ConfigurationType = types.StringValue(s.ConfigurationType)

	// OIDC sub-block — preserve nil intent on inactive branches.
	if state.OidcSettings != nil || s.ConfigurationType == configurationTypeOIDC || s.ConfigurationType == configurationTypeOIDCWithSAML {
		o := &oidcSettingsModel{
			UserMapping:                   types.StringValue(s.OidcSettings.UserMapping),
			JamfIDAuthenticationEnabled:   helpers.BoolPointerValueOrNull(s.OidcSettings.JamfIDAuthenticationEnabled),
			UsernameAttributeClaimMapping: helpers.StringPointerValueOrNull(s.OidcSettings.UsernameAttributeClaimMapping),
		}
		// Echo-back stub from SAML-only mode: if user did not author the
		// block AND we are in pure SAML mode AND UserMapping is the stub
		// default we injected, leave state nil.
		if state.OidcSettings == nil && s.ConfigurationType == configurationTypeSAML && o.UserMapping.ValueString() == pro.OidcSettingsUserMappingEmail &&
			o.JamfIDAuthenticationEnabled.IsNull() && o.UsernameAttributeClaimMapping.IsNull() {
			state.OidcSettings = nil
		} else {
			state.OidcSettings = o
		}
	}

	// SAML sub-block — preserve nil intent on inactive branches.
	if state.SamlSettings != nil || s.ConfigurationType == configurationTypeSAML || s.ConfigurationType == configurationTypeOIDCWithSAML {
		state.SamlSettings = assignSamlSettingsModel(state.SamlSettings, &s.SamlSettings)
	}

	// EnrollmentSsoConfig — only populate when user authored OR server returns content.
	if s.EnrollmentSsoConfig != nil && (state.EnrollmentSsoConfig != nil || hasEnrollmentSsoContent(s.EnrollmentSsoConfig)) {
		ec, d := assignEnrollmentSsoConfigModel(ctx, s.EnrollmentSsoConfig)
		diags.Append(d...)
		state.EnrollmentSsoConfig = ec
	}

	return diags
}

// assignSamlSettingsModel populates the SAML sub-model from the SDK type.
// Preserves user-intent for unset optional fields via ReconcileOptional*.
func assignSamlSettingsModel(prev *samlSettingsModel, s *pro.SamlSettings) *samlSettingsModel {
	out := &samlSettingsModel{}
	if prev != nil {
		*out = *prev
	}

	out.IdpProviderType = helpers.ReconcileOptionalStringPointer(s.IdpProviderType, out.IdpProviderType)
	out.OtherProviderTypeName = helpers.ReconcileOptionalStringPointer(s.OtherProviderTypeName, out.OtherProviderTypeName)
	out.EntityID = helpers.ReconcileOptionalStringPointer(s.EntityID, out.EntityID)
	out.MetadataSource = helpers.ReconcileOptionalStringPointer(s.MetadataSource, out.MetadataSource)
	out.IdpURL = helpers.ReconcileOptionalStringPointer(s.IdpURL, out.IdpURL)
	out.MetadataFileName = helpers.ReconcileOptionalStringPointer(s.MetadataFileName, out.MetadataFileName)
	out.UserMapping = helpers.ReconcileOptionalStringPointer(s.UserMapping, out.UserMapping)
	out.UserAttributeName = helpers.ReconcileOptionalStringPointer(s.UserAttributeName, out.UserAttributeName)
	out.GroupAttributeName = helpers.ReconcileOptionalStringPointer(s.GroupAttributeName, out.GroupAttributeName)
	out.GroupRdnKey = helpers.ReconcileOptionalStringPointer(s.GroupRdnKey, out.GroupRdnKey)

	out.UserAttributeEnabled = helpers.ReconcileOptionalBoolPointer(s.UserAttributeEnabled, out.UserAttributeEnabled)
	out.TokenExpirationDisabled = helpers.ReconcileOptionalBoolPointer(s.TokenExpirationDisabled, out.TokenExpirationDisabled)

	if s.SessionTimeout != nil {
		out.SessionTimeout = types.Int64Value(int64(*s.SessionTimeout))
	} else if out.SessionTimeout.IsUnknown() {
		out.SessionTimeout = types.Int64Null()
	}

	// federationMetadataFile is Optional only — preserve the user's plan
	// value verbatim. `filebase64()` output can contain line wraps or
	// trailing newlines that the canonical wire re-encoding strips, which
	// would otherwise diff against the plan even when content matches.
	// out.FederationMetadataFile already carries the plan value (copied
	// from prev above); no reconciliation needed.

	return out
}

// hasEnrollmentSsoContent reports whether an EnrollmentSsoConfig contains
// any user-meaningful content. A bare `{Hosts: nil, ManagementHint: nil}` is
// considered absent so an OIDC-only tenant doesn't see a spurious empty
// block in state.
func hasEnrollmentSsoContent(c *pro.EnrollmentSsoConfig) bool {
	if c == nil {
		return false
	}
	if c.Hosts != nil && len(*c.Hosts) > 0 {
		return true
	}
	if c.ManagementHint != nil && *c.ManagementHint != "" {
		return true
	}
	return false
}

// assignEnrollmentSsoConfigModel turns the SDK type into the TF model.
func assignEnrollmentSsoConfigModel(ctx context.Context, c *pro.EnrollmentSsoConfig) (*enrollmentSsoConfigModel, diag.Diagnostics) {
	out := &enrollmentSsoConfigModel{
		ManagementHint: helpers.StringPointerValueOrNull(c.ManagementHint),
	}
	if c.Hosts == nil {
		out.Hosts = types.SetNull(types.StringType)
		return out, nil
	}
	v, d := types.SetValueFrom(ctx, types.StringType, *c.Hosts)
	if d.HasError() {
		return out, d
	}
	out.Hosts = v
	return out, nil
}

// assignSigningCertificateState writes the cert sub-block details from the
// /v2/sso/cert GET response. When the server reports keystoreSetupType=NONE
// (the "no cert" sentinel) AND the user has not configured the block, leave
// state nil. When the user configured the block, populate Computed siblings
// from the response and leave WriteOnly inputs untouched (the framework
// drops them from state regardless).
func assignSigningCertificateState(ctx context.Context, state *SsoSettingsResourceModel, cert *pro.SsoKeystoreResponseWithDetails) diag.Diagnostics {
	var diags diag.Diagnostics
	if cert == nil {
		return diags
	}

	// When the user has not configured a signing_certificate block, never
	// inject one in state from the server's current cert. Doing so would
	// surface a tenant-side cert Terraform was never asked to manage and
	// trigger "Provider produced inconsistent result after apply" on the
	// Sensitive parent path.
	if state.SigningCertificate == nil {
		return diags
	}

	if cert.Keystore != nil {
		// Populate setup_type when state lacks a concrete value. On Import
		// the freshly-constructed model carries types.StringNull(), so the
		// null and unknown branches both need to fire.
		if cert.Keystore.KeystoreSetupType != "" &&
			(state.SigningCertificate.SetupType.IsNull() || state.SigningCertificate.SetupType.IsUnknown()) {
			state.SigningCertificate.SetupType = types.StringValue(cert.Keystore.KeystoreSetupType)
		}
		state.SigningCertificate.Type = helpers.StringValueOrNull(cert.Keystore.Type)
		state.SigningCertificate.Key = helpers.StringValueOrNull(cert.Keystore.Key)
		state.SigningCertificate.KeystoreFileName = helpers.StringValueOrNull(cert.Keystore.KeystoreFileName)
	}

	if cert.KeystoreDetails != nil {
		state.SigningCertificate.SerialNumber = serialNumberToState(cert.KeystoreDetails.SerialNumber)
		state.SigningCertificate.Subject = stringValueOrNullFromWire(cert.KeystoreDetails.Subject)
		state.SigningCertificate.Issuer = stringValueOrNullFromWire(cert.KeystoreDetails.Issuer)
		state.SigningCertificate.Expiration = stringValueOrNullFromWire(cert.KeystoreDetails.Expiration)
	} else {
		state.SigningCertificate.SerialNumber = types.StringNull()
		state.SigningCertificate.Subject = types.StringNull()
		state.SigningCertificate.Issuer = types.StringNull()
		state.SigningCertificate.Expiration = types.StringNull()
	}

	// Keys list — populated from KeystoreResponse.Keys (richer shape than
	// KeystoreDetails.Keys, which is just []string).
	keys, d := buildSigningCertificateKeysList(ctx, cert)
	diags.Append(d...)
	state.SigningCertificate.Keys = keys

	return diags
}

// buildSigningCertificateKeysList converts the SDK Keys array (objects with
// id+valid) into a Terraform list-of-object.
func buildSigningCertificateKeysList(ctx context.Context, cert *pro.SsoKeystoreResponseWithDetails) (types.List, diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: signingCertificateKeyAttrTypes}
	if cert == nil || cert.Keystore == nil || len(cert.Keystore.Keys) == 0 {
		return types.ListValueMust(elemType, []attr.Value{}), nil
	}
	values := make([]attr.Value, 0, len(cert.Keystore.Keys))
	for _, k := range cert.Keystore.Keys {
		obj, d := types.ObjectValue(signingCertificateKeyAttrTypes, map[string]attr.Value{
			"id":    helpers.StringPointerValueOrNull(k.ID),
			"valid": helpers.BoolPointerValueOrNull(k.Valid),
		})
		if d.HasError() {
			return types.ListNull(elemType), d
		}
		values = append(values, obj)
	}
	return types.ListValueFrom(ctx, elemType, values)
}

// serialNumberToState renders the *json.Number serial as a state string. The
// SDK uses *json.Number to preserve real X.509 BigInts up to ~157 bits
// losslessly. Accepts *json.Number directly so a typed-nil pointer maps to
// null state without panicking on interface-coerced nil.
func serialNumberToState(n *json.Number) types.String {
	if n == nil {
		return types.StringNull()
	}
	s := n.String()
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// stringValueOrNullFromWire is the symmetric counterpart of
// helpers.StringValueOrNull for plain wire strings.
func stringValueOrNullFromWire(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

// ===== Data-source-side assigners ============================================

// assignSsoSettingsDataSourceModel populates a data source model from the
// settings + cert GET responses.
func assignSsoSettingsDataSourceModel(ctx context.Context, state *SsoSettingsDataSourceModel, s *pro.SsoSettingsV3, cert *pro.SsoKeystoreResponseWithDetails) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	state.SsoEnabled = types.BoolValue(s.SsoEnabled)
	state.SsoBypassAllowed = types.BoolValue(s.SsoBypassAllowed)
	state.SsoForEnrollmentEnabled = types.BoolValue(s.SsoForEnrollmentEnabled)
	state.SsoForMacOsSelfServiceEnabled = types.BoolValue(s.SsoForMacOsSelfServiceEnabled)
	state.EnrollmentSsoForAccountDrivenEnrollmentEnabled = types.BoolValue(s.EnrollmentSsoForAccountDrivenEnrollmentEnabled)
	state.GroupEnrollmentAccessEnabled = types.BoolValue(s.GroupEnrollmentAccessEnabled)
	state.GroupEnrollmentAccessName = helpers.StringPointerValueOrNull(s.GroupEnrollmentAccessName)
	state.ConfigurationType = types.StringValue(s.ConfigurationType)

	state.OidcSettings = &oidcSettingsModel{
		UserMapping:                   types.StringValue(s.OidcSettings.UserMapping),
		JamfIDAuthenticationEnabled:   helpers.BoolPointerValueOrNull(s.OidcSettings.JamfIDAuthenticationEnabled),
		UsernameAttributeClaimMapping: helpers.StringPointerValueOrNull(s.OidcSettings.UsernameAttributeClaimMapping),
	}

	state.SamlSettings = assignSamlSettingsModel(nil, &s.SamlSettings)

	if s.EnrollmentSsoConfig != nil && hasEnrollmentSsoContent(s.EnrollmentSsoConfig) {
		ec, d := assignEnrollmentSsoConfigModel(ctx, s.EnrollmentSsoConfig)
		diags.Append(d...)
		state.EnrollmentSsoConfig = ec
	}

	// Cert read-only projection.
	if cert != nil && cert.Keystore != nil && cert.Keystore.KeystoreSetupType != setupTypeNone {
		rocert := &signingCertificateReadOnlyModel{
			SetupType:        types.StringValue(cert.Keystore.KeystoreSetupType),
			Type:             helpers.StringValueOrNull(cert.Keystore.Type),
			Key:              helpers.StringValueOrNull(cert.Keystore.Key),
			KeystoreFileName: helpers.StringValueOrNull(cert.Keystore.KeystoreFileName),
		}
		if cert.KeystoreDetails != nil {
			rocert.SerialNumber = serialNumberToState(cert.KeystoreDetails.SerialNumber)
			rocert.Subject = stringValueOrNullFromWire(cert.KeystoreDetails.Subject)
			rocert.Issuer = stringValueOrNullFromWire(cert.KeystoreDetails.Issuer)
			rocert.Expiration = stringValueOrNullFromWire(cert.KeystoreDetails.Expiration)
		} else {
			rocert.SerialNumber = types.StringNull()
			rocert.Subject = types.StringNull()
			rocert.Issuer = types.StringNull()
			rocert.Expiration = types.StringNull()
		}
		keys, d := buildSigningCertificateKeysList(ctx, cert)
		diags.Append(d...)
		rocert.Keys = keys
		state.SigningCertificate = rocert
	}

	return diags
}
