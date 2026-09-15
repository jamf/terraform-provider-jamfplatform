// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro Cloud Identity Provider endpoints
// (/v2/cloud-ldaps for Google, /v1/cloud-azure for Microsoft Entra ID). Google
// tests are gated behind three env vars because they require a real PKCS#12
// keystore issued by Google. Entra ID create-only tests run unconditionally
// since the server accepts the placeholder consent code and returns 201; Entra
// ID updates require a live-consented Entra connection and are intentionally
// excluded (see TestAccResource_ProCloudIdentityProvider_Azure for the full
// explanation).

package cloud_identity_provider_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// cipClient builds a Jamf Pro SDK client for out-of-band acceptance checks
// (CheckDestroy, drift simulation).
func cipClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

// testAccCheckCloudIdentityProviderDestroy asserts that every managed Cloud
// Identity Provider left in state is actually gone from the tenant after
// destroy — i.e. that Delete ran server-side. Data-source entries (which share
// the resource's type name) are skipped via the "data." address prefix. The
// unified registry GET (GetCloudIdpV1) covers both Google and Entra ID.
func testAccCheckCloudIdentityProviderDestroy(t *testing.T) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		c := cipClient(t)
		for addr, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_cloud_identity_provider" || strings.HasPrefix(addr, "data.") {
				continue
			}
			id := rs.Primary.Attributes["id"]
			if id == "" {
				continue
			}
			_, err := c.GetCloudIdpV1(context.Background(), id)
			if err == nil {
				return fmt.Errorf("Cloud Identity Provider %s still exists after destroy", id)
			}
			if !helpers.IsNotFoundError(err) {
				return fmt.Errorf("unexpected error verifying destroy of Cloud Identity Provider %s: %w", id, err)
			}
		}
		return nil
	}
}

// --- Google env-gate helpers ------------------------------------------------

const (
	envGoogleKeystoreBase64 = "JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_BASE64"
	envGooglePassword       = "JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_PASSWORD"
	envGoogleDomain         = "JAMFPLATFORM_ACC_PRO_GOOGLE_DOMAIN_NAME"
	// Optional: a SECOND, distinct keystore used by the rotation step to prove
	// that bumping wo_version with a different certificate re-derives the
	// server echoes (expiration_date/subject) without a "provider produced
	// inconsistent result" error. When unset, the rotation step re-uploads the
	// same certificate (still exercises the wo_version path, but does not guard
	// the echo-recompute regression).
	envGoogleKeystoreBase64Rotated = "JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_BASE64_ROTATED"
	envGooglePasswordRotated       = "JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_PASSWORD_ROTATED"
)

// requireGoogleEnv skips the test if any of the three Google-specific env vars
// are unset. Call this at the start of every Google lifecycle test.
func requireGoogleEnv(t *testing.T) (base64Keystore, password, domain string) {
	t.Helper()
	base64Keystore = testhelpers.AccEnv(envGoogleKeystoreBase64)
	password = testhelpers.AccEnv(envGooglePassword)
	domain = testhelpers.AccEnv(envGoogleDomain)
	if base64Keystore == "" || password == "" || domain == "" {
		t.Skipf(
			"skipping Google Cloud Identity Provider test: set %s, %s, and %s to run. "+
				"These must be a valid Google Secure LDAP PKCS#12 keystore (base64), its password, and the Google Apps domain.",
			envGoogleKeystoreBase64, envGooglePassword, envGoogleDomain,
		)
	}
	return
}

// --- Google lifecycle -------------------------------------------------------

