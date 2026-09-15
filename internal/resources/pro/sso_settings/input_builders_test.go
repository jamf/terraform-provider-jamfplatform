// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// unknownStringPlan is what the framework hands the builder for an
// Optional+Computed attribute the configuration omits on a first apply:
// UseStateForUnknown has no prior state to substitute, so the value stays
// unknown. Every "undeclared on create" case below is built from these rather
// than from nulls, so the tests exercise the shape the plan really has.
func unknownStringPlan() types.String { return types.StringUnknown() }

// unknownBoolPlan is unknownStringPlan for a bool attribute.
func unknownBoolPlan() types.Bool { return types.BoolUnknown() }

// configuredTenant is a tenant holding a complete SAML-capable configuration,
// used as the merge base a create adopts. The values are deliberately
// non-default so a reset is visible.
func configuredTenant() *pro.SsoSettingsV3 {
	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }
	hosts := []string{"idp.example.com"}

	return &pro.SsoSettingsV3{
		ConfigurationType:             "OIDC_WITH_SAML",
		SsoEnabled:                    true,
		SsoBypassAllowed:              true,
		SsoForEnrollmentEnabled:       true,
		SsoForMacOsSelfServiceEnabled: true,
		EnrollmentSsoForAccountDrivenEnrollmentEnabled: true,
		GroupEnrollmentAccessEnabled:                   true,
		GroupEnrollmentAccessName:                      strPtr("Enrollment Admins"),
		OidcSettings: pro.OidcSettings{
			UserMapping:                   "USERNAME",
			JamfIDAuthenticationEnabled:   boolPtr(true),
			UsernameAttributeClaimMapping: strPtr("USERNAME"),
		},
		SamlSettings: pro.SamlSettings{
			IdpProviderType:         strPtr("OKTA"),
			EntityID:                strPtr("/saml/metadata"),
			MetadataSource:          strPtr("URL"),
			IdpURL:                  strPtr("https://idp.example.com/metadata"),
			SessionTimeout:          intPtr(240),
			TokenExpirationDisabled: boolPtr(false),
			UserMapping:             strPtr("EMAIL"),
			UserAttributeEnabled:    boolPtr(true),
			UserAttributeName:       strPtr("uid"),
			GroupAttributeName:      strPtr("http://schemas.xmlsoap.org/claims/Group"),
			GroupRdnKey:             strPtr("CN"),
		},
		EnrollmentSsoConfig: &pro.EnrollmentSsoConfig{
			Hosts:          &hosts,
			ManagementHint: strPtr("Sign in with Okta"),
		},
	}
}

// minimalCreatePlan is a configuration that declares only what the schema
// requires: the configuration type and the OIDC user mapping. Everything else is
// unknown, the shape of an omitted Optional+Computed attribute on a first apply.
func minimalCreatePlan() SsoSettingsResourceModel {
	return SsoSettingsResourceModel{
		SsoEnabled:                    unknownBoolPlan(),
		SsoBypassAllowed:              unknownBoolPlan(),
		SsoForEnrollmentEnabled:       unknownBoolPlan(),
		SsoForMacOsSelfServiceEnabled: unknownBoolPlan(),
		EnrollmentSsoForAccountDrivenEnrollmentEnabled: unknownBoolPlan(),
		GroupEnrollmentAccessEnabled:                   unknownBoolPlan(),
		GroupEnrollmentAccessName:                      types.StringNull(),
		ConfigurationType:                              types.StringValue("OIDC_WITH_SAML"),
		OidcSettings: &oidcSettingsModel{
			UserMapping:                   types.StringValue("EMAIL"),
			JamfIDAuthenticationEnabled:   unknownBoolPlan(),
			UsernameAttributeClaimMapping: unknownStringPlan(),
		},
	}
}

