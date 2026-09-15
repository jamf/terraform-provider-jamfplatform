// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// Attribute-level validators for SSO settings cross-field rules. Each
// validator is value-discriminated — it only fires when the discriminator
// attribute holds a specific value — so off-the-shelf
// `stringvalidator.AlsoRequires` / `ConflictsWith` (which trigger on any
// value) cannot express the rule.
//
// All validators are anchored to absolute paths under the resource root
// because the rules cross between root attributes (`configuration_type`,
// `group_enrollment_access_enabled`) and nested block attributes
// (`saml_settings.*`, `signing_certificate.*`).

// ===== configuration_type ===================================================

// configurationTypeBlockValidator enforces the rules between
// configuration_type and the saml_settings / oidc_settings sub-blocks:
//
//   - OIDC: oidc_settings required; saml_settings forbidden (Jamf Pro
//     ignores saml_settings in pure OIDC mode, so accepting one would
//     silently discard user intent).
//   - SAML: saml_settings required.
//   - OIDC_WITH_SAML: both sub-blocks required.
type configurationTypeBlockValidator struct{}

// ConfigurationTypeBlockValidator constructs the validator.
func ConfigurationTypeBlockValidator() validator.String {
	return configurationTypeBlockValidator{}
}

// Description returns the validator description.
func (configurationTypeBlockValidator) Description(_ context.Context) string {
	return "configuration_type controls which of oidc_settings / saml_settings must be present"
}

// MarkdownDescription returns the markdown description.
func (v configurationTypeBlockValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (configurationTypeBlockValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var samlPresent, oidcPresent types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("saml_settings"), &samlPresent)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("oidc_settings"), &oidcPresent)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// "Present" (known, non-null) drives the forbidden checks; a genuinely-null
	// block drives the required checks. An UNKNOWN block (variable-driven)
	// defers on both — it is neither proven present nor proven absent.
	samlSet := !samlPresent.IsNull() && !samlPresent.IsUnknown()

	switch req.ConfigValue.ValueString() {
	case configurationTypeOIDC:
		if samlSet {
			resp.Diagnostics.AddAttributeError(
				path.Root("saml_settings"),
				"saml_settings forbidden when configuration_type = \"OIDC\"",
				"Remove the `saml_settings` block, or set `configuration_type = \"OIDC_WITH_SAML\"` to mix both modes.",
			)
		}
		if oidcPresent.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("oidc_settings"),
				"oidc_settings required when configuration_type = \"OIDC\"",
				"Supply at minimum `oidc_settings = { user_mapping = \"EMAIL\" }`.",
			)
		}
	case configurationTypeSAML:
		if samlPresent.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("saml_settings"),
				"saml_settings required when configuration_type = \"SAML\"",
				"Supply at minimum `entity_id`, `metadata_source`, and `group_attribute_name`.",
			)
		}
	case configurationTypeOIDCWithSAML:
		if samlPresent.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("saml_settings"),
				"saml_settings required when configuration_type = \"OIDC_WITH_SAML\"",
				"Both `saml_settings` and `oidc_settings` must be supplied.",
			)
		}
		if oidcPresent.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("oidc_settings"),
				"oidc_settings required when configuration_type = \"OIDC_WITH_SAML\"",
				"Both `saml_settings` and `oidc_settings` must be supplied.",
			)
		}
	}
}

// samlEntityIDRequiredValidator enforces entity_id non-empty when
// configuration_type ∈ {SAML, OIDC_WITH_SAML}.
type samlEntityIDRequiredValidator struct{}

// SamlEntityIDRequiredValidator constructs the validator.
func SamlEntityIDRequiredValidator() validator.String { return samlEntityIDRequiredValidator{} }

// Description returns the validator description.
func (samlEntityIDRequiredValidator) Description(_ context.Context) string {
	return "entity_id must be non-empty when configuration_type includes SAML"
}

