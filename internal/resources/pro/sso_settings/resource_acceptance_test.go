// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package sso_settings_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// Acceptance tests keep the tenant's SSO record in an OIDC or OIDC_WITH_SAML
// configuration with `sso_enabled = true` throughout. Pure SAML mode is
// avoided so admin Jamf ID login keeps working — the Platform API depends
// on SSO remaining enabled for cross-product calls. The Delete handler on
// jamfplatform_pro_sso_settings is state-only by design; CheckDestroy hooks
// verify the record persists with `sso_enabled = true`.
//
// Tests that need a SAML IdP URL or metadata file are gated behind env
// vars:
//   - JAMFPLATFORM_ACC_PRO_SSO_IDP_URL: SAML metadata URL (Okta trial fine)
//   - JAMFPLATFORM_ACC_PRO_SSO_METADATA_BASE64: base64 of Google IdP metadata
//     XML (the IdP type must differ from any previously-stored URL to
//     trigger a real switch on the tenant).
const (
	envSsoIdpURL         = "JAMFPLATFORM_ACC_PRO_SSO_IDP_URL"
	envSsoMetadataBase64 = "JAMFPLATFORM_ACC_PRO_SSO_METADATA_BASE64"
)

// requireIdpURL skips the test when the IdP URL env var is unset.
func requireIdpURL(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv(envSsoIdpURL)
	if v == "" {
		t.Skipf("skipping: set %s to a SAML IdP metadata URL to exercise URL-mode SAML tests", envSsoIdpURL)
	}
	return v
}

// requireMetadataBase64 skips the test when the metadata env var is unset.
func requireMetadataBase64(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv(envSsoMetadataBase64)
	if v == "" {
		t.Skipf("skipping: set %s to a base64-encoded SAML metadata XML to exercise FILE-mode SAML tests", envSsoMetadataBase64)
	}
	return v
}

// checkSsoStillEnabledAfterDestroy verifies Delete did not touch the SSO
// record on the tenant. The Delete handler is documented as state-only so
// the record's `sso_enabled` must remain `true` after destroy.
func checkSsoStillEnabledAfterDestroy(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetSsoSettingsV3(context.Background())
		if err != nil {
			return fmt.Errorf("expected SSO settings record to persist after destroy: %w", err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil SSO settings post-destroy")
		}
		if !got.SsoEnabled {
			return fmt.Errorf("expected sso_enabled=true to persist after state-only destroy, got false")
		}
		return nil
	}
}

// checkCertSetupType verifies the current /v2/sso/cert setup type matches `want`.
func checkCertSetupType(t *testing.T, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetSsoCertificateV2(context.Background())
		if err != nil {
			return fmt.Errorf("error reading cert: %w", err)
		}
		var actual string
		if got != nil && got.Keystore != nil {
			actual = got.Keystore.KeystoreSetupType
		}
		if actual == "" {
			actual = "NONE"
		}
		if actual != want {
			return fmt.Errorf("cert setup type = %q, want %q", actual, want)
		}
		return nil
	}
}

// oidcEnabledConfig is the minimum-viable OIDC+enabled config used as the
// baseline for tests that swap into and out of OIDC_WITH_SAML.
const oidcEnabledConfig = `
	resource "jamfplatform_pro_sso_settings" "test" {
		sso_enabled                                          = true
		# The SSO feature flags (enrollment, macOS Self Service, account-driven
		# enrollment, group access) are SAML-only: in pure OIDC mode writing them
		# is a no-op — Jamf Pro keeps and returns whatever the tenant already has
		# (false on a never-SAML tenant, true on one with residual SAML metadata).
		# Omit them so the Optional+Computed values absorb the tenant's value
		# instead of forcing a plan-vs-apply mismatch in either direction.
		configuration_type                                   = "OIDC"
		oidc_settings = {
			user_mapping                   = "EMAIL"
			jamf_id_authentication_enabled = true
		}
	}
`

