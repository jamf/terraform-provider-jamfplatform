// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package enrollment_customization_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// envSsoIdpURL gates the enrollment-customization tests that create a real SSO
// pane. Jamf Pro rejects an SSO panel unless SAML is configured on the tenant
// ("[INVALID_STATE] : SAML must be configured for the JSS to use the SSO
// panel"), so those tests stand up an sso_settings SAML fixture from this URL
// and depend_on it. Set it to a SAML IdP metadata URL (an Okta trial works).
const envSsoIdpURL = "JAMFPLATFORM_ACC_PRO_SSO_IDP_URL"

// requireSsoIdpURL skips an SSO-pane test unless a SAML IdP metadata URL is set.
func requireSsoIdpURL(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv(envSsoIdpURL)
	if v == "" {
		t.Skipf("skipping SSO-pane enrollment-customization test: set %s to a SAML IdP metadata URL so the test can configure an OIDC_WITH_SAML sso_settings fixture (Jamf Pro requires SAML configured before an SSO panel can be created)", envSsoIdpURL)
	}
	return v
}

// ssoSamlFixture returns an sso_settings resource (Terraform name "ec_fixture")
// in OIDC_WITH_SAML mode: SAML is configured so the SSO panel can be created,
// while OIDC + Jamf ID login is preserved so admin access keeps working. SSO-pane
// tests depend_on this so SAML lands before the panel POST. The sso_settings
// Delete is state-only, so teardown leaves the tenant in OIDC_WITH_SAML — the
// documented steady state for SSO acceptance tests; re-applying is idempotent.
func ssoSamlFixture(idpURL string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_sso_settings" "ec_fixture" {
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
`, idpURL)
}

// LDAP-pane tests need a directory service configured on the tenant, or Jamf Pro
// rejects the LDAP/Directory Service panel ("[INVALID_STATE] Directory service
// must be configured for Jamf Pro to use the Directory Service panel"). They stand
// up the shared Okta LDAP server fixture (testhelpers.LdapServerFixture, labelled
// acc_ldap) and depend_on it.

// fixtureIconPath returns the absolute path to the committed 100x100 PNG
// fixture used by icon-related acceptance tests.
func fixtureIconPath(t *testing.T) string {
	return fixturePath(t, "icon.png")
}

// fixtureAltIconPath returns the absolute path to a second committed PNG
// (distinct bytes) used to assert icon-drift behaviour on Update.
func fixtureAltIconPath(t *testing.T) string {
	return fixturePath(t, "icon_alt.png")
}

// fixtureIconHash returns the canonical hash of a committed fixture, so a test
// can assert the value the provider arrived at rather than only its format.
func fixtureIconHash(t *testing.T, path string) string {
	t.Helper()
	return files.ComputeContentSHA256(readIconFixture(t, path))
}

// unstableIconServer answers every request for one path with a different one of
// the two committed PNGs, the way Apple's iTunes artwork CDN answers a fixed
// URL. The returned counter records how many times the provider fetched it, so
// a test can prove the provider read the source once rather than once per plan.
//
// The provider runs in this process, so a loopback server is reachable from it.
func unstableIconServer(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	images := [][]byte{
		readIconFixture(t, fixtureIconPath(t)),
		readIconFixture(t, fixtureAltIconPath(t)),
	}

	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(images[int(n-1)%len(images)])
	}))
	t.Cleanup(server.Close)

	return server.URL + "/icon.png", &hits
}

// readIconFixture returns the bytes of a committed PNG fixture.
func readIconFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // committed test fixture
	if err != nil {
		t.Fatalf("reading icon fixture %q: %v", path, err)
	}
	return b
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("resolving fixture path: %v", err)
	}
	return p
}

// testAccCheckEnrollmentCustomizationDestroy verifies enrollment customizations
// created during the test were destroyed.
func testAccCheckEnrollmentCustomizationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_enrollment_customization" {
				continue
			}
			_, err := c.GetEnrollmentCustomizationV2(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking enrollment customization %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("enrollment customization %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// configCommon returns the branding settings stanza shared across scenarios.
func configCommon() string {
	return `branding_settings = {
		body_text_color   = "333333"
		button_color      = "0066cc"
		button_text_color = "ffffff"
		background_color  = "ffffff"
	}`
}

// TestAccResource_ProEnrollmentCustomization_TextOnly exercises a parent with
// two text panes only.
func TestAccResource_ProEnrollmentCustomization_TextOnly(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-text-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc text-only"
			%s
			text_panes = [
				{ display_name = "Welcome", rank = 0, title = "Hello", body = "Body",
				  previous_button_text = "Back", next_button_text = "Next" },
				{ display_name = "EULA",    rank = 1, title = "EULA",  body = "Terms",
				  previous_button_text = "Back", next_button_text = "Accept" },
			]
		}
	`, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "id"),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "display_name", name),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.#", "2"),
				resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "text_panes.0.id"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_LdapOnly exercises a single LDAP