// MarkdownDescription returns the markdown description.
func (v samlEntityIDRequiredValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (samlEntityIDRequiredValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	validateSamlActiveStringNonEmpty(ctx, req, resp, "entity_id", "SAML EntityID")
}

// samlGroupAttributeNameRequiredValidator enforces group_attribute_name
// non-empty when configuration_type ∈ {SAML, OIDC_WITH_SAML}.
type samlGroupAttributeNameRequiredValidator struct{}

// SamlGroupAttributeNameRequiredValidator constructs the validator.
func SamlGroupAttributeNameRequiredValidator() validator.String {
	return samlGroupAttributeNameRequiredValidator{}
}

// Description returns the validator description.
func (samlGroupAttributeNameRequiredValidator) Description(_ context.Context) string {
	return "group_attribute_name must be non-empty when configuration_type includes SAML"
}

// MarkdownDescription returns the markdown description.
func (v samlGroupAttributeNameRequiredValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (samlGroupAttributeNameRequiredValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	validateSamlActiveStringNonEmpty(ctx, req, resp, "group_attribute_name", "Group attribute name")
}

// validateSamlActiveStringNonEmpty is shared by the SAML-active companions:
// when configuration_type is SAML or OIDC_WITH_SAML, the wrapped attribute
// must hold a non-empty string.
func validateSamlActiveStringNonEmpty(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse, attrName, displayName string) {
	var configType types.String
	if d := req.Config.GetAttribute(ctx, path.Root("configuration_type"), &configType); d.HasError() {
		return
	}
	if configType.IsNull() || configType.IsUnknown() {
		return
	}
	if configType.ValueString() != configurationTypeSAML && configType.ValueString() != configurationTypeOIDCWithSAML {
		return
	}
	// Defer when the validated value is unknown (variable/for_each-driven):
	// config-time validation cannot see it. Error only when genuinely null/empty.
	if req.ConfigValue.IsUnknown() {
		return
	}
	if !req.ConfigValue.IsNull() && req.ConfigValue.ValueString() != "" {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		fmt.Sprintf("%s required when configuration_type = %q or %q", attrName, configurationTypeSAML, configurationTypeOIDCWithSAML),
		fmt.Sprintf("%s must be a non-empty string when SAML is part of the configuration. Either set this attribute or change configuration_type to \"%s\".", displayName, configurationTypeOIDC),
	)
}

// ===== metadata_source URL/FILE mutex =======================================

// metadataSourceBranchValidator enforces the URL/FILE mutex on
// saml_settings.metadata_source:
//
//   - URL: idp_url required; federation_metadata_file + metadata_file_name forbidden.
//   - FILE: federation_metadata_file + metadata_file_name required; idp_url forbidden.
type metadataSourceBranchValidator struct{}

// MetadataSourceBranchValidator constructs the validator.
func MetadataSourceBranchValidator() validator.String { return metadataSourceBranchValidator{} }

// Description returns the validator description.
func (metadataSourceBranchValidator) Description(_ context.Context) string {
	return "metadata_source = URL requires idp_url; metadata_source = FILE requires federation_metadata_file + metadata_file_name; the two branches are mutually exclusive"
}

// MarkdownDescription returns the markdown description.
func (v metadataSourceBranchValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (metadataSourceBranchValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	parent := req.Path.ParentPath()
	idpURL, fmf, mfn := types.String{}, types.String{}, types.String{}
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, parent.AtName("idp_url"), &idpURL)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, parent.AtName("federation_metadata_file"), &fmf)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, parent.AtName("metadata_file_name"), &mfn)...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch req.ConfigValue.ValueString() {
	case metadataSourceURL:
		if !isStringConfigured(idpURL) && !idpURL.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				parent.AtName("idp_url"),
				"idp_url required when metadata_source = \"URL\"",
				"Set `idp_url` to the IdP metadata endpoint.",
			)
		}
		if isStringConfigured(fmf) {
			resp.Diagnostics.AddAttributeError(
				parent.AtName("federation_metadata_file"),
				"federation_metadata_file forbidden when metadata_source = \"URL\"",
				"URL-sourced and FILE-sourced metadata are mutually exclusive. Remove `federation_metadata_file` or switch to `metadata_source = \"FILE\"`.",
			)
		}
		if isStringConfigured(mfn) {
			resp.Diagnostics.AddAttributeError(
				parent.AtName("metadata_file_name"),
				"metadata_file_name forbidden when metadata_source = \"URL\"",
				"`metadata_file_name` applies only to FILE-sourced metadata.",
			)
		}
	case metadataSourceFILE:
		if !isStringConfigured(fmf) && !fmf.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				parent.AtName("federation_metadata_file"),
				"federation_metadata_file required when metadata_source = \"FILE\"",
				"Supply `federation_metadata_file = filebase64(\"idp-metadata.xml\")`.",
			)
		}
		if !isStringConfigured(mfn) && !mfn.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				parent.AtName("metadata_file_name"),
				"metadata_file_name required when metadata_source = \"FILE\"",
				"Supply a display filename (e.g. \"idp-metadata.xml\").",
			)
		}
		if isStringConfigured(idpURL) {
			resp.Diagnostics.AddAttributeError(
				parent.AtName("idp_url"),
				"idp_url forbidden when metadata_source = \"FILE\"",
				"URL-sourced and FILE-sourced metadata are mutually exclusive. Remove `idp_url` or switch to `metadata_source = \"URL\"`.",
			)
		}
	}
}