// TestAccResource_ProSsoSettings_DisableEnable toggles sso_enabled false
// and then back to true. Verifies the disabled-SSO path works and that
// re-enabling restores admin access. Always ends with sso_enabled=true so
// the tenant's Platform API access (which depends on SSO being enabled)
// is not left broken.
func TestAccResource_ProSsoSettings_DisableEnable(t *testing.T) {
	testhelpers.AccPreCheck(t)

	// sso_for_enrollment_enabled and other bools are Optional+Computed so
	// the tenant's stored values flow through unchanged. Only sso_enabled
	// is exercised explicitly here.
	const disabled = `
		resource "jamfplatform_pro_sso_settings" "test" {
			sso_enabled                                          = false
			# OIDC-only: SSO feature flags are a no-op; omit (see oidcEnabledConfig).
			configuration_type                                   = "OIDC"
			oidc_settings = {
				user_mapping                   = "EMAIL"
				jamf_id_authentication_enabled = true
			}
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: disabled,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "sso_enabled", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "configuration_type", "OIDC"),
				),
			},
			// Restore enabled state so the tenant stays usable.
			{
				Config: oidcEnabledConfig,
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "sso_enabled", "true"),
			},
		},
	})
}

// TestAccResource_ProSsoSettings_OIDC_Baseline exercises the OIDC mode
// the rest of the suite re-applies to leave the tenant in a known state
// after each test.
func TestAccResource_ProSsoSettings_OIDC_Baseline(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcEnabledConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "id", "singleton"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "sso_enabled", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "configuration_type", "OIDC"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "oidc_settings.user_mapping", "EMAIL"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "oidc_settings.jamf_id_authentication_enabled", "true"),
				),
			},
		},
	})
}

// TestAccResource_ProSsoSettings_CreateAdoptsTheTenantsSettings pins #405. The
// PUT replaces the whole configuration and an omitted field arrives as null,
// which resets it — wire-probed 2026-09-08 on this very field. Update was hidden
// from that by UseStateForUnknown, but a first apply has no prior state, so
// every setting the configuration did not declare was wiped from a tenant that
// was already configured. Create now reads the settings and merges the plan over
// them.
//
// The field is set out of band first, because the point is a value Terraform has
// never held. oidcEnabledConfig does not declare
// username_attribute_claim_mapping, so a create that resets it is visible both
// in state and on the tenant. A t.Cleanup puts the tenant's original settings
// back whichever way the test ends.
func TestAccResource_ProSsoSettings_CreateAdoptsTheTenantsSettings(t *testing.T) {
	testhelpers.AccPreCheck(t)
	setUsernameClaimMappingOutOfBand(t, "USERNAME")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcEnabledConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "oidc_settings.username_attribute_claim_mapping", "USERNAME"),
					checkUsernameClaimMapping(t, "USERNAME"),
				),
			},
		},
	})
}

// setUsernameClaimMappingOutOfBand writes the OIDC username claim mapping the
// way an administrator would, and registers the restore of the tenant's
// original settings. It fails the test if the write did not take, since a value
// that never landed would let the adoption test pass without proving anything.
func setUsernameClaimMappingOutOfBand(t *testing.T, want string) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	before, err := c.GetSsoSettingsV3(ctx)
	if err != nil {
		t.Fatalf("reading the tenant's SSO settings: %v", err)
	}
	if before == nil {
		t.Fatal("reading the tenant's SSO settings: no settings returned")
	}

	restore := *before
	t.Cleanup(func() {
		if _, err := c.UpdateSsoSettingsV3(context.Background(), &restore); err != nil {
			t.Logf("restoring the tenant's SSO settings: %v", err)
		}
	})

	edited := *before
	edited.OidcSettings.UsernameAttributeClaimMapping = &want
	if _, err := c.UpdateSsoSettingsV3(ctx, &edited); err != nil {
		t.Fatalf("setting the username claim mapping outside Terraform: %v", err)
	}

	after, err := c.GetSsoSettingsV3(ctx)
	if err != nil {
		t.Fatalf("re-reading the tenant's SSO settings: %v", err)
	}
	if after.OidcSettings.UsernameAttributeClaimMapping == nil || *after.OidcSettings.UsernameAttributeClaimMapping != want {
		t.Fatalf("the out-of-band write did not take: username claim mapping is %v, want %q", after.OidcSettings.UsernameAttributeClaimMapping, want)
	}
}

// checkUsernameClaimMapping asserts the tenant still reports the given OIDC
// username claim mapping.
func checkUsernameClaimMapping(t *testing.T, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetSsoSettingsV3(context.Background())
		if err != nil {
			return fmt.Errorf("reading SSO settings: %w", err)
		}
		if got.OidcSettings.UsernameAttributeClaimMapping == nil {
			return fmt.Errorf("username claim mapping was cleared; want %q", want)
		}
		if *got.OidcSettings.UsernameAttributeClaimMapping != want {
			return fmt.Errorf("username claim mapping = %q, want %q", *got.OidcSettings.UsernameAttributeClaimMapping, want)
		}
		return nil
	}
}

// TestAccResource_ProSsoSettings_OIDC_WithSAML_URL configures the hybrid
// OIDC_WITH_SAML mode with metadata_source = URL. Gated by
// JAMFPLATFORM_ACC_PRO_SSO_IDP_URL. Returns to OIDC at the end so subsequent
// suites start from a clean baseline.
func TestAccResource_ProSsoSettings_OIDC_WithSAML_URL(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireIdpURL(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_sso_settings" "test" {
						sso_enabled                                          = true
							sso_for_enrollment_enabled                           = true
						sso_for_macos_self_service_enabled                   = false
						enrollment_sso_for_account_driven_enrollment_enabled = false
						group_enrollment_access_enabled                      = false
						configuration_type                                   = "OIDC_WITH_SAML"
						oidc_settings = {
							user_mapping                   = "EMAIL"
							jamf_id_authentication_enabled = true
						}
						saml_settings = {
							idp_provider_type    = "OKTA"
							entity_id            = "/saml/metadata"
							metadata_source      = "URL"
							idp_url              = %q
							session_timeout      = 480
							user_mapping         = "EMAIL"
							group_attribute_name = "http://schemas.xmlsoap.org/claims/Group"
						}
					}
				`, idpURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "configuration_type", "OIDC_WITH_SAML"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "saml_settings.idp_provider_type", "OKTA"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "saml_settings.metadata_source", "URL"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "saml_settings.idp_url", idpURL),
				),
			},
			// Restore baseline.
			{Config: oidcEnabledConfig},
		},
	})
}