// TestBuildSsoSettingsInput_CreateAdoptsTheTenantsSettings pins #405. The PUT is
// a full replacement — an absent key and an explicit null both reset a field —
// so a body built from the plan alone wipes every setting the configuration is
// silent about on the first apply. With the live settings as the merge base,
// each of them is sent back unchanged.
func TestBuildSsoSettingsInput_CreateAdoptsTheTenantsSettings(t *testing.T) {
	got, diags := buildSsoSettingsInput(context.Background(), minimalCreatePlan(), configuredTenant())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if !got.SsoEnabled {
		t.Error("sso_enabled was not declared and must keep the tenant's true")
	}
	if !got.SsoBypassAllowed || !got.SsoForEnrollmentEnabled || !got.SsoForMacOsSelfServiceEnabled {
		t.Errorf("undeclared toggles must keep the tenant's values, got %+v", got)
	}
	if !got.EnrollmentSsoForAccountDrivenEnrollmentEnabled || !got.GroupEnrollmentAccessEnabled {
		t.Errorf("undeclared enrollment toggles must keep the tenant's values, got %+v", got)
	}
	if got.GroupEnrollmentAccessName == nil || *got.GroupEnrollmentAccessName != "Enrollment Admins" {
		t.Errorf("group_enrollment_access_name must be preserved, got %v", got.GroupEnrollmentAccessName)
	}
	if got.OidcSettings.UsernameAttributeClaimMapping == nil || *got.OidcSettings.UsernameAttributeClaimMapping != "USERNAME" {
		t.Errorf("an undeclared field inside a declared block must be preserved, got %v", got.OidcSettings.UsernameAttributeClaimMapping)
	}
	if got.EnrollmentSsoConfig == nil || got.EnrollmentSsoConfig.ManagementHint == nil || *got.EnrollmentSsoConfig.ManagementHint != "Sign in with Okta" {
		t.Errorf("an undeclared enrollment_sso_config must be preserved whole, got %+v", got.EnrollmentSsoConfig)
	}
}

// TestBuildSsoSettingsInput_UndeclaredSamlBlockIsPreserved covers the block the
// minimal plan omits entirely. A zero samlSettings is what used to go out, which
// on a SAML-capable tenant is every SAML field cleared at once.
func TestBuildSsoSettingsInput_UndeclaredSamlBlockIsPreserved(t *testing.T) {
	got, diags := buildSsoSettingsInput(context.Background(), minimalCreatePlan(), configuredTenant())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}

	if got.SamlSettings.IdpProviderType == nil || *got.SamlSettings.IdpProviderType != "OKTA" {
		t.Errorf("idp_provider_type must be preserved, got %v", got.SamlSettings.IdpProviderType)
	}
	if got.SamlSettings.GroupRdnKey == nil || *got.SamlSettings.GroupRdnKey != "CN" {
		t.Errorf("group_rdn_key must be preserved, got %v", got.SamlSettings.GroupRdnKey)
	}
	if got.SamlSettings.SessionTimeout == nil || *got.SamlSettings.SessionTimeout != 240 {
		t.Errorf("session_timeout must be preserved, got %v", got.SamlSettings.SessionTimeout)
	}
}