// ===== idp_provider_type = OTHER companion ==================================

// idpProviderTypeOtherValidator enforces other_provider_type_name when
// idp_provider_type = "OTHER".
type idpProviderTypeOtherValidator struct{}

// IdpProviderTypeOtherValidator constructs the validator.
func IdpProviderTypeOtherValidator() validator.String { return idpProviderTypeOtherValidator{} }

// Description returns the validator description.
func (idpProviderTypeOtherValidator) Description(_ context.Context) string {
	return "idp_provider_type = \"OTHER\" requires other_provider_type_name"
}

// MarkdownDescription returns the markdown description.
func (v idpProviderTypeOtherValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (idpProviderTypeOtherValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != pro.SamlSettingsIdpProviderTypeOther {
		return
	}
	parent := req.Path.ParentPath()
	var other types.String
	if d := req.Config.GetAttribute(ctx, parent.AtName("other_provider_type_name"), &other); d.HasError() {
		return
	}
	if other.IsUnknown() || isStringConfigured(other) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		parent.AtName("other_provider_type_name"),
		"other_provider_type_name required when idp_provider_type = \"OTHER\"",
		"Supply a display name for the IdP (e.g. \"Internal SAML provider\").",
	)
}

// ===== user_attribute_enabled = true companion ==============================

// userAttributeEnabledValidator enforces user_attribute_name when
// user_attribute_enabled = true.
type userAttributeEnabledValidator struct{}

// UserAttributeEnabledValidator constructs the validator.
func UserAttributeEnabledValidator() validator.Bool { return userAttributeEnabledValidator{} }

// Description returns the validator description.
func (userAttributeEnabledValidator) Description(_ context.Context) string {
	return "user_attribute_enabled = true requires user_attribute_name"
}

// MarkdownDescription returns the markdown description.
func (v userAttributeEnabledValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateBool implements validator.Bool.
func (userAttributeEnabledValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !req.ConfigValue.ValueBool() {
		return
	}
	parent := req.Path.ParentPath()
	var name types.String
	if d := req.Config.GetAttribute(ctx, parent.AtName("user_attribute_name"), &name); d.HasError() {
		return
	}
	if name.IsUnknown() || isStringConfigured(name) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		parent.AtName("user_attribute_name"),
		"user_attribute_name required when user_attribute_enabled = true",
		"Supply the SAML attribute name carrying the username claim.",
	)
}

// ===== SAML-only feature-flag guard =========================================

// requiresSamlBoolValidator rejects a feature-flag bool set to `true` when
// `configuration_type = "OIDC"`. Jamf Pro only honors these SSO feature
// toggles — bypass, enrollment SSO, macOS Self Service SSO, account-driven
// enrollment SSO, group enrollment access — when SAML is part of the
// configuration (`SAML` or `OIDC_WITH_SAML`). On pure OIDC the server silently
// coerces every one of them to false, which would otherwise produce a
// plan-vs-apply mismatch ("was true, but now false") at apply time.
type requiresSamlBoolValidator struct {
	// fieldName is the snake_case attribute name, used verbatim in the
	// diagnostic so the message names the offending field.
	fieldName string
}

// RequiresSamlBoolValidator constructs a validator that forbids fieldName=true
// unless configuration_type includes SAML.
func RequiresSamlBoolValidator(fieldName string) validator.Bool {
	return requiresSamlBoolValidator{fieldName: fieldName}
}

// SsoBypassAllowedValidator constructs the bypass-specific guard. Retained as a
// named constructor for the existing schema wiring; the rule is identical to
// every other SAML-only flag.
func SsoBypassAllowedValidator() validator.Bool {
	return RequiresSamlBoolValidator("sso_bypass_allowed")
}

// Description returns the validator description.
func (v requiresSamlBoolValidator) Description(_ context.Context) string {
	return fmt.Sprintf("%s = true requires configuration_type to include SAML", v.fieldName)
}

