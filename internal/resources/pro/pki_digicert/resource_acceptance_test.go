// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package pki_digicert_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// DigiCert acceptance runs entirely against a committed self-signed dummy .p12.
// Wire-probing (2026-06-09) proved DigiCert accepts a self-signed certificate at
// create — validation happens at connection-test time, which this resource does
// not model — so no real DigiCert One infrastructure is required.
const (
	resourceAddr        = "jamfplatform_pro_pki_digicert.test"
	dummyP12Password    = "dummy-p12-password"
	detailsSerialAttr   = "client_certificate_details.serial_number"
	detailsSubjectAttr  = "client_certificate_details.subject"
	detailsFilenameAttr = "client_certificate_details.filename"
)

// dummyP12Base64 reads the bundled self-signed dummy keystore and base64-encodes it.
func dummyP12Base64(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "testdata", "pki-dummy.p12")
	b, err := os.ReadFile(p) //nolint:gosec // test fixture path is fixed
	if err != nil {
		t.Fatalf("reading dummy p12 fixture: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func testAccCheckDigicertDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_pki_digicert" {
				continue
			}
			_, err := c.GetDigicertTrustLifecycleManagerV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking DigiCert integration %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("DigiCert integration %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// digicertConfig builds the HCL. woVersion drives certificate rotation.
func digicertConfig(displayName, host string, revocation bool, b64 string, woVersion int) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_pki_digicert" "test" {
  display_name       = %q
  host_name          = %q
  revocation_enabled = %t

  client_certificate = {
    data_wo     = %q
    password_wo = %q
    filename    = "client.p12"
    wo_version  = %d
  }
}
`, displayName, host, revocation, b64, dummyP12Password, woVersion)
}

// TestAccResource_ProPkiDigicert drives the full lifecycle against a real tenant
// with the committed dummy certificate: create, a multi-step update that edits the
// scalar fields and rotates the certificate via a bumped wo_version, and an
// import round-trip. The Computed client_certificate_details block is asserted
// populated post-apply.
func TestAccResource_ProPkiDigicert(t *testing.T) {
	testhelpers.AccPreCheck(t)
	b64 := dummyP12Base64(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pki-digicert-" + suffix
	nameUpdated := "tf-acc-pki-digicert-upd-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigicertDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with the dummy certificate.
				Config: digicertConfig(name, "one.digicert.com", false, b64, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddr, "id"),
					resource.TestCheckResourceAttr(resourceAddr, "display_name", name),
					resource.TestCheckResourceAttr(resourceAddr, "host_name", "one.digicert.com"),
					resource.TestCheckResourceAttr(resourceAddr, "revocation_enabled", "false"),
					// Server-derived certificate metadata is populated post-apply.
					resource.TestCheckResourceAttrSet(resourceAddr, detailsSerialAttr),
					resource.TestCheckResourceAttrSet(resourceAddr, detailsSubjectAttr),
					resource.TestCheckResourceAttr(resourceAddr, detailsFilenameAttr, "client.p12"),
				),
			},
			{
				// Update: edit scalar fields and rotate the certificate (wo_version bump).
				Config: digicertConfig(nameUpdated, "two.digicert.com", true, b64, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddr, "display_name", nameUpdated),
					resource.TestCheckResourceAttr(resourceAddr, "host_name", "two.digicert.com"),
					resource.TestCheckResourceAttr(resourceAddr, "revocation_enabled", "true"),
					resource.TestCheckResourceAttrSet(resourceAddr, detailsSerialAttr),
				),
			},
			{
				ResourceName:      resourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// The entire client_certificate input block cannot be reconstructed on
				// import: data_wo/password_wo are WriteOnly, wo_version is config-only,
				// and Read never repopulates the input block's filename. timeouts is not
				// imported either.
				ImportStateVerifyIgnore: []string{"timeouts", "client_certificate"},
			},
		},
	})
}