// pane with a directory_service_groups entry.
func TestAccResource_ProEnrollmentCustomization_LdapOnly(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-ldap-" + suffix
	cfg := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc ldap"
			%s
			ldap_panes = [{
				display_name         = "LDAP login"
				rank                 = 0
				title                = "Sign in"
				username_text        = "Username"
				password_text        = "Password"
				previous_button_text = "Back"
				login_button_text    = "Login"
			}]
		}
	`, testhelpers.LdapServerFixture(name, ldapEnv), name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				// Assert the LDAP fixture itself created — this is the directory
				// service the LDAP pane depends on.
				resource.TestCheckResourceAttrSet("jamfplatform_pro_ldap_server.acc_ldap", "id"),
				resource.TestCheckResourceAttr("jamfplatform_pro_ldap_server.acc_ldap", "connection_settings.display_name", name+"-Okta"),
				resource.TestCheckResourceAttr("jamfplatform_pro_ldap_server.acc_ldap", "connection_settings.directory_service", "Custom"),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "1"),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.0.username_text", "Username"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_SsoOnly exercises a single SSO
// pane with enrollment_access = any_idp_user.
func TestAccResource_ProEnrollmentCustomization_SsoOnly(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireSsoIdpURL(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-sso-" + suffix
	cfg := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_sso_settings.ec_fixture]
			display_name = %q
			description  = "tf acc sso"
			%s
			sso_panes = [{
				display_name              = "SSO"
				rank                      = 0
				enrollment_access         = "any_idp_user"
				account_name_attribute    = "shortName"
				account_full_name_attribute = "fullName"
			}]
		}
	`, ssoSamlFixture(idpURL), name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.0.enrollment_access", "any_idp_user"),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.0.account_name_attribute", "shortName"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_TextAndLdap mixes one text pane
// with one LDAP pane.
func TestAccResource_ProEnrollmentCustomization_TextAndLdap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-textldap-" + suffix
	cfg := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc text+ldap"
			%s
			text_panes = [{
				display_name = "Welcome"
				rank         = 0
				title        = "Hello"
				body         = "Body"
				previous_button_text = "Back"
				next_button_text     = "Next"
			}]
			ldap_panes = [{
				display_name         = "LDAP login"
				rank                 = 1
				title                = "Sign in"
				username_text        = "Username"
				password_text        = "Password"
				previous_button_text = "Back"
				login_button_text    = "Login"
			}]
		}
	`, testhelpers.LdapServerFixture(name, ldapEnv), name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.#", "1"),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "1"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_TextAndSso mixes one text pane
// with one SSO pane (specific_group with access_group_name).
func TestAccResource_ProEnrollmentCustomization_TextAndSso(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireSsoIdpURL(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-textsso-" + suffix
	cfg := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_sso_settings.ec_fixture]
			display_name = %q
			description  = "tf acc text+sso"
			%s
			text_panes = [{
				display_name = "Welcome"
				rank         = 0
				title        = "Hello"
				body         = "Body"
				previous_button_text = "Back"
				next_button_text     = "Next"
			}]
			sso_panes = [{
				display_name        = "SSO"
				rank                = 1
				enrollment_access   = "specific_group"
				access_group_name   = "Enrollers"
			}]
		}
	`, ssoSamlFixture(idpURL), name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.0.enrollment_access", "specific_group"),
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.0.access_group_name", "Enrollers"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_IconSourceUpload exercises the
// image upload + ModifyPlan path using the committed 100x100 PNG fixture under
// testdata/.
func TestAccResource_ProEnrollmentCustomization_IconSourceUpload(t *testing.T) {
	testhelpers.AccPreCheck(t)
	pngPath := fixtureIconPath(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-img-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc icon source"
			icon_source  = %q
			%s
		}
	`, name, pngPath, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					// Resolved in Create from the bytes uploaded, so it cannot
					// be planned (issue #373).
					plancheck.ExpectUnknownValue("jamfplatform_pro_enrollment_customization.test", tfjsonpath.New("icon_source_hash")),
				},
			},
			Check: resource.ComposeAggregateTestCheckFunc(
				// The committed hash must be the fixture's own, which is what
				// proves it came from the bytes the upload received.
				resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "icon_source_hash", fixtureIconHash(t, pngPath)),
				resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "branding_settings.icon_url"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_IconSource_UnstableURL is the
// acceptance cover for issue #373 on this resource, reproduced against a live
// tenant on 2026-09-04.
//
// The condition is a URL whose bytes differ between two requests, which no
// public URL can be relied on to do on demand, so the test serves it. The apply
// has to succeed on the first run, the re-apply has to plan nothing, and the
// provider has to have read the URL exactly once across both: before the fix it
// read it on every plan, so the count is the assertion that would still catch a
// plan-time fetch reintroduced somewhere the hash comparison no longer visits.
func TestAccResource_ProEnrollmentCustomization_IconSource_UnstableURL(t *testing.T) {
	testhelpers.AccPreCheck(t)
	url, hits := unstableIconServer(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-iconurl-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc unstable icon url"
			icon_source  = %q
			%s
		}
	`, name, url, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("jamfplatform_pro_enrollment_customization.test", tfjsonpath.New("icon_source_hash")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("jamfplatform_pro_enrollment_customization.test", "icon_source_hash", regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "branding_settings.icon_url"),
				),
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})

	if got := hits.Load(); got != 1 {
		t.Fatalf("the provider fetched the icon URL %d times, want 1: only the upload may read it, or an unstable source plans one hash and applies another", got)
	}
}