// TestAccResource_ProCloudIdentityProvider_Google exercises the full Google
// Secure LDAP lifecycle:
//
//  1. Create with keystore (file from env, wo_version=1). Verify echoes
//     (type/subject/expiration_date) are known and server defaults are applied
//     (server_url, port, connection_type).
//  2. Update a non-replace field (search_timeout 60→90, use_wildcards
//     true→false) without bumping wo_version — keystore preserved.
//  3. Keystore re-upload: bump wo_version 1→2 — applies cleanly.
//  4. ImportState with WriteOnly attrs excluded from verification.
func TestAccResource_ProCloudIdentityProvider_Google(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ks64, pw, domain := requireGoogleEnv(t)
	suffix := testhelpers.RunSuffix()
	displayName := "tf-acc-cip-google-" + suffix

	const rn = "jamfplatform_pro_cloud_identity_provider.test"

	googleConfig := func(displayName, ks64, pw, domain string, searchTimeout int, useWildcards bool, woVersion int) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = %q
  provider_name = "GOOGLE"

  google = {
    server = {
      domain_name     = %q
      search_timeout  = %d
      use_wildcards   = %t

      keystore = {
        file       = %q
        password   = %q
        wo_version = %d
      }
    }
  }
}
`, displayName, domain, searchTimeout, useWildcards, ks64, pw, woVersion)
	}

	// Rotation target for step 3. When a distinct rotated keystore is supplied,
	// step 3 uploads it and asserts expiration_date changed — the regression
	// guard for the keystore-echo plan modifiers. Otherwise it re-uploads the
	// same certificate.
	rotKs := testhelpers.AccEnv(envGoogleKeystoreBase64Rotated)
	rotPw := testhelpers.AccEnv(envGooglePasswordRotated)
	differentCert := rotKs != "" && rotKs != ks64
	if rotKs == "" {
		rotKs = ks64
	}
	if rotPw == "" {
		rotPw = pw
	}

	// createExpiry captures the expiration_date echoed after the initial create
	// so the rotation step can assert it changed when a different cert is used.
	var createExpiry string

	rotationCheck := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(rn, "google.server.keystore.wo_version", "2"),
		resource.TestCheckResourceAttrSet(rn, "google.server.keystore.type"),
		resource.TestCheckResourceAttrSet(rn, "google.server.keystore.expiration_date"),
	}
	if differentCert {
		rotationCheck = append(rotationCheck, func(s *terraform.State) error {
			rs, ok := s.RootModule().Resources[rn]
			if !ok {
				return fmt.Errorf("resource %s not found in state", rn)
			}
			got := rs.Primary.Attributes["google.server.keystore.expiration_date"]
			if got == createExpiry {
				return fmt.Errorf("expiration_date did not change after rotating to a different certificate (still %q) — keystore echoes are being pinned to stale state", got)
			}
			return nil
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudIdentityProviderDestroy(t),
		Steps: []resource.TestStep{
			// Step 1: Create.
			{
				Config: googleConfig(displayName, ks64, pw, domain, 60, true, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(rn, "id"),
					resource.TestCheckResourceAttr(rn, "display_name", displayName),
					resource.TestCheckResourceAttr(rn, "provider_name", "GOOGLE"),
					// Server defaults.
					resource.TestCheckResourceAttr(rn, "google.server.server_url", "ldap.google.com"),
					resource.TestCheckResourceAttr(rn, "google.server.port", "636"),
					resource.TestCheckResourceAttr(rn, "google.server.connection_type", "LDAPS"),
					// Server-derived keystore echoes must be known (non-empty).
					resource.TestCheckResourceAttrSet(rn, "google.server.keystore.type"),
					resource.TestCheckResourceAttrSet(rn, "google.server.keystore.subject"),
					resource.TestCheckResourceAttrSet(rn, "google.server.keystore.expiration_date"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources[rn]
						if rs != nil {
							createExpiry = rs.Primary.Attributes["google.server.keystore.expiration_date"]
						}
						return nil
					},
				),
			},
			// Step 2: Update non-replace fields; wo_version unchanged → keystore preserved.
			{
				Config: googleConfig(displayName, ks64, pw, domain, 90, false, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "google.server.search_timeout", "90"),
					resource.TestCheckResourceAttr(rn, "google.server.use_wildcards", "false"),
				),
			},
			// Step 3: Keystore re-upload via wo_version bump (1→2). Uses a distinct
			// certificate when one is supplied (then asserts expiration_date changed).
			{
				Config: googleConfig(displayName, rotKs, rotPw, domain, 90, false, 2),
				Check:  resource.ComposeAggregateTestCheckFunc(rotationCheck...),
			},
			// Step 4: ImportState — WriteOnly + rotation trigger are unrecoverable,
			// and mappings are hydrated in full by an import (#391) while this
			// config declares none, so the two legitimately differ.
			{
				ResourceName:      rn,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"google.server.keystore.file",
					"google.server.keystore.password",
					"google.server.keystore.wo_version",
					"google.mappings",
				},
			},
		},
	})
}

// --- Google plan-time validation (no env gate — fails at plan, never applies) --

// TestAccResource_ProCloudIdentityProvider_Google_AzureBlockForbidden verifies
// that supplying an entra_id block when provider_name=GOOGLE triggers a
// plan-time error. No keystore env vars needed — the validator fires before any
// API call.
func TestAccResource_ProCloudIdentityProvider_Google_AzureBlockForbidden(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = "tf-acc-cip-google-forbidden"
  provider_name = "GOOGLE"

  google = {
    server = {
      domain_name = "example.com"
      keystore    = {}
    }
  }

  entra_id = {
    tenant_id = "d5749c84-5cc5-4691-a187-4545c02ff915"
  }
}
`,
				ExpectError: regexp.MustCompile(`forbidden`),
			},
		},
	})
}