// TestAccResource_ProSsoSettings_OIDC_WithSAML_File configures the hybrid
// OIDC_WITH_SAML mode with metadata_source = FILE. Gated by
// JAMFPLATFORM_ACC_PRO_SSO_METADATA_BASE64 (must be Google IdP metadata so the
// transition triggers a genuine IdP-type switch on the tenant).
func TestAccResource_ProSsoSettings_OIDC_WithSAML_File(t *testing.T) {
	testhelpers.AccPreCheck(t)
	metadataB64 := requireMetadataBase64(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_sso_settings" "test" {
						sso_enabled                                          = true
							sso_for_enrollment_enabled                           = true
						sso_for_macos_self_service_enabled                   = false
						enrollment_sso_for_account_driven_enrollment_enabled = false
						group_enrollment_access_enabled                      = false
						configuration_type                                   = "OIDC_WITH_SAML"
						oidc_settings = {
							user_mapping                   = "EMAIL"
							jamf_id_authentication_enabled = true
						}
						saml_settings = {
							idp_provider_type        = "GOOGLE"
							entity_id                = "/saml/metadata"
							metadata_source          = "FILE"
							federation_metadata_file = %q
							metadata_file_name       = "idp-metadata.xml"
							user_mapping             = "EMAIL"
							group_attribute_name     = "http://schemas.xmlsoap.org/claims/Group"
						}
					}
				`, metadataB64),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "configuration_type", "OIDC_WITH_SAML"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "saml_settings.idp_provider_type", "GOOGLE"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "saml_settings.metadata_source", "FILE"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "saml_settings.metadata_file_name", "idp-metadata.xml"),
				),
			},
			{Config: oidcEnabledConfig},
		},
	})
}

// TestAccResource_ProSsoSettings_BypassAllowedValidator exercises the
// value-specific validator on sso_bypass_allowed.
func TestAccResource_ProSsoSettings_BypassAllowedValidator(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_sso_settings" "test" {
						sso_enabled                                          = true
						sso_bypass_allowed                                   = true
						configuration_type                                   = "OIDC"
						oidc_settings = {
							user_mapping = "EMAIL"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`sso_bypass_allowed = true requires configuration_type to include SAML`),
			},
		},
	})
}

// TestAccResource_ProSsoSettings_GroupEnrollmentValidator exercises the
// value-specific validator on group_enrollment_access_enabled. Group enrollment
// access (and sso_for_enrollment) are SAML-only, so the name-required rule is
// only reachable in OIDC_WITH_SAML mode — hence the IdP-URL gate.
func TestAccResource_ProSsoSettings_GroupEnrollmentValidator(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireIdpURL(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_sso_settings" "test" {
						sso_enabled                                          = true
						sso_for_enrollment_enabled                           = true
						sso_for_macos_self_service_enabled                   = false
						enrollment_sso_for_account_driven_enrollment_enabled = false
						group_enrollment_access_enabled                      = true
						# group_enrollment_access_name intentionally omitted.
						configuration_type                                   = "OIDC_WITH_SAML"
						oidc_settings = {
							user_mapping = "EMAIL"
						}
						saml_settings = {
							entity_id            = "/saml/metadata"
							metadata_source      = "URL"
							idp_url              = %q
							group_attribute_name = "http://schemas.xmlsoap.org/claims/Group"
						}
					}
				`, idpURL),
				ExpectError: regexp.MustCompile(`group_enrollment_access_name required`),
			},
		},
	})
}

