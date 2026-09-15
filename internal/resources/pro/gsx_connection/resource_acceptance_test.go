// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package gsx_connection_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// GSX Connection acceptance requires a REAL, Apple-registered GSX certificate, token, and
// service account. Wire-probing (2026-06-09) proved every write — even with enabled=false —
// is validated against Apple's live GSX service, so a self-signed dummy cannot be used: the
// server rejects it with a GSX 401. All write-path tests are therefore gated behind env
// vars and skipped when unset; only the non-singleton-import guard runs without credentials.
//
// ⚠️ These tests OVERWRITE the tenant's live GSX connection with the supplied certificate.
// The WriteOnly secrets cannot be read back, so the prior connection is not restorable from
// state — restore it manually afterward if needed. CheckDestroy verifies the singleton
// record persists (Delete is state-only).
const (
	envGsxToken            = "JAMFPLATFORM_ACC_PRO_GSX_TOKEN"
	envGsxKeystoreBase64   = "JAMFPLATFORM_ACC_PRO_GSX_KEYSTORE_BASE64"
	envGsxKeystorePassword = "JAMFPLATFORM_ACC_PRO_GSX_KEYSTORE_PASSWORD"
	envGsxUsername         = "JAMFPLATFORM_ACC_PRO_GSX_USERNAME"
	envGsxServiceAccount   = "JAMFPLATFORM_ACC_PRO_GSX_SERVICE_ACCOUNT"
	envGsxShipTo           = "JAMFPLATFORM_ACC_PRO_GSX_SHIP_TO" // optional
)

type gsxCreds struct {
	token, keystoreB64, keystorePassword, username, serviceAccount, shipTo string
}

// requireGsxCreds returns the real GSX credentials from the environment, skipping the test
// when any mandatory value is unset.
func requireGsxCreds(t *testing.T) gsxCreds {
	t.Helper()
	c := gsxCreds{
		token:            testhelpers.AccEnv(envGsxToken),
		keystoreB64:      testhelpers.AccEnv(envGsxKeystoreBase64),
		keystorePassword: testhelpers.AccEnv(envGsxKeystorePassword),
		username:         testhelpers.AccEnv(envGsxUsername),
		serviceAccount:   testhelpers.AccEnv(envGsxServiceAccount),
		shipTo:           testhelpers.AccEnv(envGsxShipTo),
	}
	if c.token == "" || c.keystoreB64 == "" || c.keystorePassword == "" || c.username == "" || c.serviceAccount == "" {
		t.Skipf("skipping: set %s, %s, %s, %s, %s to a real Apple-registered GSX certificate/token/account to exercise GSX Connection write tests",
			envGsxToken, envGsxKeystoreBase64, envGsxKeystorePassword, envGsxUsername, envGsxServiceAccount)
	}
	return c
}

// checkSingletonRecordStillExists verifies the GSX Connection record persists on the tenant
// after Terraform destroys the resource from state (Delete is a no-op for singletons).
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "GSX Connection", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetGSXConnectionV1(ctx)
	})
}

func gsxConfig(c gsxCreds, enabled bool) string {
	shipTo := ""
	if c.shipTo != "" {
		shipTo = fmt.Sprintf("ship_to_number = %q\n", c.shipTo)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_gsx_connection_settings" "test" {
  enabled                = %t
  username               = %q
  service_account_number = %q
  %s
  token_wo               = %q
  keystore_bytes_wo      = %q
  keystore_password_wo   = %q
  keystore_name          = "acc-test.p12"
}
`, enabled, c.username, c.serviceAccount, shipTo, c.token, c.keystoreB64, c.keystorePassword)
}

// TestAccResource_ProGsxConnectionSettings_Basic exercises create + a multi-step update
// (toggle enabled) against a real tenant with real GSX credentials, then an import
// round-trip. The WriteOnly secrets are not in state, so import ignores them.
func TestAccResource_ProGsxConnectionSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	creds := requireGsxCreds(t)

	const addr = "jamfplatform_pro_gsx_connection_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: gsxConfig(creds, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttr(addr, "enabled", "false"),
					resource.TestCheckResourceAttr(addr, "username", creds.username),
					resource.TestCheckResourceAttr(addr, "service_account_number", creds.serviceAccount),
					// Read-only certificate metadata is populated post-apply.
					resource.TestCheckResourceAttrSet(addr, "keystore_expiration_epoch"),
				),
			},
			{
				Config: gsxConfig(creds, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "enabled", "true"),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				// WriteOnly secrets are never in state; timeouts is not imported.
				ImportStateVerifyIgnore: []string{"timeouts", "token_wo", "keystore_bytes_wo", "keystore_password_wo"},
			},
		},
	})
}

// TestAccResource_ProGsxConnectionSettings_RejectsNonSingletonImport verifies the import
// guard against a real tenant. The resource cannot be applied without a valid GSX
// certificate, so the first (apply) step requires credentials and the test skips without
// them. A credential-free unit test of the same guard lives in import_test.go.
func TestAccResource_ProGsxConnectionSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	creds := requireGsxCreds(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{Config: gsxConfig(creds, false)},
			{
				ResourceName:  "jamfplatform_pro_gsx_connection_settings.test",
				ImportState:   true,
				ImportStateId: "not-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}