// TestAccResource_ProCloudIdentityProvider_Google_KeystoreFileRequiresPassword
// verifies that supplying keystore.file without keystore.password triggers the
// AlsoRequires plan-time error (Invalid Attribute Combination).
func TestAccResource_ProCloudIdentityProvider_Google_KeystoreFileRequiresPassword(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = "tf-acc-cip-google-no-password"
  provider_name = "GOOGLE"

  google = {
    server = {
      domain_name = "example.com"
      keystore = {
        file = "dGVzdA=="
        # password intentionally omitted
      }
    }
  }
}
`,
				ExpectError: regexp.MustCompile(`Combination`),
			},
		},
	})
}

// --- Azure lifecycle ---------------------------------------------------------

// TestAccResource_ProCloudIdentityProvider_Azure exercises the Microsoft Entra
// ID Cloud Identity Provider create-and-import lifecycle.
//
// Why no update TestStep: the Entra ID PUT endpoint (/v1/cloud-azure/:id)
// returns 400 INVALID_CONNECTION unless a real consented Entra connection
// exists (the consent flow is a manual UI step in Jamf Pro). This test creates
// and reads the provider, confirms the server-derived echoes (type/migrated/
// deprecated_consent) are populated, and then exercises ImportState. The update
// step is omitted by mutual agreement with maintainers — it would require a
// live Entra tenant with active consent, which is outside the CI test boundary.
// See TestAccResource_ProCloudIdentityProvider_Azure_UpdateSkipped for the
// explicit skip placeholder.
func TestAccResource_ProCloudIdentityProvider_Azure(t *testing.T) {
	t.Log("Entra ID update round-trip intentionally omitted: PUT returns 400 INVALID_CONNECTION without a real consented Entra connection — see resource docs")
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	displayName := "tf-acc-cip-azure-" + suffix

	const rn = "jamfplatform_pro_cloud_identity_provider.test"

	azureConfig := fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = %q
  provider_name = "ENTRA_ID"

  entra_id = {
    tenant_id = "d5749c84-5cc5-4691-a187-4545c02ff915"
  }
}
`, displayName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudIdentityProviderDestroy(t),
		Steps: []resource.TestStep{
			// Step 1: Create — the placeholder consent code is accepted by the
			// server (201); the connection is inactive until the admin completes
			// the manual "refresh consent" step in the Jamf Pro UI.
			{
				Config: azureConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(rn, "id"),
					resource.TestCheckResourceAttr(rn, "display_name", displayName),
					resource.TestCheckResourceAttr(rn, "provider_name", "ENTRA_ID"),
					// Server-derived echoes must be populated.
					resource.TestCheckResourceAttrSet(rn, "entra_id.type"),
					resource.TestCheckResourceAttrSet(rn, "entra_id.migrated"),
					resource.TestCheckResourceAttrSet(rn, "entra_id.deprecated_consent"),
				),
			},
			// ImportState — no WriteOnly attrs on Entra ID, but mappings are
			// hydrated in full by an import (#391) and this config declares
			// none, so the two legitimately differ.
			{
				ResourceName:            rn,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "entra_id.mappings"},
			},
		},
	})
}