// TestAccResource_ProSsoSettings_MetadataSourceMutex exercises the URL/FILE
// branch mutex validator in OIDC_WITH_SAML mode.
func TestAccResource_ProSsoSettings_MetadataSourceMutex(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireIdpURL(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_sso_settings" "test" {
						sso_enabled                                          = true
							sso_for_enrollment_enabled                           = true
						sso_for_macos_self_service_enabled                   = false
						enrollment_sso_for_account_driven_enrollment_enabled = false
						group_enrollment_access_enabled                      = false
						configuration_type                                   = "OIDC_WITH_SAML"
						oidc_settings = {
							user_mapping = "EMAIL"
						}
						saml_settings = {
							entity_id                = "/saml/metadata"
							metadata_source          = "URL"
							idp_url                  = %q
							federation_metadata_file = "Zm9v"
							group_attribute_name     = "http://schemas.xmlsoap.org/claims/Group"
						}
					}
				`, idpURL),
				ExpectError: regexp.MustCompile(`federation_metadata_file forbidden when metadata_source = "URL"`),
			},
		},
	})
}

// TestAccResource_ProSsoSettings_OIDCForbidsSAMLBlock verifies the
// configuration_type=OIDC validator rejects a samlSettings sibling.
func TestAccResource_ProSsoSettings_OIDCForbidsSAMLBlock(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_sso_settings" "test" {
						sso_enabled                                          = true
						configuration_type                                   = "OIDC"
						oidc_settings = {
							user_mapping = "EMAIL"
						}
						saml_settings = {
							entity_id            = "/saml/metadata"
							group_attribute_name = "http://schemas.xmlsoap.org/claims/Group"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`saml_settings forbidden when configuration_type = "OIDC"`),
			},
		},
	})
}

// TestAccResource_ProSsoSettings_CertGenerated exercises GENERATED cert
// creation and confirms a re-apply is a no-op (no new serial issued).
func TestAccResource_ProSsoSettings_CertGenerated(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const cfg = `
		resource "jamfplatform_pro_sso_settings" "test" {
			sso_enabled                                          = true
			# OIDC-only: SSO feature flags are a no-op; omit (see oidcEnabledConfig).
			configuration_type                                   = "OIDC"
			oidc_settings = {
				user_mapping                   = "EMAIL"
				jamf_id_authentication_enabled = true
			}
			signing_certificate = {
				setup_type = "GENERATED"
			}
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCertSetupType(t, "GENERATED"),
					resource.TestCheckResourceAttr("jamfplatform_pro_sso_settings.test", "signing_certificate.setup_type", "GENERATED"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_sso_settings.test", "signing_certificate.serial_number"),
				),
			},
			{
				Config: cfg,
				Check:  checkCertSetupType(t, "GENERATED"),
			},
			{Config: oidcEnabledConfig},
		},
	})
}