// TestBuildSsoSettingsInput_GroupRdnKeyPreservedInsideADeclaredBlock is the
// field the issue was filed against. Declaring saml_settings without
// group_rdn_key sent an explicit "" and cleared whatever RDN token the tenant
// held; it now falls back to that token like every sibling in the block.
func TestBuildSsoSettingsInput_GroupRdnKeyPreservedInsideADeclaredBlock(t *testing.T) {
	plan := minimalCreatePlan()
	plan.SamlSettings = &samlSettingsModel{
		IdpProviderType:         types.StringValue("OKTA"),
		OtherProviderTypeName:   types.StringNull(),
		EntityID:                types.StringValue("/saml/metadata"),
		MetadataSource:          types.StringValue("URL"),
		IdpURL:                  types.StringValue("https://idp.example.com/metadata"),
		FederationMetadataFile:  types.StringNull(),
		MetadataFileName:        unknownStringPlan(),
		SessionTimeout:          types.Int64Unknown(),
		TokenExpirationDisabled: unknownBoolPlan(),
		UserMapping:             types.StringValue("EMAIL"),
		UserAttributeEnabled:    unknownBoolPlan(),
		UserAttributeName:       unknownStringPlan(),
		GroupAttributeName:      types.StringValue("http://schemas.xmlsoap.org/claims/Group"),
		GroupRdnKey:             unknownStringPlan(),
	}

	got, diags := buildSsoSettingsInput(context.Background(), plan, configuredTenant())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.SamlSettings.GroupRdnKey == nil || *got.SamlSettings.GroupRdnKey != "CN" {
		t.Errorf("an omitted group_rdn_key must keep the tenant's token, got %v", got.SamlSettings.GroupRdnKey)
	}
	if got.SamlSettings.UserAttributeName == nil || *got.SamlSettings.UserAttributeName != "uid" {
		t.Errorf("an omitted user_attribute_name must keep the tenant's value, got %v", got.SamlSettings.UserAttributeName)
	}
	if got.SamlSettings.SessionTimeout == nil || *got.SamlSettings.SessionTimeout != 240 {
		t.Errorf("an omitted session_timeout must keep the tenant's value, got %v", got.SamlSettings.SessionTimeout)
	}
}

// TestBuildSsoSettingsInput_GroupRdnKeyIsNeverNull pins the wire constraint that
// made this field its own branch: Jamf Pro rejects a null groupRdnKey in SAML
// mode, so a tenant holding no token gets an explicit empty string.
func TestBuildSsoSettingsInput_GroupRdnKeyIsNeverNull(t *testing.T) {
	plan := minimalCreatePlan()
	plan.SamlSettings = &samlSettingsModel{
		MetadataSource: types.StringValue("URL"),
		IdpURL:         types.StringValue("https://idp.example.com/metadata"),
		GroupRdnKey:    unknownStringPlan(),
	}

	got, diags := buildSsoSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.SamlSettings.GroupRdnKey == nil {
		t.Fatal("group_rdn_key must never be sent as null")
	}
	if *got.SamlSettings.GroupRdnKey != "" {
		t.Errorf("group_rdn_key must be the empty string when neither plan nor tenant has one, got %q", *got.SamlSettings.GroupRdnKey)
	}
}

// TestBuildSsoSettingsInput_DeclaredValuesBeatTheMergeBase is the other half of
// the contract: the merge base only fills what the configuration is silent
// about, and an explicit empty string still clears.
func TestBuildSsoSettingsInput_DeclaredValuesBeatTheMergeBase(t *testing.T) {
	plan := minimalCreatePlan()
	plan.SsoEnabled = types.BoolValue(false)
	plan.GroupEnrollmentAccessName = types.StringValue("")
	plan.SamlSettings = &samlSettingsModel{
		MetadataSource: types.StringValue("URL"),
		IdpURL:         types.StringValue("https://other.example.com/metadata"),
		GroupRdnKey:    types.StringValue("OU"),
	}

	got, diags := buildSsoSettingsInput(context.Background(), plan, configuredTenant())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.SsoEnabled {
		t.Error("a declared sso_enabled = false must win over the tenant's true")
	}
	if got.GroupEnrollmentAccessName == nil || *got.GroupEnrollmentAccessName != "" {
		t.Errorf("an explicit empty string must clear, got %v", got.GroupEnrollmentAccessName)
	}
	if got.SamlSettings.GroupRdnKey == nil || *got.SamlSettings.GroupRdnKey != "OU" {
		t.Errorf("a declared group_rdn_key must win, got %v", got.SamlSettings.GroupRdnKey)
	}
	if got.SamlSettings.IdpURL == nil || *got.SamlSettings.IdpURL != "https://other.example.com/metadata" {
		t.Errorf("a declared idp_url must win, got %v", got.SamlSettings.IdpURL)
	}
}

