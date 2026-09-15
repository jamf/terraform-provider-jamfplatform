// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro Venafi PKI endpoints
// (`/api/v1/pki/venafi`). A name-only create needs no external Venafi CA
// infrastructure; the proxy-trust-store steps use a committed dummy PEM.

package pki_venafi_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// dummyPEM reads the committed dummy proxy-trust-store certificate.
func dummyPEM(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "testdata", "pki-dummy.pem")
	b, err := os.ReadFile(p) //nolint:gosec // committed test fixture
	if err != nil {
		t.Fatalf("read dummy PEM: %v", err)
	}
	return string(b)
}

// testAccCheckPkiVenafiDestroy verifies Venafi CAs created during the test were
// destroyed.
func testAccCheckPkiVenafiDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_pki_venafi" {
				continue
			}
			_, err := c.GetVenafiV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro Venafi CA %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro Venafi CA %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func venafiConfig(name, body string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_pki_venafi" "test" {
  name = %q
%s
}
`, name, body)
}

// TestAccResource_ProPkiVenafi_Lifecycle exercises the full lifecycle:
//   - Step 1: name-only create; jamf_public_key Computed populates.
//   - Step 2: set proxy_address/client_id/revocation + rotate refresh_token via
//     a bumped _wo_version; merge-PATCH preserves the rest.
//   - Step 3: clear proxy_address with "".
//   - Step 4: import by id, verifying state round-trips.
func TestAccResource_ProPkiVenafi_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pki-venafi-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPkiVenafiDestroy(t),
		Steps: []resource.TestStep{
			{
				// Name-only create.
				Config: venafiConfig(name, ``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_pki_venafi.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "refresh_token_configured", "false"),
					// Jamf mints a public key at create.
					resource.TestCheckResourceAttrSet("jamfplatform_pro_pki_venafi.test", "jamf_public_key"),
				),
			},
			{
				// Set fields + supply + rotate the refresh token.
				Config: venafiConfig(name, `
  proxy_address            = "proxy.example.com:8443"
  client_id                = "venafi-client-abc"
  revocation_enabled       = true
  refresh_token_wo         = "super-secret-token"
  refresh_token_wo_version = 1`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "proxy_address", "proxy.example.com:8443"),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "client_id", "venafi-client-abc"),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "revocation_enabled", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "refresh_token_configured", "true"),
				),
			},
			{
				// Import-verify while every Optional+Computed field carries a
				// non-empty value that round-trips cleanly through the assigner.
				// (Run before the "" clear: an imported-from-scratch read maps a
				// cleared field to null, which would differ from the live ""
				// state under ImportStateVerify.)
				ResourceName:      "jamfplatform_pro_pki_venafi.test",
				ImportState:       true,
				ImportStateVerify: true,
				// proxy_trust_store round-trips byte-exact, so it stays in the
				// verify set. The write-only token + rotation triggers do not.
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"refresh_token_wo",
					"refresh_token_wo_version",
					"jamf_public_key_rotation",
				},
			},
			{
				// Merge update: change client_id + flip revocation; proxy_address
				// kept. The server rejects an empty proxyAddress ("HTTP Host must
				// not be empty"), so clearing it to "" is not supported. Omitting
				// refresh_token preserves the stored secret.
				Config: venafiConfig(name, `
  proxy_address      = "proxy.example.com:8443"
  client_id          = "venafi-client-xyz"
  revocation_enabled = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "proxy_address", "proxy.example.com:8443"),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "client_id", "venafi-client-xyz"),
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "revocation_enabled", "false"),
					// Omitting the token preserves the stored secret.
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "refresh_token_configured", "true"),
				),
			},
		},
	})
}

// TestAccResource_ProPkiVenafi_ProxyTrustStore covers the proxy-trust-store
// upload, byte-exact round-trip, and clear; and the jamf_public_key rotation
// trigger (the key value must change after a bump).
func TestAccResource_ProPkiVenafi_ProxyTrustStore(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pki-venafi-pts-" + suffix
	pem := dummyPEM(t)

	var keyBeforeRotation string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPkiVenafiDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with a proxy trust store uploaded after the main POST.
				Config: venafiConfig(name, fmt.Sprintf("  proxy_trust_store = %q", pem)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "proxy_trust_store", pem),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_pki_venafi.test", "jamf_public_key"),
					resource.TestCheckResourceAttrWith("jamfplatform_pro_pki_venafi.test", "jamf_public_key", func(v string) error {
						keyBeforeRotation = v
						return nil
					}),
				),
			},
			{
				// Bump the public-key rotation trigger; the key must change.
				Config: venafiConfig(name, fmt.Sprintf("  proxy_trust_store        = %q\n  jamf_public_key_rotation = 1", pem)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "proxy_trust_store", pem),
					resource.TestCheckResourceAttrWith("jamfplatform_pro_pki_venafi.test", "jamf_public_key", func(v string) error {
						if strings.TrimSpace(v) == "" {
							return fmt.Errorf("jamf_public_key empty after rotation")
						}
						if v == keyBeforeRotation {
							return fmt.Errorf("jamf_public_key unchanged after jamf_public_key_rotation bump; expected a regenerated key")
						}
						return nil
					}),
				),
			},
			{
				// Clear the proxy trust store with "".
				Config: venafiConfig(name, `  proxy_trust_store = ""`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_pki_venafi.test", "proxy_trust_store", ""),
				),
			},
		},
	})
}