// TestAccResource_ProSsoSettings_CertTransition_GeneratedToNone exercises
// the cert sub-block DELETE path.
func TestAccResource_ProSsoSettings_CertTransition_GeneratedToNone(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const base = `
		resource "jamfplatform_pro_sso_settings" "test" {
			sso_enabled                                          = true
			# OIDC-only: SSO feature flags are a no-op; omit (see oidcEnabledConfig).
			configuration_type                                   = "OIDC"
			oidc_settings = {
				user_mapping                   = "EMAIL"
				jamf_id_authentication_enabled = true
			}
			signing_certificate = {
				setup_type = %q
			}
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSsoStillEnabledAfterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(base, "GENERATED"),
				Check:  checkCertSetupType(t, "GENERATED"),
			},
			{
				Config: fmt.Sprintf(base, "NONE"),
				Check:  checkCertSetupType(t, "NONE"),
			},
			{Config: oidcEnabledConfig},
		},
	})
}

// TestAccResource_ProSsoSettings_RejectsNonSingletonImport verifies the
// import-id guard.
func TestAccResource_ProSsoSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: oidcEnabledConfig},
			{
				ResourceName:  "jamfplatform_pro_sso_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccDataSource_ProSsoSettings_Basic exercises the DS.
func TestAccDataSource_ProSsoSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: oidcEnabledConfig + `
					data "jamfplatform_pro_sso_settings" "ds" {
						depends_on = [jamfplatform_pro_sso_settings.test]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_settings.ds", "id", "singleton"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_settings.ds", "configuration_type", "OIDC"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_settings.ds", "sso_enabled", "true"),
				),
			},
		},
	})
}

// TestAccDataSource_ProSsoDependencies exercises the dependencies DS.
func TestAccDataSource_ProSsoDependencies(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "jamfplatform_pro_sso_dependencies" "ds" {}`,
				Check:  resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_dependencies.ds", "id", "singleton"),
			},
		},
	})
}

// TestAccDataSource_ProSsoSpMetadata_OIDCWarning verifies the warning path
// when the tenant has no SAML configuration.
func TestAccDataSource_ProSsoSpMetadata_OIDCWarning(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: oidcEnabledConfig + `
					data "jamfplatform_pro_sso_sp_metadata" "ds" {
						depends_on = [jamfplatform_pro_sso_settings.test]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_sp_metadata.ds", "id", "singleton"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_sp_metadata.ds", "xml", ""),
				),
			},
		},
	})
}

// TestAccDataSource_ProSsoSpMetadata_SAML verifies the DS returns SP
// metadata when the tenant is configured with OIDC_WITH_SAML.
func TestAccDataSource_ProSsoSpMetadata_SAML(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireIdpURL(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_sso_settings" "src" {
						sso_enabled                                          = true
							sso_for_enrollment_enabled                           = true
						sso_for_macos_self_service_enabled                   = false
						enrollment_sso_for_account_driven_enrollment_enabled = false
						group_enrollment_access_enabled                      = false
						configuration_type                                   = "OIDC_WITH_SAML"
						oidc_settings = {
							user_mapping                   = "EMAIL"
							jamf_id_authentication_enabled = true
						}
						saml_settings = {
							idp_provider_type    = "OKTA"
							entity_id            = "/saml/metadata"
							metadata_source      = "URL"
							idp_url              = %q
							group_attribute_name = "http://schemas.xmlsoap.org/claims/Group"
							user_mapping         = "EMAIL"
						}
					}
					data "jamfplatform_pro_sso_sp_metadata" "ds" {
						depends_on = [jamfplatform_pro_sso_settings.src]
					}
				`, idpURL),
				Check: resource.TestCheckResourceAttr("data.jamfplatform_pro_sso_sp_metadata.ds", "id", "singleton"),
			},
			// Restore baseline so the next test starts from a known state.
			{Config: oidcEnabledConfig},
		},
	})
}