// TestAccResource_ProCloudIdentityProvider_Azure_UpdateSkipped is an explicit
// skip placeholder. It appears in test output so the omission is visible and
// documented rather than silent.
//
// One consequence is worth naming, because it is the reason #404 was fixed
// blind: the mappings-preserving merge base the Entra update now builds has no
// acceptance coverage and cannot have any here. The refusal lands before
// persistence — probed 2026-09-08, an update sending eleven empty mappings to an
// unconsented connection left the stored ones byte identical — so nothing on
// this estate can observe either the erasure or the fix. The unit tests over
// buildAzureMappings are the whole guard.
func TestAccResource_ProCloudIdentityProvider_Azure_UpdateSkipped(t *testing.T) {
	t.Skip("Azure update requires a real consented Entra connection; PUT 400 INVALID_CONNECTION otherwise — see resource docs")
}

// TestAccResource_ProCloudIdentityProvider_GoogleMappings exercises the inline
// attribute-mapping path that the lifecycle test omits: a fully-authored
// mappings block (all three sub-blocks) on create, plus a mapping-field update.
// This is the code that carried the Optional-only nested-object fix and the
// prior-presence state preservation, so it is worth its own coverage.
//
// Import note: mappings cannot round-trip on import. With no prior state the
// provider cannot tell whether the user authored the block (the server always
// returns generated mappings), so it leaves mappings null on import; the import
// step therefore ignores `google.mappings` (same treatment as the WriteOnly
// keystore attrs). Users re-declare mappings in config after an import.
func TestAccResource_ProCloudIdentityProvider_GoogleMappings(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ks64, pw, domain := requireGoogleEnv(t)
	suffix := testhelpers.RunSuffix()
	displayName := "tf-acc-cip-gmap-" + suffix
	const rn = "jamfplatform_pro_cloud_identity_provider.test"

	cfg := func(userID string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = %q
  provider_name = "GOOGLE"

  google = {
    server = {
      domain_name = %q
      keystore = {
        file       = %q
        password   = %q
        wo_version = 1
      }
    }

    mappings = {
      user_mappings = {
        object_class_limitation = "ANY_OBJECT_CLASSES"
        object_classes          = "inetOrgPerson"
        search_base             = "ou=Users"
        search_scope            = "ALL_SUBTREES"
        # When mappings is overridden the server validates additional_search_base
        # as an LDAP DN — it must be a valid DN (not empty) on write.
        additional_search_base = "ou=Users"
        user_id                = %q
        username               = "uid"
        real_name              = "displayName"
        email_address          = "mail"
      }
      group_mappings = {
        object_class_limitation = "ANY_OBJECT_CLASSES"
        object_classes          = "groupOfNames"
        search_base             = "ou=Groups"
        search_scope            = "ALL_SUBTREES"
        group_id                = "cn"
        group_name              = "cn"
      }
      membership_mappings = {
        group_membership_mapping = "memberOf"
      }
    }
  }
}
`, displayName, domain, ks64, pw, userID)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudIdentityProviderDestroy(t),
		Steps: []resource.TestStep{
			// Create with a fully-authored mappings block; assert the echoes.
			{
				Config: cfg("mail"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "google.mappings.user_mappings.user_id", "mail"),
					resource.TestCheckResourceAttr(rn, "google.mappings.user_mappings.object_classes", "inetOrgPerson"),
					resource.TestCheckResourceAttr(rn, "google.mappings.group_mappings.group_name", "cn"),
					resource.TestCheckResourceAttr(rn, "google.mappings.membership_mappings.group_membership_mapping", "memberOf"),
				),
			},
			// Update a single mapping field — exercises the inline-mappings PUT path.
			{
				Config: cfg("uid"),
				Check:  resource.TestCheckResourceAttr(rn, "google.mappings.user_mappings.user_id", "uid"),
			},
			// Import — mappings hydrates in full (#391), while this config declares
			// only user_mappings, so the sibling sub-blocks legitimately differ.
			{
				ResourceName:      rn,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"google.server.keystore.file",
					"google.server.keystore.password",
					"google.server.keystore.wo_version",
					"google.mappings",
				},
			},
		},
	})
}

// TestAccResource_ProCloudIdentityProvider_Azure_TenantIDForcesReplace verifies
// the tenant_id RequiresReplace plan modifier: the Entra ID update endpoint
// cannot change the tenant, so a tenant_id change must destroy-and-recreate
// (new server id), not attempt an in-place update. No env vars needed — the
// placeholder consent code makes both creates return 201.
func TestAccResource_ProCloudIdentityProvider_Azure_TenantIDForcesReplace(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	displayName := "tf-acc-cip-azrepl-" + suffix
	const rn = "jamfplatform_pro_cloud_identity_provider.test"

	cfg := func(tenantID string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = %q
  provider_name = "ENTRA_ID"

  entra_id = {
    tenant_id = %q
  }
}
`, displayName, tenantID)
	}

	var firstID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudIdentityProviderDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg("d5749c84-5cc5-4691-a187-4545c02ff915"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(rn, "id"),
					func(s *terraform.State) error {
						if rs := s.RootModule().Resources[rn]; rs != nil {
							firstID = rs.Primary.Attributes["id"]
						}
						return nil
					},
				),
			},
			{
				// Different (valid v4) tenant_id → must force replacement.
				Config: cfg("a1b2c3d4-1111-4222-8333-444455556666"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "entra_id.tenant_id", "a1b2c3d4-1111-4222-8333-444455556666"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources[rn]
						if rs == nil {
							return fmt.Errorf("resource %s not found in state", rn)
						}
						if got := rs.Primary.Attributes["id"]; got == firstID {
							return fmt.Errorf("expected replacement (new id) after tenant_id change, but id is unchanged (%s)", got)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccResource_ProCloudIdentityProvider_Azure_DriftRecovery verifies that an
// out-of-band deletion is detected on the next refresh: Read returns not-found,
// the resource is removed from state, and the follow-up plan is non-empty
// (Terraform wants to recreate it). Exercises the readAzure NotFound →
// RemoveResource path (readGoogle is structurally identical).
func TestAccResource_ProCloudIdentityProvider_Azure_DriftRecovery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	displayName := "tf-acc-cip-azdrift-" + suffix
	const rn = "jamfplatform_pro_cloud_identity_provider.test"

	cfg := fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "test" {
  display_name  = %q
  provider_name = "ENTRA_ID"

  entra_id = {
    tenant_id = "d5749c84-5cc5-4691-a187-4545c02ff915"
  }
}
`, displayName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudIdentityProviderDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(rn, "id"),
					// Delete the record out-of-band so the next refresh sees it gone.
					func(s *terraform.State) error {
						rs := s.RootModule().Resources[rn]
						if rs == nil {
							return fmt.Errorf("resource %s not found in state", rn)
						}
						id := rs.Primary.Attributes["id"]
						if err := cipClient(t).DeleteCloudAzureV1(context.Background(), id); err != nil {
							return fmt.Errorf("out-of-band delete of %s failed: %w", id, err)
						}
						return nil
					},
				),
				// The post-step refresh + plan must detect the deletion and want to
				// recreate the resource.
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