// MarkdownDescription returns the markdown description.
func (v requiresSamlBoolValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateBool implements validator.Bool.
func (v requiresSamlBoolValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !req.ConfigValue.ValueBool() {
		return
	}
	var configType types.String
	if d := req.Config.GetAttribute(ctx, path.Root("configuration_type"), &configType); d.HasError() {
		return
	}
	if configType.IsNull() || configType.IsUnknown() {
		return
	}
	if configType.ValueString() == configurationTypeSAML || configType.ValueString() == configurationTypeOIDCWithSAML {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		fmt.Sprintf("%s = true requires configuration_type to include SAML", v.fieldName),
		fmt.Sprintf("Jamf Pro only honors %s when configuration_type is \"SAML\" or \"OIDC_WITH_SAML\"; in pure OIDC mode it is silently coerced to false. Set %s to false (or omit it), or change configuration_type.", v.fieldName, v.fieldName),
	)
}

// ===== group_enrollment_access_enabled = true companion =====================

// groupEnrollmentAccessEnabledValidator enforces group_enrollment_access_name
// when group_enrollment_access_enabled = true AND sso_for_enrollment_enabled = true.
// The conjunction matters: without sso_for_enrollment_enabled the gating
// field is inert and Jamf Pro tolerates an empty name.
type groupEnrollmentAccessEnabledValidator struct{}

// GroupEnrollmentAccessEnabledValidator constructs the validator.
func GroupEnrollmentAccessEnabledValidator() validator.Bool {
	return groupEnrollmentAccessEnabledValidator{}
}

// Description returns the validator description.
func (groupEnrollmentAccessEnabledValidator) Description(_ context.Context) string {
	return "group_enrollment_access_enabled = true with sso_for_enrollment_enabled = true requires group_enrollment_access_name"
}

// MarkdownDescription returns the markdown description.
func (v groupEnrollmentAccessEnabledValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateBool implements validator.Bool.
func (groupEnrollmentAccessEnabledValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !req.ConfigValue.ValueBool() {
		return
	}
	var ssoForEnrollment types.Bool
	if d := req.Config.GetAttribute(ctx, path.Root("sso_for_enrollment_enabled"), &ssoForEnrollment); d.HasError() {
		return
	}
	if ssoForEnrollment.IsNull() || ssoForEnrollment.IsUnknown() || !ssoForEnrollment.ValueBool() {
		return
	}
	var name types.String
	if d := req.Config.GetAttribute(ctx, path.Root("group_enrollment_access_name"), &name); d.HasError() {
		return
	}
	if name.IsUnknown() || isStringConfigured(name) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("group_enrollment_access_name"),
		"group_enrollment_access_name required when group_enrollment_access_enabled and sso_for_enrollment_enabled are both true",
		"Supply the LDAP/IdP group name allowed to enroll.",
	)
}

// ===== signing_certificate.setup_type = UPLOADED companions =================

// signingCertificateSetupTypeValidator enforces the upload-only fields when
// setup_type = UPLOADED.
type signingCertificateSetupTypeValidator struct{}

// SigningCertificateSetupTypeValidator constructs the validator.
func SigningCertificateSetupTypeValidator() validator.String {
	return signingCertificateSetupTypeValidator{}
}

// Description returns the validator description.
func (signingCertificateSetupTypeValidator) Description(_ context.Context) string {
	return "setup_type = \"UPLOADED\" requires type, key, keystore_file, keystore_file_name, keystore_password, password"
}

// MarkdownDescription returns the markdown description.
func (v signingCertificateSetupTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (signingCertificateSetupTypeValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != setupTypeUploaded {
		return
	}
	parent := req.Path.ParentPath()
	required := []string{
		"type",
		"key",
		"keystore_file",
		"keystore_file_name",
		"keystore_password",
		"password",
	}
	for _, name := range required {
		var v types.String
		if d := req.Config.GetAttribute(ctx, parent.AtName(name), &v); d.HasError() {
			continue
		}
		if v.IsUnknown() || isStringConfigured(v) {
			continue
		}
		resp.Diagnostics.AddAttributeError(
			parent.AtName(name),
			fmt.Sprintf("%s required when signing_certificate.setup_type = \"UPLOADED\"", name),
			"All of `type`, `key`, `keystore_file`, `keystore_file_name`, `keystore_password`, and `password` must be supplied to upload a keystore.",
		)
	}
}

// isStringConfigured reports whether a types.String holds a non-empty,
// non-null, non-unknown value.
func isStringConfigured(s types.String) bool {
	if s.IsNull() || s.IsUnknown() {
		return false
	}
	return s.ValueString() != ""
}

// Compile-time assertions.
var (
	_ validator.String = configurationTypeBlockValidator{}
	_ validator.String = samlEntityIDRequiredValidator{}
	_ validator.String = samlGroupAttributeNameRequiredValidator{}
	_ validator.String = metadataSourceBranchValidator{}
	_ validator.String = idpProviderTypeOtherValidator{}
	_ validator.Bool   = userAttributeEnabledValidator{}
	_ validator.Bool   = groupEnrollmentAccessEnabledValidator{}
	_ validator.String = signingCertificateSetupTypeValidator{}
)