// TestAccResource_ProEnrollmentCustomization_IconSource_UnrelatedChangeKeepsIcon
// is the regression the unknown-hash signal most invites. Update uploads when
// the planned hash is unknown, so a plan that leaves the icon alone must leave
// the hash known: otherwise renaming a customization silently re-uploads its
// icon and mints a new image on the tenant every apply.
func TestAccResource_ProEnrollmentCustomization_IconSource_UnrelatedChangeKeepsIcon(t *testing.T) {
	testhelpers.AccPreCheck(t)
	pngPath := fixtureIconPath(t)
	suffix := testhelpers.RunSuffix()

	mkCfg := func(desc string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_enrollment_customization" "test" {
				display_name = %q
				description  = %q
				icon_source  = %q
				%s
			}
		`, "tf-acc-ec-iconkeep-"+suffix, desc, pngPath, configCommon())
	}

	iconHash := fixtureIconHash(t, pngPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mkCfg("tf acc icon keep"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "icon_source_hash", iconHash),
				),
			},
			{
				Config: mkCfg("tf acc icon keep, edited"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jamfplatform_pro_enrollment_customization.test", plancheck.ResourceActionUpdate),
						// Known, not unknown: nothing about the icon changed, so
						// Update must not upload.
						plancheck.ExpectKnownValue("jamfplatform_pro_enrollment_customization.test", tfjsonpath.New("icon_source_hash"), knownvalue.StringExact(iconHash)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "icon_source_hash", iconHash),
				),
			},
		},
	})
}

// TestAccResource_ProEnrollmentCustomization_IconURLOverride exercises the
// "pre-uploaded URL" branch. A seed resource uploads the fixture PNG; the
// test resource then references the seed's server-returned icon_url so the
// pass-through path runs without depending on an externally provisioned URL.
func TestAccResource_ProEnrollmentCustomization_IconURLOverride(t *testing.T) {
	testhelpers.AccPreCheck(t)
	pngPath := fixtureIconPath(t)
	suffix := testhelpers.RunSuffix()
	seedName := "tf-acc-ec-url-seed-" + suffix
	name := "tf-acc-ec-url-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "seed" {
			display_name = %q
			description  = "tf acc icon url seed"
			icon_source  = %q
			%s
		}

		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc icon url"
			branding_settings = {
				body_text_color   = "333333"
				button_color      = "0066cc"
				button_text_color = "ffffff"
				background_color  = "ffffff"
				icon_url          = jamfplatform_pro_enrollment_customization.seed.branding_settings.icon_url
			}
		}
	`, seedName, pngPath, configCommon(), name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrPair(
					"jamfplatform_pro_enrollment_customization.test", "branding_settings.icon_url",
					"jamfplatform_pro_enrollment_customization.seed", "branding_settings.icon_url",
				),
				resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "branding_settings.icon_url"),
			),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_ImportRoundTrip imports an
