// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic
// /diskencryptionconfigurations endpoint. Classic has known concurrency
// issues when multiple writes hit the same resource type — keep these
// tests serial with any other classic acceptance work in this package.

package disk_encryption_configuration_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckDiskEncryptionConfigurationDestroy verifies configurations
// created during the test were destroyed.
func testAccCheckDiskEncryptionConfigurationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_disk_encryption_configuration" {
				continue
			}
			_, err := c.GetDiskEncryptionConfigurationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro disk encryption configuration %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro disk encryption configuration %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProDiskEncryptionConfiguration_Individual covers the
// minimal Individual key_type path: no IRK block, server emits the
// empty IRK block on read which must collapse to a nil TF model
// (otherwise we'd see a perma-diff). Step 2 flips
// file_vault_enabled_users and verifies the rest of the envelope
// holds steady. Step 3 imports.
func TestAccResource_ProDiskEncryptionConfiguration_Individual(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-disk-encryption-individual-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDiskEncryptionConfigurationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_disk_encryption_configuration" "test" {
						name                     = %q
						key_type                 = "Individual"
						file_vault_enabled_users = "Current or Next User"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_disk_encryption_configuration.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "key_type", "Individual"),
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "file_vault_enabled_users", "Current or Next User"),
					// IRK block must collapse to nil — see
					// state_builders.go assignIRKResourceModel for the
					// rationale (server always emits empty IRK block
					// on read).
					resource.TestCheckNoResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "institutional_recovery_key"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_disk_encryption_configuration" "test" {
						name                     = %q
						key_type                 = "Individual"
						file_vault_enabled_users = "Management Account"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "file_vault_enabled_users", "Management Account"),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_disk_encryption_configuration.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProDiskEncryptionConfiguration_Institutional_DER covers
// the institutional path with a DER-format certificate (no password
// required — DER is public-cert only). The fixture generator
// (helpers_test.go) builds a fresh self-signed cert per run so the
// server's cert validation logic gets a real X.509 to parse.
func TestAccResource_ProDiskEncryptionConfiguration_Institutional_DER(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-disk-encryption-institutional-" + suffix
	derB64 := loadDERFixture(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDiskEncryptionConfigurationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_disk_encryption_configuration" "test" {
						name                     = %q
						key_type                 = "Individual and Institutional"
						file_vault_enabled_users = "Current or Next User"

						institutional_recovery_key = {
							certificate_type = "DER"
							data             = %q
						}
					}
				`, name, derB64),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_disk_encryption_configuration.test", "id"),
					// Wire-canonical form: lowercase `and`.
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "key_type", "Individual and Institutional"),
					// Server derives Subject DN from the cert.
					resource.TestCheckResourceAttrSet("jamfplatform_pro_disk_encryption_configuration.test", "institutional_recovery_key.key"),
					// DER public-cert upload — server tags it as DER.
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "institutional_recovery_key.certificate_type", "DER"),
					// password_sha256 is no longer surfaced by the provider
					// (the wire value is a literal 20-asterisk redaction
					// string with no drift-detection signal). Assertion
					// retained as a regression guard that the attribute
					// does not return.
					resource.TestCheckNoResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "institutional_recovery_key.password_sha256"),
				),
			},
			{
				// IRK-preservation across an unrelated update. Audit
				// §2.6 confirms Classic PUT applies a partial merge:
				// flipping `file_vault_enabled_users` must leave the
				// stored cert intact. The schema description warns
				// users that the IRK block CANNOT be cleared via PUT
				// (§2.7) — this step pins the corollary that it also
				// cannot be accidentally lost.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_disk_encryption_configuration" "test" {
						name                     = %q
						key_type                 = "Individual and Institutional"
						file_vault_enabled_users = "Management Account"

						institutional_recovery_key = {
							certificate_type = "DER"
							data             = %q
						}
					}
				`, name, derB64),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "file_vault_enabled_users", "Management Account"),
					// Cert survived the update.
					resource.TestCheckResourceAttrSet("jamfplatform_pro_disk_encryption_configuration.test", "institutional_recovery_key.key"),
					resource.TestCheckResourceAttr("jamfplatform_pro_disk_encryption_configuration.test", "institutional_recovery_key.certificate_type", "DER"),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_disk_encryption_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					// `institutional_recovery_key.password` is
					// write-only and cannot be reconstructed by
					// import — no plaintext on the wire.
					"institutional_recovery_key.password",
				},
			},
		},
	})
}

// TestAccResource_ProDiskEncryptionConfiguration_InstitutionalRequiresIRK
// exercises the plan-time cross-field validator. key_type=Institutional
// without an institutional_recovery_key block must surface a typed
// plan-time error and never reach apply.
func TestAccResource_ProDiskEncryptionConfiguration_InstitutionalRequiresIRK(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-disk-encryption-missing-irk-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_disk_encryption_configuration" "test" {
						name                     = %q
						key_type                 = "Institutional"
						file_vault_enabled_users = "Current or Next User"
					}
				`, name),
				ExpectError: regexp.MustCompile(`institutional_recovery_key required when key_type`),
			},
		},
	})
}

// TestAccResource_ProDiskEncryptionConfiguration_InstitutionalRequiresData
// exercises the variant where the IRK block is supplied but `data` is
// missing. `data` is marked Required at the schema layer, so the
// framework rejects the plan before the resource's ValidateConfig runs.
func TestAccResource_ProDiskEncryptionConfiguration_InstitutionalRequiresData(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-disk-encryption-missing-data-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_disk_encryption_configuration" "test" {
						name                     = %q
						key_type                 = "Institutional"
						file_vault_enabled_users = "Current or Next User"

						institutional_recovery_key = {
							# data and certificate_type deliberately missing
						}
					}
				`, name),
				ExpectError: regexp.MustCompile(`(?s)attributes\s+"certificate_type"\s+and\s+"data"\s+are\s+required`),
			},
		},
	})
}
