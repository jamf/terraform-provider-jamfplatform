// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package supervision_identity_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const addr = "jamfplatform_pro_supervision_identity.test"

// Env overrides for the upload path: supply a real .p12 (base64) + its password to
// exercise the import path with a real certificate. When unset, the test generates
// a throwaway self-signed .p12 with openssl; if openssl is also unavailable it skips.
const (
	envUploadP12Base64   = "JAMFPLATFORM_ACC_PRO_SUPERVISION_P12_BASE64"
	envUploadP12Password = "JAMFPLATFORM_ACC_PRO_SUPERVISION_P12_PASSWORD"
)

// testAccCheckSupervisionIdentityDestroy verifies identities created during the
// test were destroyed.
func testAccCheckSupervisionIdentityDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_supervision_identity" {
				continue
			}
			_, err := c.GetSupervisionIdentityV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro supervision identity %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro supervision identity %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func supervisionClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

func generateConfig(displayName string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_supervision_identity" "test" {
  display_name = %q
  password     = "AccSupervisionPassw0rd!"
}
`, displayName)
}

// TestAccResource_ProSupervisionIdentity_Generate exercises the generate path:
// create (Jamf Pro mints the identity), rename (Update), then an import round-trip.
// The write-only password is never in state, so import ignores it.
func TestAccResource_ProSupervisionIdentity_Generate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-supervision-" + suffix
	nameUpdated := "tf-acc-pro-supervision-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSupervisionIdentityDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: generateConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "display_name", name),
					// Jamf Pro names the generated certificate after the display name.
					resource.TestCheckResourceAttr(addr, "common_name", "Jamf Identity - "+name),
					resource.TestCheckResourceAttrSet(addr, "expiration_date"),
				),
			},
			{
				Config: generateConfig(nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "display_name", nameUpdated),
					// A rename does not change the certificate's common name.
					resource.TestCheckResourceAttr(addr, "common_name", "Jamf Identity - "+name),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
				// Write-only secrets are never in state; timeouts is not imported.
				ImportStateVerifyIgnore: []string{"timeouts", "password", "certificate_data"},
			},
		},
	})
}

// TestAccResource_ProSupervisionIdentity_EmptyDisplayName verifies the
// LengthAtLeast(1) validator rejects an empty display name at plan time.
func TestAccResource_ProSupervisionIdentity_EmptyDisplayName(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      generateConfig(""),
				ExpectError: regexp.MustCompile(`length must be at least 1`),
			},
		},
	})
}

// TestAccResource_ProSupervisionIdentity_DeleteRecovery deletes the identity
// out-of-band, then asserts the next refresh detects it gone (Read hits
// IsNotFoundError -> RemoveResource) and the follow-up plan wants to recreate it.
func TestAccResource_ProSupervisionIdentity_DeleteRecovery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-supervision-del-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSupervisionIdentityDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: generateConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					// Delete the identity out-of-band so the next refresh sees it gone.
					func(s *terraform.State) error {
						rs := s.RootModule().Resources[addr]
						if rs == nil {
							return fmt.Errorf("resource %s not found in state", addr)
						}
						id := rs.Primary.Attributes["id"]
						if err := supervisionClient(t).DeleteSupervisionIdentityV1(context.Background(), id); err != nil {
							return fmt.Errorf("out-of-band delete of %s failed: %w", id, err)
						}
						return nil
					},
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_ProSupervisionIdentity_DriftRecovery renames the identity
// out-of-band, asserts the next plan is non-empty (Terraform wants to restore the
// configured name), then re-applies and confirms the name is reconciled.
func TestAccResource_ProSupervisionIdentity_DriftRecovery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-supervision-drift-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSupervisionIdentityDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: generateConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "display_name", name),
					// Rename the identity out-of-band; the next plan must want to fix it.
					func(s *terraform.State) error {
						rs := s.RootModule().Resources[addr]
						if rs == nil {
							return fmt.Errorf("resource %s not found in state", addr)
						}
						id := rs.Primary.Attributes["id"]
						_, err := supervisionClient(t).UpdateSupervisionIdentityV1(
							context.Background(), id, &pro.SupervisionIdentityUpdate{DisplayName: name + "-drifted"})
						if err != nil {
							return fmt.Errorf("out-of-band rename of %s failed: %w", id, err)
						}
						return nil
					},
				),
				ExpectNonEmptyPlan: true,
			},
			{
				Config: generateConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "display_name", name),
				),
			},
		},
	})
}

// TestAccResource_ProSupervisionIdentity_Upload exercises the import path with a
// supplied .p12. A throwaway self-signed certificate (generated with openssl, or
// supplied via env) is sufficient — Jamf Pro accepts any valid .p12. The test
// skips when neither an env-supplied .p12 nor openssl is available.
func TestAccResource_ProSupervisionIdentity_Upload(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-supervision-upload-" + suffix

	p12Base64, password := uploadCertificate(t, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSupervisionIdentityDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_supervision_identity" "test" {
  display_name     = %q
  password         = %q
  certificate_data = %q
}
`, name, password, p12Base64),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "display_name", name),
					resource.TestCheckResourceAttrSet(addr, "common_name"),
					resource.TestCheckResourceAttrSet(addr, "expiration_date"),
				),
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "password", "certificate_data"},
			},
		},
	})
}

// uploadCertificate returns a base64 .p12 and its password for the upload-path
// test. It prefers an env-supplied real certificate; otherwise it generates a
// throwaway self-signed .p12 with openssl. The test is skipped when neither is
// available.
func uploadCertificate(t *testing.T, cn string) (b64, password string) {
	t.Helper()

	if envB64 := testhelpers.AccEnv(envUploadP12Base64); envB64 != "" {
		return envB64, testhelpers.AccEnv(envUploadP12Password)
	}

	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skipf("skipping upload-path test: set %s/%s to a base64 .p12 + password, or install openssl to generate a throwaway certificate",
			envUploadP12Base64, envUploadP12Password)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	certPath := filepath.Join(dir, "cert.pem")
	p12Path := filepath.Join(dir, "identity.p12")
	const pw = "AccUploadPassw0rd!"

	mustRun(t, "openssl", "req", "-x509", "-newkey", "rsa:2048",
		"-keyout", keyPath, "-out", certPath, "-days", "365", "-nodes",
		"-subj", "/CN="+cn)
	mustRun(t, "openssl", "pkcs12", "-export",
		"-inkey", keyPath, "-in", certPath, "-out", p12Path, "-passout", "pass:"+pw)

	raw, err := os.ReadFile(p12Path)
	if err != nil {
		t.Fatalf("reading generated .p12: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw), pw
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