// existing record and asserts state matches.
func TestAccResource_ProEnrollmentCustomization_ImportRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-imp-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc import"
			%s
		}
	`, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				ResourceName:      "jamfplatform_pro_enrollment_customization.test",
				ImportState:       true,
				ImportStateVerify: true,
				// icon_source is null on import and recomputed from null state — exclude
				// from verify; icon_source_hash similarly is null on import.
				ImportStateVerifyIgnore: []string{"icon_source", "icon_source_hash", "timeouts"},
			},
		},
	})
}

// TestAccResource_ProEnrollmentCustomization_AlreadyHasAuth_Rejection asserts
// that supplying both ldap_panes and sso_panes is rejected at plan time by
// the framework's ConflictsWith. Server's ALREADY_HAS_AUTH guard is the
// backup; the client validator is the first line of defence.
func TestAccResource_ProEnrollmentCustomization_AlreadyHasAuth_Rejection(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-auth-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc auth conflict"
			%s
			ldap_panes = [{
				display_name        = "LDAP"
				rank                = 0
				title               = "T"
				username_text       = "U"
				password_text       = "P"
				previous_button_text = "Back"
				login_button_text   = "Login"
			}]
			sso_panes = [{
				display_name      = "SSO"
				rank              = 1
				enrollment_access = "any_idp_user"
			}]
		}
	`, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?i)cannot be specified when`),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_UpdateRoundTrip drives the Update
// CRUD path across multiple mutations: branding palette change, text pane body
// mutation, text pane add, text pane removal, and LDAP pane attach/detach. All
// mutations stay within a single auth class (none → ldap → none) because the
// server's tolerance for cross-auth Update transitions is not yet wire-probed.
func TestAccResource_ProEnrollmentCustomization_UpdateRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-update-" + suffix
	// step 2 attaches an LDAP pane, so the directory-service fixture must exist;
	// keep it stable across all three steps to avoid create/destroy churn.
	fixture := testhelpers.LdapServerFixture(name, ldapEnv)

	// step 1: single text pane, baseline branding.
	step1 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc update step1"
			branding_settings = {
				body_text_color   = "111111"
				button_color      = "0066cc"
				button_text_color = "ffffff"
				background_color  = "ffffff"
			}
			text_panes = [{
				display_name = "Welcome"
				rank         = 0
				title        = "Hello"
				body         = "Body v1"
				previous_button_text = "Back"
				next_button_text     = "Next"
			}]
		}
	`, fixture, name)

	// step 2: branding palette mutated, text pane body mutated, second text
	// pane appended, LDAP pane attached.
	step2 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc update step2"
			branding_settings = {
				body_text_color   = "222222"
				button_color      = "112233"
				button_text_color = "fefefe"
				background_color  = "eeeeee"
			}
			text_panes = [
				{
					display_name = "Welcome"
					rank         = 0
					title        = "Hello"
					body         = "Body v2"
					previous_button_text = "Back"
					next_button_text     = "Next"
				},
				{
					display_name = "Terms"
					rank         = 1
					title        = "Terms"
					body         = "Read carefully"
					previous_button_text = "Back"
					next_button_text     = "Accept"
				},
			]
			ldap_panes = [{
				display_name        = "LDAP"
				rank                = 2
				title               = "Sign in"
				username_text       = "Username"
				password_text       = "Password"
				previous_button_text = "Back"
				login_button_text   = "Login"
			}]
		}
	`, fixture, name)

	// step 3: LDAP pane removed, second text pane removed (back to single text
	// pane). Description mutated to assert plain-string Update too.
	step3 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc update step3"
			branding_settings = {
				body_text_color   = "222222"
				button_color      = "112233"
				button_text_color = "fefefe"
				background_color  = "eeeeee"
			}
			text_panes = [{
				display_name = "Welcome"
				rank         = 0
				title        = "Hello"
				body         = "Body v3"
				previous_button_text = "Back"
				next_button_text     = "Next"
			}]
		}
	`, fixture, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.0.body", "Body v1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "branding_settings.body_text_color", "111111"),
				),
			},
			{
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.#", "2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.0.body", "Body v2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.1.display_name", "Terms"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "branding_settings.body_text_color", "222222"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "branding_settings.background_color", "eeeeee"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "description", "tf acc update step2"),
				),
			},
			{
				Config: step3,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "text_panes.0.body", "Body v3"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "0"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "description", "tf acc update step3"),
				),
			},
		},
	})
}