// TestBuildSsoSettingsInput_MetadataMutexOutranksTheMergeBase guards the order
// the transforms run in. The tenant's stored FILE-mode metadata must not survive
// into a URL-mode body: the cached bytes have to arrive as null or they clash
// with the mode the configuration declares.
func TestBuildSsoSettingsInput_MetadataMutexOutranksTheMergeBase(t *testing.T) {
	current := configuredTenant()
	stored := []byte("<EntityDescriptor/>")
	fileName := "idp-metadata.xml"
	current.SamlSettings.FederationMetadataFile = &stored
	current.SamlSettings.MetadataFileName = &fileName

	plan := minimalCreatePlan()
	plan.SamlSettings = &samlSettingsModel{
		MetadataSource:         types.StringValue("URL"),
		IdpURL:                 types.StringValue("https://idp.example.com/metadata"),
		FederationMetadataFile: types.StringNull(),
		MetadataFileName:       unknownStringPlan(),
		GroupRdnKey:            unknownStringPlan(),
	}

	got, diags := buildSsoSettingsInput(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.SamlSettings.FederationMetadataFile != nil {
		t.Error("URL mode must clear the tenant's cached metadata file")
	}
	if got.SamlSettings.MetadataFileName != nil {
		t.Error("URL mode must clear the tenant's cached metadata file name")
	}
}

// TestBuildSsoSettingsInput_WithoutAMergeBaseTheBodyIsThePlanAlone reproduces
// the pre-fix body, which is what a nil current now means. It is the shape that
// reset every setting the configuration was silent about.
func TestBuildSsoSettingsInput_WithoutAMergeBaseTheBodyIsThePlanAlone(t *testing.T) {
	plan := minimalCreatePlan()
	plan.SsoEnabled = types.BoolValue(true)

	got, diags := buildSsoSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !got.SsoEnabled {
		t.Error("a declared sso_enabled must survive with no merge base")
	}
	if got.SsoBypassAllowed {
		t.Error("an unknown toggle with no merge base falls back to false, not to a tenant value")
	}
	if got.GroupEnrollmentAccessName != nil {
		t.Errorf("an undeclared optional string with no merge base stays null, got %v", got.GroupEnrollmentAccessName)
	}
}

// TestBuildSsoSettingsInput_OidcStubWhenTheTenantHoldsNothing keeps the stub
// that predates the merge base. oidcSettings.userMapping always serialises and
// Jamf Pro rejects it empty, so a pure-SAML configuration on a tenant with no
// stored OIDC block still needs a value.
func TestBuildSsoSettingsInput_OidcStubWhenTheTenantHoldsNothing(t *testing.T) {
	plan := minimalCreatePlan()
	plan.ConfigurationType = types.StringValue("SAML")
	plan.OidcSettings = nil

	got, diags := buildSsoSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.OidcSettings.UserMapping != pro.OidcSettingsUserMappingEmail {
		t.Errorf("expected the stub user mapping %q, got %q", pro.OidcSettingsUserMappingEmail, got.OidcSettings.UserMapping)
	}
}

// TestBuildSsoSettingsInput_UndeclaredOidcBlockIsPreserved is the same path with
// a tenant that does hold an OIDC block: it is carried across rather than
// replaced by the stub.
func TestBuildSsoSettingsInput_UndeclaredOidcBlockIsPreserved(t *testing.T) {
	plan := minimalCreatePlan()
	plan.ConfigurationType = types.StringValue("SAML")
	plan.OidcSettings = nil

	got, diags := buildSsoSettingsInput(context.Background(), plan, configuredTenant())
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.OidcSettings.UserMapping != "USERNAME" {
		t.Errorf("the tenant's user mapping must be preserved, got %q", got.OidcSettings.UserMapping)
	}
	if got.OidcSettings.JamfIDAuthenticationEnabled == nil || !*got.OidcSettings.JamfIDAuthenticationEnabled {
		t.Errorf("the tenant's Jamf ID toggle must be preserved, got %v", got.OidcSettings.JamfIDAuthenticationEnabled)
	}
}