// TestAccResource_ProEnrollmentCustomization_CrossAuthUpdate_LdapToSso wire-
// probes whether Jamf Pro tolerates swapping an LDAP authentication pane for
// an SSO pane via Update on a customization already carrying auth.
//
// Server tolerance is currently unknown. The test is written for the
// permissive ("Jamf accepts the swap") outcome: step 1 creates with
// ldap_panes, step 2 mutates to sso_panes. If the server rejects the
// transition with ALREADY_HAS_AUTH (or any other guard), this test fails and
// must be flipped to use `ExpectError` on step 2 with the documented
// rejection pattern — record the wire-probe outcome in the
// `project_pro_enrollment_customization_spike` memory.
func TestAccResource_ProEnrollmentCustomization_CrossAuthUpdate_LdapToSso(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireSsoIdpURL(t)
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-xauth-l2s-" + suffix
	// Both fixtures present throughout: SAML for the SSO pane (step 2), the LDAP
	// directory service for the LDAP pane (step 1).
	fixture := ssoSamlFixture(idpURL) + testhelpers.LdapServerFixture(name, ldapEnv)

	step1 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_sso_settings.ec_fixture, jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc cross-auth ldap->sso step1"
			%s
			ldap_panes = [{
				display_name        = "LDAP"
				rank                = 0
				title               = "Sign in"
				username_text       = "Username"
				password_text       = "Password"
				previous_button_text = "Back"
				login_button_text   = "Login"
			}]
		}
	`, fixture, name, configCommon())

	step2 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_sso_settings.ec_fixture, jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc cross-auth ldap->sso step2"
			%s
			sso_panes = [{
				display_name      = "SSO"
				rank              = 0
				enrollment_access = "any_idp_user"
			}]
		}
	`, fixture, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.#", "0"),
				),
			},
			{
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "0"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.0.enrollment_access", "any_idp_user"),
				),
			},
		},
	})
}

// TestAccResource_ProEnrollmentCustomization_CrossAuthUpdate_SsoToLdap mirrors
// the LdapToSso probe in the other direction. See LdapToSso doc for the
// rejection-pattern fallback if the server refuses the swap.
func TestAccResource_ProEnrollmentCustomization_CrossAuthUpdate_SsoToLdap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	idpURL := requireSsoIdpURL(t)
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-xauth-s2l-" + suffix
	// Both fixtures present throughout: SAML for the SSO pane (step 1), the LDAP
	// directory service for the LDAP pane (step 2).
	fixture := ssoSamlFixture(idpURL) + testhelpers.LdapServerFixture(name, ldapEnv)

	step1 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_sso_settings.ec_fixture, jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc cross-auth sso->ldap step1"
			%s
			sso_panes = [{
				display_name      = "SSO"
				rank              = 0
				enrollment_access = "any_idp_user"
			}]
		}
	`, fixture, name, configCommon())

	step2 := fmt.Sprintf(`
		%s
		resource "jamfplatform_pro_enrollment_customization" "test" {
			depends_on   = [jamfplatform_pro_sso_settings.ec_fixture, jamfplatform_pro_ldap_server.acc_ldap]
			display_name = %q
			description  = "tf acc cross-auth sso->ldap step2"
			%s
			ldap_panes = [{
				display_name        = "LDAP"
				rank                = 0
				title               = "Sign in"
				username_text       = "Username"
				password_text       = "Password"
				previous_button_text = "Back"
				login_button_text   = "Login"
			}]
		}
	`, fixture, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "0"),
				),
			},
			{
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "sso_panes.#", "0"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "ldap_panes.0.display_name", "LDAP"),
				),
			},
		},
	})
}

// TestAccResource_ProEnrollmentCustomization_DuplicateDisplayName_Rejection
// asserts the shared unique-display_name validator rejects two text panes that share a
// display_name at plan time.
func TestAccResource_ProEnrollmentCustomization_DuplicateDisplayName_Rejection(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-dup-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc duplicate display_name"
			%s
			text_panes = [
				{
					display_name = "Same"
					rank         = 0
					title        = "T1"
					body         = "B1"
					previous_button_text = "Back"
					next_button_text     = "Next"
				},
				{
					display_name = "Same"
					rank         = 1
					title        = "T2"
					body         = "B2"
					previous_button_text = "Back"
					next_button_text     = "Next"
				},
			]
		}
	`, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?i)display_name`),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_AccessGroupName_Required asserts
// AccessGroupNameValidator rejects an SSO pane with enrollment_access =
// "specific_group" and no access_group_name.
func TestAccResource_ProEnrollmentCustomization_AccessGroupName_Required(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-agn-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc access_group_name required"
			%s
			sso_panes = [{
				display_name      = "SSO"
				rank              = 0
				enrollment_access = "specific_group"
			}]
		}
	`, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?i)access_group_name`),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_EnrollmentAccess_InvalidEnum
// asserts the OneOf validator on enrollment_access rejects an unknown value.
func TestAccResource_ProEnrollmentCustomization_EnrollmentAccess_InvalidEnum(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-enum-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc enrollment_access enum"
			%s
			sso_panes = [{
				display_name      = "SSO"
				rank              = 0
				enrollment_access = "everyone"
			}]
		}
	`, name, configCommon())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?i)enrollment_access`),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_IconConflict_Rejection asserts
// the ConflictsWith pair on icon_source vs branding_settings.icon_url is
// enforced at plan time.
func TestAccResource_ProEnrollmentCustomization_IconConflict_Rejection(t *testing.T) {
	testhelpers.AccPreCheck(t)
	pngPath := fixtureIconPath(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-iconconf-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_enrollment_customization" "test" {
			display_name = %q
			description  = "tf acc icon conflict"
			icon_source  = %q
			branding_settings = {
				body_text_color   = "333333"
				button_color      = "0066cc"
				button_text_color = "ffffff"
				background_color  = "ffffff"
				icon_url          = "https://example.invalid/preuploaded.png"
			}
		}
	`, name, pngPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?i)cannot be specified when`),
		}},
	})
}

// TestAccResource_ProEnrollmentCustomization_IconSource_DriftOnUpdate
// exercises ModifyPlan's hash-change detection. Step 1 uploads icon.png; step
// 2 swaps icon_source to a different PNG and asserts icon_source_hash and
// branding_settings.icon_url both change.
func TestAccResource_ProEnrollmentCustomization_IconSource_DriftOnUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	pngPath := fixtureIconPath(t)
	altPath := fixtureAltIconPath(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ec-icondrift-" + suffix

	mkCfg := func(src string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_enrollment_customization" "test" {
				display_name = %q
				description  = "tf acc icon drift"
				icon_source  = %q
				%s
			}
		`, name, src, configCommon())
	}

	var initialHash, initialURL string
	captureAttr := func(attr string, dst *string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs, ok := s.RootModule().Resources["jamfplatform_pro_enrollment_customization.test"]
			if !ok {
				return fmt.Errorf("resource not found in state")
			}
			v, ok := rs.Primary.Attributes[attr]
			if !ok || v == "" {
				return fmt.Errorf("attribute %s missing or empty", attr)
			}
			*dst = v
			return nil
		}
	}
	expectAttrChanged := func(attr string, prior *string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs, ok := s.RootModule().Resources["jamfplatform_pro_enrollment_customization.test"]
			if !ok {
				return fmt.Errorf("resource not found in state")
			}
			v, ok := rs.Primary.Attributes[attr]
			if !ok || v == "" {
				return fmt.Errorf("attribute %s missing or empty post-update", attr)
			}
			if v == *prior {
				return fmt.Errorf("attribute %s unchanged across update (%q); icon-drift recompute did not run", attr, v)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentCustomizationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mkCfg(pngPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "icon_source_hash"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_enrollment_customization.test", "branding_settings.icon_url"),
					captureAttr("icon_source_hash", &initialHash),
					captureAttr("branding_settings.icon_url", &initialURL),
				),
			},
			{
				Config: mkCfg(altPath),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// A customization has an update endpoint, so a new icon
						// is delivered in place rather than by replacement.
						plancheck.ExpectResourceAction("jamfplatform_pro_enrollment_customization.test", plancheck.ResourceActionUpdate),
						// Both values the upload settles go unknown together:
						// the hash is the signal Update reads, and icon_url is
						// what the upload returns (issue #373).
						plancheck.ExpectUnknownValue("jamfplatform_pro_enrollment_customization.test", tfjsonpath.New("icon_source_hash")),
						plancheck.ExpectUnknownValue("jamfplatform_pro_enrollment_customization.test", tfjsonpath.New("branding_settings").AtMapKey("icon_url")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectAttrChanged("icon_source_hash", &initialHash),
					expectAttrChanged("branding_settings.icon_url", &initialURL),
					// The committed hash must be the new fixture's own, which is
					// what proves it came from the bytes the upload received.
					resource.TestCheckResourceAttr("jamfplatform_pro_enrollment_customization.test", "icon_source_hash", fixtureIconHash(t, altPath)),
				),
			},
		},
	})
}
